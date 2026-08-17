// Package database owns Soro's PostgreSQL pool, Bun bridge, and transactions.
package database

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sync"

	"github.com/datasoro/soro/config"
	"github.com/datasoro/soro/lifecycle"
	"github.com/datasoro/soro/validation"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/uptrace/bun"
	"github.com/uptrace/bun/dialect/pgdialect"
)

// DB owns one pgx pool and the database/sql facade used by Bun.
type DB struct {
	pool      *pgxpool.Pool
	sqlDB     *sql.DB
	bunDB     *bun.DB
	hooks     *lifecycle.Registry
	validator *validation.Engine
	closeOnce sync.Once
	closeErr  error
}

func Open(ctx context.Context, settings config.DatabaseConfig) (*DB, error) {
	if settings.URL == "" {
		return nil, fmt.Errorf("database: URL is required")
	}
	poolConfig, err := pgxpool.ParseConfig(settings.URL)
	if err != nil {
		return nil, fmt.Errorf("database: parse URL: %w", err)
	}
	poolConfig.MinConns = settings.MinConns
	poolConfig.MaxConns = settings.MaxConns
	poolConfig.MaxConnLifetime = settings.MaxConnLifetime
	poolConfig.MaxConnIdleTime = settings.MaxConnIdleTime
	poolConfig.HealthCheckPeriod = settings.HealthCheckEvery
	poolConfig.ConnConfig.ConnectTimeout = settings.ConnectTimeout

	pool, err := pgxpool.NewWithConfig(ctx, poolConfig)
	if err != nil {
		return nil, fmt.Errorf("database: create pool: %w", err)
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("database: connect: %w", err)
	}
	sqlDB := stdlib.OpenDBFromPool(pool)
	bunDB := bun.NewDB(sqlDB, pgdialect.New())
	return newDB(pool, sqlDB, bunDB), nil
}

// Wrap creates a Soro database around an existing Bun database. The wrapper
// owns the Bun database but has no directly exposed pgx pool.
func Wrap(bunDB *bun.DB) (*DB, error) {
	if bunDB == nil {
		return nil, fmt.Errorf("database: Bun database is required")
	}
	return newDB(nil, bunDB.DB, bunDB), nil
}

func newDB(pool *pgxpool.Pool, sqlDB *sql.DB, bunDB *bun.DB) *DB {
	return &DB{
		pool:      pool,
		sqlDB:     sqlDB,
		bunDB:     bunDB,
		hooks:     lifecycle.NewRegistry(),
		validator: validation.New(),
	}
}

func (db *DB) Bun() *bun.DB { return db.bunDB }

// SQL exposes the shared database/sql facade used by Bun. Integrations must
// not close it independently of DB.
func (db *DB) SQL() *sql.DB { return db.sqlDB }

// SQLTx returns the active database/sql transaction carried by ctx.
func (db *DB) SQLTx(ctx context.Context) (*sql.Tx, bool) {
	state := transactionFrom(ctx, db)
	if state == nil {
		return nil, false
	}
	return state.tx.Tx, true
}

// Pool exposes the shared pool intended for pgx-native integrations such as River.
func (db *DB) Pool() *pgxpool.Pool { return db.pool }

func (db *DB) Hooks() *lifecycle.Registry { return db.hooks }

func (db *DB) Validator() *validation.Engine { return db.validator }

func (db *DB) Ping(ctx context.Context) error {
	if db == nil || db.bunDB == nil {
		return fmt.Errorf("database: not initialized")
	}
	return db.bunDB.PingContext(ctx)
}

func (db *DB) Close() error {
	if db == nil {
		return nil
	}
	db.closeOnce.Do(func() {
		if db.bunDB != nil {
			db.closeErr = db.bunDB.Close()
		} else if db.sqlDB != nil {
			db.closeErr = db.sqlDB.Close()
		}
		if db.pool != nil {
			db.pool.Close()
		}
	})
	return db.closeErr
}

func (db *DB) IDB(ctx context.Context) bun.IDB {
	if state := transactionFrom(ctx, db); state != nil {
		return state.tx
	}
	return db.bunDB
}

func (db *DB) InTransaction(ctx context.Context) bool {
	return transactionFrom(ctx, db) != nil
}

var ErrRollbackOnly = errors.New("database: transaction marked rollback-only")
