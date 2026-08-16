package database

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sync"

	"github.com/uptrace/bun"
)

type Callback func(context.Context) error

type transactionKey struct{}

type transactionState struct {
	db            *DB
	tx            bun.Tx
	mu            sync.Mutex
	rollbackOnly  bool
	rollbackCause error
	afterCommit   []Callback
	afterRollback []Callback
}

// Transaction runs fn in a transaction. Nested calls join the current
// transaction and mark it rollback-only if they return an error.
func (db *DB) Transaction(ctx context.Context, fn func(context.Context) error) error {
	if fn == nil {
		return fmt.Errorf("database: transaction callback is required")
	}
	if existing := transactionFrom(ctx, db); existing != nil {
		err := fn(ctx)
		if err != nil {
			existing.markRollback(err)
		}
		return err
	}

	tx, err := db.bunDB.BeginTx(ctx, &sql.TxOptions{})
	if err != nil {
		return fmt.Errorf("database: begin transaction: %w", err)
	}
	state := &transactionState{db: db, tx: tx}
	txContext := context.WithValue(ctx, transactionKey{}, state)
	callbackErr := fn(txContext)
	if callbackErr != nil {
		state.markRollback(callbackErr)
	}

	rollbackOnly, rollbackCause := state.rollbackState()
	if rollbackOnly {
		if rollbackCause == nil {
			rollbackCause = ErrRollbackOnly
		}
		rollbackErr := tx.Rollback()
		hookErr := runCallbacks(context.WithoutCancel(txContext), state.rollbackCallbacks())
		return errors.Join(rollbackCause, ignoreTxDone(rollbackErr), hookErr)
	}

	if err := tx.Commit(); err != nil {
		rollbackErr := tx.Rollback()
		hookErr := runCallbacks(context.WithoutCancel(txContext), state.rollbackCallbacks())
		return errors.Join(fmt.Errorf("database: commit transaction: %w", err), ignoreTxDone(rollbackErr), hookErr)
	}
	return runCallbacks(context.WithoutCancel(txContext), state.commitCallbacks())
}

func (db *DB) AfterCommit(ctx context.Context, callback Callback) error {
	if callback == nil {
		return fmt.Errorf("database: after-commit callback is required")
	}
	state := transactionFrom(ctx, db)
	if state == nil {
		return fmt.Errorf("database: after-commit callback requires a transaction")
	}
	state.mu.Lock()
	defer state.mu.Unlock()
	state.afterCommit = append(state.afterCommit, callback)
	return nil
}

func (db *DB) AfterRollback(ctx context.Context, callback Callback) error {
	if callback == nil {
		return fmt.Errorf("database: after-rollback callback is required")
	}
	state := transactionFrom(ctx, db)
	if state == nil {
		return fmt.Errorf("database: after-rollback callback requires a transaction")
	}
	state.mu.Lock()
	defer state.mu.Unlock()
	state.afterRollback = append(state.afterRollback, callback)
	return nil
}

func transactionFrom(ctx context.Context, db *DB) *transactionState {
	state, _ := ctx.Value(transactionKey{}).(*transactionState)
	if state == nil || state.db != db {
		return nil
	}
	return state
}

func (state *transactionState) markRollback(cause error) {
	state.mu.Lock()
	defer state.mu.Unlock()
	state.rollbackOnly = true
	if state.rollbackCause == nil {
		state.rollbackCause = cause
	}
}

func (state *transactionState) rollbackState() (bool, error) {
	state.mu.Lock()
	defer state.mu.Unlock()
	return state.rollbackOnly, state.rollbackCause
}

func (state *transactionState) commitCallbacks() []Callback {
	state.mu.Lock()
	defer state.mu.Unlock()
	return append([]Callback(nil), state.afterCommit...)
}

func (state *transactionState) rollbackCallbacks() []Callback {
	state.mu.Lock()
	defer state.mu.Unlock()
	return append([]Callback(nil), state.afterRollback...)
}

func runCallbacks(ctx context.Context, callbacks []Callback) error {
	var result error
	for _, callback := range callbacks {
		result = errors.Join(result, callback(ctx))
	}
	return result
}

func ignoreTxDone(err error) error {
	if errors.Is(err, sql.ErrTxDone) {
		return nil
	}
	return err
}
