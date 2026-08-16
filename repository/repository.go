// Package repository provides typed PostgreSQL persistence for Soro models.
package repository

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sort"
	"time"

	"github.com/datasoro/soro/auth"
	"github.com/datasoro/soro/database"
	soroerrors "github.com/datasoro/soro/errors"
	"github.com/datasoro/soro/lifecycle"
	"github.com/datasoro/soro/model"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/uptrace/bun"
)

type deletionScope uint8

const (
	withoutDeleted deletionScope = iota
	withDeleted
	onlyDeleted
)

type Repository[T any] struct {
	db       *database.DB
	metadata *modelMetadata
	scope    deletionScope
	clock    Clock
	uuid     UUIDGenerator
	initErr  error
}

// New constructs a typed repository. An invalid model contract is reported by
// every operation, keeping normal construction compatible with one-value use.
func New[T any](db *database.DB, options ...Option) *Repository[T] {
	settings := settings{clock: time.Now, uuidGenerator: uuid.NewV7}
	for _, option := range options {
		option(&settings)
	}
	metadata, err := inspectModel[T]()
	if db == nil && err == nil {
		err = fmt.Errorf("repository database is required")
	}
	if settings.clock == nil && err == nil {
		err = fmt.Errorf("repository clock is required")
	}
	if settings.uuidGenerator == nil && err == nil {
		err = fmt.Errorf("repository UUID generator is required")
	}
	return &Repository[T]{db: db, metadata: metadata, scope: withoutDeleted, clock: settings.clock, uuid: settings.uuidGenerator, initErr: err}
}

func (r *Repository[T]) WithDeleted() *Repository[T] {
	copy := *r
	copy.scope = withDeleted
	return &copy
}

func (r *Repository[T]) OnlyDeleted() *Repository[T] {
	copy := *r
	copy.scope = onlyDeleted
	return &copy
}

func (r *Repository[T]) Find(ctx context.Context, id uuid.UUID) (*T, error) {
	if err := r.ready(); err != nil {
		return nil, err
	}
	if id == uuid.Nil {
		return nil, soroerrors.InvalidArgument("id is required")
	}
	return r.find(ctx, id, r.scope)
}

func (r *Repository[T]) FindBy(ctx context.Context, field string, value any) (*T, error) {
	if err := r.ready(); err != nil {
		return nil, err
	}
	column, ok := r.metadata.column(field)
	if !ok {
		return nil, soroerrors.InvalidArgument("unsupported repository field " + field)
	}
	entity := new(T)
	query := r.scopedQuery(ctx).Model(entity).Where("? = ?", bun.Ident(column), value).Limit(1)
	if err := query.Scan(ctx); err != nil {
		return nil, r.mapReadError(err)
	}
	return entity, nil
}

func (r *Repository[T]) First(ctx context.Context) (*T, error) {
	if err := r.ready(); err != nil {
		return nil, err
	}
	entity := new(T)
	if err := r.scopedQuery(ctx).Model(entity).OrderExpr("? ASC", bun.Ident("created_at")).Limit(1).Scan(ctx); err != nil {
		return nil, r.mapReadError(err)
	}
	return entity, nil
}

func (r *Repository[T]) All(ctx context.Context) ([]*T, error) {
	if err := r.ready(); err != nil {
		return nil, err
	}
	entities := make([]*T, 0)
	if err := r.scopedQuery(ctx).Model(&entities).Scan(ctx); err != nil {
		return nil, r.mapDatabaseError(err)
	}
	return entities, nil
}

func (r *Repository[T]) Exists(ctx context.Context, id uuid.UUID) (bool, error) {
	if err := r.ready(); err != nil {
		return false, err
	}
	exists, err := r.scopedQuery(ctx).Where("? = ?", bun.Ident("id"), id).Exists(ctx)
	if err != nil {
		return false, r.mapDatabaseError(err)
	}
	return exists, nil
}

func (r *Repository[T]) Count(ctx context.Context) (int, error) {
	if err := r.ready(); err != nil {
		return 0, err
	}
	count, err := r.scopedQuery(ctx).Count(ctx)
	if err != nil {
		return 0, r.mapDatabaseError(err)
	}
	return count, nil
}

// Query returns a Bun query with the repository's deletion scope applied.
func (r *Repository[T]) Query(ctx context.Context) *bun.SelectQuery {
	return r.scopedQuery(ctx)
}

// DB exposes the current Bun executor. It is a bun.Tx when ctx carries a Soro transaction.
func (r *Repository[T]) DB(ctx context.Context) bun.IDB { return r.db.IDB(ctx) }

func (r *Repository[T]) Transaction(ctx context.Context, fn func(context.Context, *Repository[T]) error) error {
	if err := r.ready(); err != nil {
		return err
	}
	if fn == nil {
		return soroerrors.InvalidArgument("transaction callback is required")
	}
	return r.db.Transaction(ctx, func(txContext context.Context) error { return fn(txContext, r) })
}

func (r *Repository[T]) Create(ctx context.Context, entity *T, options ...OperationOption) error {
	if err := r.validateEntity(entity); err != nil {
		return err
	}
	settings := newOperationSettings(options)
	return r.db.Transaction(ctx, func(txContext context.Context) error {
		base := baseOf(entity)
		if base.ID == uuid.Nil {
			id, err := r.uuid()
			if err != nil {
				return fmt.Errorf("repository: generate UUID: %w", err)
			}
			base.ID = id
		}
		now := r.now()
		if base.Metadata == nil {
			base.Metadata = model.Metadata{}
		}
		base.CreatedAt = now
		base.UpdatedAt = now
		base.CreatedBy = auth.ActorID(txContext)
		base.UpdatedBy = cloneUUID(base.CreatedBy)

		lifecycleContext := &lifecycle.Context{
			Operation: lifecycle.OperationCreate,
			Tx:        r.db.IDB(txContext), ActorID: cloneUUID(base.CreatedBy), Metadata: settings.metadata,
		}
		if err := r.registerTerminalHooks(txContext, entity, lifecycleContext, settings); err != nil {
			return err
		}
		if err := r.run(txContext, lifecycle.BeforeValidate, entity, lifecycleContext, settings); err != nil {
			return err
		}
		if err := r.db.Validator().Validate(txContext, entity); err != nil {
			return err
		}
		for _, stage := range []lifecycle.Stage{lifecycle.AfterValidate, lifecycle.BeforeSave, lifecycle.BeforeCreate} {
			if err := r.run(txContext, stage, entity, lifecycleContext, settings); err != nil {
				return err
			}
		}
		if _, err := r.db.IDB(txContext).NewInsert().Model(entity).Exec(txContext); err != nil {
			return r.mapDatabaseError(err)
		}
		for _, stage := range []lifecycle.Stage{lifecycle.AfterCreate, lifecycle.AfterSave} {
			if err := r.run(txContext, stage, entity, lifecycleContext, settings); err != nil {
				return err
			}
		}
		return nil
	})
}

func (r *Repository[T]) Update(ctx context.Context, entity *T, options ...OperationOption) error {
	if err := r.validateEntity(entity); err != nil {
		return err
	}
	if baseOf(entity).ID == uuid.Nil {
		return soroerrors.InvalidArgument("id is required")
	}
	settings := newOperationSettings(options)
	return r.db.Transaction(ctx, func(txContext context.Context) error {
		persisted, err := r.find(txContext, baseOf(entity).ID, withoutDeleted)
		if err != nil {
			return err
		}
		copyImmutableBase(baseOf(entity), baseOf(persisted))
		base := baseOf(entity)
		base.UpdatedAt = r.now()
		base.UpdatedBy = auth.ActorID(txContext)
		changes, err := lifecycle.Compare(persisted, entity)
		if err != nil {
			return err
		}
		lifecycleContext := &lifecycle.Context{
			Operation: lifecycle.OperationUpdate, Changes: changes,
			Tx: r.db.IDB(txContext), ActorID: cloneUUID(base.UpdatedBy), Metadata: settings.metadata,
		}
		if err := r.registerTerminalHooks(txContext, entity, lifecycleContext, settings); err != nil {
			return err
		}
		if err := r.runUpdateBeforeStages(txContext, persisted, entity, lifecycleContext, settings); err != nil {
			return err
		}
		query := r.db.IDB(txContext).NewUpdate().Model(entity).Column(r.updateColumns()...).
			Where("? = ?", bun.Ident("id"), base.ID).Where("? IS NULL", bun.Ident("deleted_at"))
		result, err := query.Exec(txContext)
		if err != nil {
			return r.mapDatabaseError(err)
		}
		if err := ensureAffected(result, r.metadata.resourceName); err != nil {
			return err
		}
		for _, stage := range []lifecycle.Stage{lifecycle.AfterUpdate, lifecycle.AfterSave} {
			if err := r.run(txContext, stage, entity, lifecycleContext, settings); err != nil {
				return err
			}
		}
		return nil
	})
}

func (r *Repository[T]) Save(ctx context.Context, entity *T, options ...OperationOption) error {
	if err := r.validateEntity(entity); err != nil {
		return err
	}
	if baseOf(entity).ID == uuid.Nil {
		return r.Create(ctx, entity, options...)
	}
	return r.Update(ctx, entity, options...)
}

func (r *Repository[T]) Delete(ctx context.Context, id uuid.UUID, options ...OperationOption) error {
	return r.mutateDeletion(ctx, id, lifecycle.OperationDelete, options)
}

func (r *Repository[T]) Restore(ctx context.Context, id uuid.UUID, options ...OperationOption) error {
	return r.mutateDeletion(ctx, id, lifecycle.OperationRestore, options)
}

func (r *Repository[T]) ForceDelete(ctx context.Context, id uuid.UUID, options ...OperationOption) error {
	if err := r.ready(); err != nil {
		return err
	}
	if id == uuid.Nil {
		return soroerrors.InvalidArgument("id is required")
	}
	settings := newOperationSettings(options)
	return r.db.Transaction(ctx, func(txContext context.Context) error {
		entity, err := r.find(txContext, id, withDeleted)
		if err != nil {
			return err
		}
		lifecycleContext := &lifecycle.Context{
			Operation: lifecycle.OperationForceDelete, Tx: r.db.IDB(txContext),
			ActorID: auth.ActorID(txContext), Metadata: settings.metadata,
		}
		if err := r.registerTerminalHooks(txContext, entity, lifecycleContext, settings); err != nil {
			return err
		}
		if err := r.run(txContext, lifecycle.BeforeForceDelete, entity, lifecycleContext, settings); err != nil {
			return err
		}
		result, err := r.db.IDB(txContext).NewDelete().Model(entity).Where("? = ?", bun.Ident("id"), id).Exec(txContext)
		if err != nil {
			return r.mapDatabaseError(err)
		}
		if err := ensureAffected(result, r.metadata.resourceName); err != nil {
			return err
		}
		return r.run(txContext, lifecycle.AfterForceDelete, entity, lifecycleContext, settings)
	})
}

func (r *Repository[T]) mutateDeletion(ctx context.Context, id uuid.UUID, operation lifecycle.Operation, options []OperationOption) error {
	if err := r.ready(); err != nil {
		return err
	}
	if id == uuid.Nil {
		return soroerrors.InvalidArgument("id is required")
	}
	settings := newOperationSettings(options)
	return r.db.Transaction(ctx, func(txContext context.Context) error {
		scope := withoutDeleted
		before, after := lifecycle.BeforeDelete, lifecycle.AfterDelete
		if operation == lifecycle.OperationRestore {
			scope = onlyDeleted
			before, after = lifecycle.BeforeRestore, lifecycle.AfterRestore
		}
		entity, err := r.find(txContext, id, scope)
		if err != nil {
			return err
		}
		persisted := *entity
		base := baseOf(entity)
		now := r.now()
		base.UpdatedAt = now
		base.UpdatedBy = auth.ActorID(txContext)
		if operation == lifecycle.OperationDelete {
			base.DeletedAt = &now
			base.DeletedBy = auth.ActorID(txContext)
		} else {
			base.DeletedAt = nil
			base.DeletedBy = nil
		}
		changes, err := lifecycle.Compare(&persisted, entity)
		if err != nil {
			return err
		}
		lifecycleContext := &lifecycle.Context{
			Operation: operation, Changes: changes, Tx: r.db.IDB(txContext),
			ActorID: cloneUUID(base.UpdatedBy), Metadata: settings.metadata,
		}
		if err := r.registerTerminalHooks(txContext, entity, lifecycleContext, settings); err != nil {
			return err
		}
		if err := r.run(txContext, before, entity, lifecycleContext, settings); err != nil {
			return err
		}
		query := r.db.IDB(txContext).NewUpdate().Model(entity).
			Column("deleted_at", "deleted_by", "updated_at", "updated_by").
			Where("? = ?", bun.Ident("id"), id)
		if operation == lifecycle.OperationDelete {
			query = query.Where("? IS NULL", bun.Ident("deleted_at"))
		} else {
			query = query.Where("? IS NOT NULL", bun.Ident("deleted_at"))
		}
		result, err := query.Exec(txContext)
		if err != nil {
			return r.mapDatabaseError(err)
		}
		if err := ensureAffected(result, r.metadata.resourceName); err != nil {
			return err
		}
		return r.run(txContext, after, entity, lifecycleContext, settings)
	})
}

func (r *Repository[T]) runUpdateBeforeStages(ctx context.Context, persisted, entity *T, lifecycleContext *lifecycle.Context, settings operationSettings) error {
	if err := r.run(ctx, lifecycle.BeforeValidate, entity, lifecycleContext, settings); err != nil {
		return err
	}
	if err := r.refreshChanges(persisted, entity, lifecycleContext); err != nil {
		return err
	}
	if err := r.db.Validator().Validate(ctx, entity); err != nil {
		return err
	}
	for _, stage := range []lifecycle.Stage{lifecycle.AfterValidate, lifecycle.BeforeSave, lifecycle.BeforeUpdate} {
		if err := r.run(ctx, stage, entity, lifecycleContext, settings); err != nil {
			return err
		}
		if err := r.refreshChanges(persisted, entity, lifecycleContext); err != nil {
			return err
		}
	}
	return nil
}

func (r *Repository[T]) refreshChanges(persisted, entity *T, lifecycleContext *lifecycle.Context) error {
	changes, err := lifecycle.Compare(persisted, entity)
	if err != nil {
		return err
	}
	lifecycleContext.Changes = changes
	return nil
}

func (r *Repository[T]) registerTerminalHooks(ctx context.Context, entity *T, lifecycleContext *lifecycle.Context, settings operationSettings) error {
	if !settings.skips(lifecycle.AfterCommit) {
		if err := r.db.AfterCommit(ctx, func(callbackContext context.Context) error {
			return r.db.Hooks().Run(callbackContext, lifecycle.AfterCommit, entity, lifecycleContext)
		}); err != nil {
			return err
		}
	}
	if !settings.skips(lifecycle.AfterRollback) {
		if err := r.db.AfterRollback(ctx, func(callbackContext context.Context) error {
			return r.db.Hooks().Run(callbackContext, lifecycle.AfterRollback, entity, lifecycleContext)
		}); err != nil {
			return err
		}
	}
	return nil
}

func (r *Repository[T]) run(ctx context.Context, stage lifecycle.Stage, entity *T, lifecycleContext *lifecycle.Context, settings operationSettings) error {
	if settings.skips(stage) {
		return nil
	}
	return r.db.Hooks().Run(ctx, stage, entity, lifecycleContext)
}

func (r *Repository[T]) find(ctx context.Context, id uuid.UUID, scope deletionScope) (*T, error) {
	entity := new(T)
	query := r.queryForScope(ctx, scope).Model(entity).Where("? = ?", bun.Ident("id"), id).Limit(1)
	if err := query.Scan(ctx); err != nil {
		return nil, r.mapReadError(err)
	}
	return entity, nil
}

func (r *Repository[T]) scopedQuery(ctx context.Context) *bun.SelectQuery {
	return r.queryForScope(ctx, r.scope)
}

func (r *Repository[T]) queryForScope(ctx context.Context, scope deletionScope) *bun.SelectQuery {
	query := r.db.IDB(ctx).NewSelect().Model((*T)(nil))
	switch scope {
	case withoutDeleted:
		return query.Where("? IS NULL", bun.Ident("deleted_at"))
	case onlyDeleted:
		return query.Where("? IS NOT NULL", bun.Ident("deleted_at"))
	default:
		return query
	}
}

func (r *Repository[T]) updateColumns() []string {
	columns := append([]string(nil), r.metadata.updateColumns...)
	sort.Strings(columns)
	return columns
}

func (r *Repository[T]) ready() error {
	if r == nil {
		return soroerrors.InvalidArgument("repository is nil")
	}
	if r.initErr != nil {
		return soroerrors.InvalidArgument(r.initErr.Error())
	}
	return nil
}

func (r *Repository[T]) validateEntity(entity *T) error {
	if err := r.ready(); err != nil {
		return err
	}
	if entity == nil {
		return soroerrors.InvalidArgument("entity is required")
	}
	return nil
}

func (r *Repository[T]) mapReadError(err error) error {
	if errors.Is(err, sql.ErrNoRows) {
		return soroerrors.NotFound(r.metadata.resourceName)
	}
	return r.mapDatabaseError(err)
}

func (r *Repository[T]) mapDatabaseError(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return err
	}
	var postgresError *pgconn.PgError
	if errors.As(err, &postgresError) && postgresError.Code == "23505" {
		return soroerrors.Conflict(r.metadata.resourceName + " conflicts with an existing record").WithCause(err)
	}
	var frameworkError *soroerrors.Error
	if errors.As(err, &frameworkError) {
		return err
	}
	return soroerrors.Internal(err)
}

func (r *Repository[T]) now() time.Time { return r.clock().UTC() }

func baseOf[T any](entity *T) *model.Base { return any(entity).(model.Entity).SoroBase() }

func cloneUUID(id *uuid.UUID) *uuid.UUID {
	if id == nil {
		return nil
	}
	copy := *id
	return &copy
}

func copyImmutableBase(destination, source *model.Base) {
	destination.ID = source.ID
	destination.CreatedAt = source.CreatedAt
	destination.CreatedBy = cloneUUID(source.CreatedBy)
	destination.DeletedAt = source.DeletedAt
	destination.DeletedBy = cloneUUID(source.DeletedBy)
}

func ensureAffected(result sql.Result, resource string) error {
	affected, err := result.RowsAffected()
	if err != nil {
		return soroerrors.Internal(err)
	}
	if affected == 0 {
		return soroerrors.NotFound(resource)
	}
	return nil
}
