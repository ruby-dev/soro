# Soro Phase 1: Foundation

Status: implemented and verified

This phase establishes the persistence boundary that every later Soro feature will use. It intentionally excludes HTTP, Huma, River workers, mail delivery, telemetry exporters, and code generators. Phase 2 must not begin until the acceptance tests in this document pass.

## Outcomes

Phase 1 is complete when an application can load validated configuration, open one PostgreSQL `pgxpool.Pool`, expose that pool through Bun, migrate a schema, and persist a model through a typed repository with UUIDs, timestamps, metadata, lifecycle hooks, dirty tracking, transactions, and soft-delete semantics.

The public example must demonstrate create, find, update, delete, deleted scopes, restore, and force delete. All mutating operations are transactional.

## Packages and responsibilities

| Package | Responsibility | May depend on |
| --- | --- | --- |
| `errors` | Typed framework errors and validation field errors; no transport formatting policy beyond a serializable envelope | standard library |
| `model` | `Base`, `Metadata`, model identity/access contract, application-generated UUIDv7 initialization | `errors`, `google/uuid` |
| `auth` | Minimal principal-in-context contract used to populate actor fields; no authentication implementation | `google/uuid` |
| `lifecycle` | Operations, stages, changes, hook interfaces, deterministic registered/global hook engine | Bun interfaces, `google/uuid` |
| `validation` | Central validator interface, struct-tag adapter, normalized field errors | `errors`, go-playground validator |
| `config` | Typed defaults, YAML layering, environment overrides, startup validation | YAML library |
| `database` | pgx pool ownership, Bun bridge, transaction coordinator, commit/rollback callbacks, health/ping | `config`, Bun, pgx |
| `migrate` | Thin, readable SQL migration runner and schema migration ledger | `database`, Bun |
| `repository` | Generic CRUD, scopes, lifecycle orchestration, dirty tracking integration, actor stamping, Bun escape hatch | all foundation packages above |
| root `soro` | Application container, options, dependency wiring, ordered shutdown | foundation packages |
| `examples/basic` | Compiling User model and Phase 1 usage | public Soro APIs only |
| `internal/testdb` | Integration-test schema isolation and migration helpers | foundation packages |

Packages are created only when their Phase 1 responsibility has executable code. Dependency flow points inward toward small foundation packages; `model`, `lifecycle`, and `errors` never import `repository` or the application container. The root package wires dependencies and is not imported by lower layers.

## Important interfaces

Models embed `model.Base`. Its promoted `SoroBase() *model.Base` method is the explicit repository contract. `repository.New[T]` validates this contract once and returns an error for an invalid model type rather than relying on unbounded reflection in every operation.

Validation supports both:

```go
type Validator interface {
	Validate(context.Context) error
}
```

and declarative `validate` tags through the application-owned validation engine. Interface validation runs first, followed by field-tag validation when the first step succeeds. Both produce `errors.ValidationError` where possible.

Each lifecycle stage is a small optional model interface. External handlers use:

```go
type Handler func(context.Context, any, *lifecycle.Context) error
```

The lifecycle registry accepts global handlers and model-specific handlers. Registration records a numeric priority and a monotonic registration sequence. Lower priority numbers run first; equal priorities run in registration order.

## PostgreSQL and connection ownership

Soro creates one `pgxpool.Pool`. `pgx/v5/stdlib.OpenDBFromPool` creates the `database/sql` facade consumed by Bun while leaving pool lifecycle under Soro's control. This is the deliberate bridge for future River support: River can receive the same pgx pool without introducing a second PostgreSQL pool.

Shutdown order is Bun, the `database/sql` facade, then pgx pool. Closing the facade does not close the underlying pool. Soro exposes both `Bun()` and `Pool()` as explicit escape hatches.

## Transaction model

`database.DB.Transaction(ctx, fn)` installs a transaction state in the derived context passed to `fn`. Repository operations always resolve their executor from the context:

- outside a transaction, a mutating repository operation starts an outer transaction;
- inside a transaction, it joins the existing Bun transaction;
- reads join the transaction when one is present and otherwise use the Bun database;
- nested `Transaction` calls join rather than creating unrelated SQL transactions or savepoints;
- any nested callback error marks the outer transaction rollback-only, even if an intermediate caller catches it;
- commit and rollback callbacks belong to the outer transaction state.

`AfterCommit` runs only after the actual outer commit succeeds. `AfterRollback` runs after an actual rollback. If commit itself fails, rollback is attempted and rollback callbacks run. An `AfterCommit` error is returned to the caller but cannot undo the already committed data; this limitation is explicit in the API documentation.

Phase 1 intentionally supports joined nested transactions, not savepoints. Savepoints can be added later without changing repository operation signatures.

## Lifecycle engine

The operation pipeline is fixed:

- Create: begin, `BeforeValidate`, validate, `AfterValidate`, `BeforeSave`, `BeforeCreate`, insert, `AfterCreate`, `AfterSave`, commit, `AfterCommit`.
- Update: begin, load persisted row, compute changes, `BeforeValidate`, validate, `AfterValidate`, `BeforeSave`, `BeforeUpdate`, update, `AfterUpdate`, `AfterSave`, commit, `AfterCommit`.
- Delete: begin, load active row, `BeforeDelete`, update deletion fields, `AfterDelete`, commit, `AfterCommit`.
- Restore: begin, load deleted row, `BeforeRestore`, clear deletion fields, `AfterRestore`, commit, `AfterCommit`.
- Force delete: begin, load any row, `BeforeForceDelete`, physical delete, `AfterForceDelete`, commit, `AfterCommit`.

Within each before stage the order is global, model-registered, model-defined. Within each after stage it is model-defined, model-registered, global. The transaction stages use the same after ordering. Hook skipping names exact stages; the separate `SkipAllHooks` option is intentionally conspicuous.

Hook or validation errors before commit abort persistence. The rollback path invokes `AfterRollback` after rollback. A hook can use `Context.Tx`, `Operation`, `Changes`, `ActorID`, and operation-local metadata.

## Dirty tracking

Update starts by selecting the persisted row in the transaction. Soro compares that snapshot with the caller's requested entity using cached reflection metadata for exported Bun-backed fields, including embedded fields. It compares Go values with `reflect.DeepEqual`; time values are normalized to UTC and metadata is deep-copied to prevent aliasing.

Changes are addressed by Go field name (`Email`) and expose the corresponding Bun column name for auditing. `Changed`, `HasChanges`, `Fields`, `Values`, `Was`, and `Is` are read-only operations. The repository refreshes the change set after mutating before-hooks so after-hooks describe the values actually sent to PostgreSQL. Framework-managed `UpdatedAt` and actor fields are included only when their persisted values actually differ.

Update writes an explicit allowlist derived from model metadata, excluding primary key and immutable creation fields. This avoids accidentally changing identity while preserving ordinary Bun tags and the Bun query escape hatch.

## Repository generics

`Repository[T]` uses a value model parameter: `repository.New[User](db)`. Entity arguments and results are `*T`. The generic type removes CRUD duplication; reflection is limited to cached model metadata, base access validation, snapshotting, and update-column discovery.

Read scopes are immutable repository copies:

- default: `deleted_at IS NULL`;
- `WithDeleted()`: no deletion predicate;
- `OnlyDeleted()`: `deleted_at IS NOT NULL`.

`Delete` is always soft delete. `ForceDelete` is separately named and is never implied. `Query(ctx)` returns a Bun select query preconfigured with the repository's deletion scope. `DB(ctx)` exposes the current `bun.IDB` for advanced SQL. PostgreSQL constraint violations are mapped to stable Soro conflict errors without returning raw database details.

## Base model and UUID convention

`model.Base` owns the exact required fields and Bun column definitions. UUIDv7 values are generated in the application before `BeforeValidate`, making identity available to validation/hooks and deterministic in tests through an injectable repository clock/UUID source. UUIDv7 is chosen over random UUIDv4 for PostgreSQL index locality while retaining globally unique application-generated IDs.

Creation timestamps are produced from one UTC clock reading. Update and delete timestamps are application-generated from the same injectable clock. PostgreSQL columns still use `TIMESTAMPTZ` and migration defaults as a safety net for direct SQL.

`Metadata` implements natural JSON marshaling plus `sql.Scanner` and `driver.Valuer`. Typed getters return `(value, error)` on invalid stored types; they do not coerce silently.

## Migration strategy

Migrations are ordered `.sql` pairs represented as normal Go values (`Name`, `Up`, `Down`) and recorded in `soro_schema_migrations`. The runner applies each migration in a transaction and supports apply, rollback, and status. It rejects duplicate or out-of-order names.

The example migration is readable PostgreSQL SQL and includes UUID primary key, all base columns, JSONB default, timestamps, soft-delete fields, and a partial unique email index:

```sql
CREATE UNIQUE INDEX users_email_unique
ON users (email)
WHERE deleted_at IS NULL;
```

Migration validity and partial-unique behavior are integration-tested by executing the generated SQL against PostgreSQL. Phase 4 generators will render this same migration shape; Phase 1 does not introduce a proprietary schema DSL.

## Configuration

Precedence is defaults, `config/application.yaml`, `config/{SORO_ENV}.yaml`, then environment variables. Phase 1 implements the fields needed to open and tune PostgreSQL plus stable top-level sections reserved by the specification only when they have concrete validation. Unknown YAML keys fail loading. Production requires `DATABASE_URL`; all environments validate pool and timeout relationships.

## Testing strategy

Fast unit tests cover metadata access/type errors, change comparison, lifecycle registration order, validation normalization, typed errors, configuration precedence, transaction callback state, and repository model-contract failures.

PostgreSQL integration tests use `SORO_TEST_DATABASE_URL` when supplied. The repository CI job provides PostgreSQL as a service. Each test suite creates a unique schema, sets `search_path`, applies the example migration, and drops only that schema during cleanup. Local test documentation includes a plain `docker run` command because Docker Compose is not required for Phase 1.

Integration assertions cover:

- UUIDv7 generation, metadata JSONB round trips, and UTC timestamps;
- default, with-deleted, and only-deleted scopes;
- restore and physical force delete;
- exact lifecycle order and hook abort behavior;
- actual commit/rollback callback timing, including failed SQL and rollback-only nesting;
- persisted-old versus requested-new dirty values;
- partial unique index reuse after soft delete;
- canceled contexts reaching pgx/Bun;
- applying and rolling back valid PostgreSQL migrations.

Commands required before completion:

```text
go test ./...
go test -race ./...
go vet ./...
govulncheck ./...
```

Tests skip PostgreSQL-only cases with a clear message when no test URL is configured; CI never skips them.

## Implementation slices

1. Bootstrap module/toolchain, license, typed errors, Base/Metadata, and focused unit tests.
2. Add configuration and the shared pgx/Bun database object with transaction coordination tests.
3. Add lifecycle registry/interfaces, dirty tracking, validation, and unit tests.
4. Add generic repositories and PostgreSQL integration tests for all persistence semantics.
5. Add SQL migrations, example application/model, application container, and documentation.
6. Run formatting, unit/integration tests, race tests, vet, vulnerability scanning, and CI-equivalent checks; resolve all failures before declaring Phase 1 complete.

## Phase boundary

Huma routing, serializers, CRUD HTTP resources, pagination/filter/search/sort, River jobs, mail, OpenTelemetry, Prometheus, and the Cobra CLI remain future phases. Only seams proven necessary by Phase 1 are introduced now.
