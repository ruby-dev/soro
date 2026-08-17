# Soro Architecture

This document describes the implemented Phase 1 persistence foundation, Phase 2 HTTP/API layer, and Phase 3 application services. Future-phase designs are intentionally not presented as finished APIs.

## Dependency direction

The root `soro` package constructs the application. Persistence flows through a narrow set of packages:

```text
soro App
  ├─ config
  ├─ api ── Huma ── net/http
  │    ├─ resource[T, Create, Update, Response]
  │    ├─ query definitions
  │    └─ serializer contracts
  ├─ health / readiness
  ├─ observability ── OpenTelemetry ── OTLP / Prometheus
  ├─ jobs ── River ── shared database/sql facade
  ├─ mail ── SMTP / console / capture
  └─ database ── pgxpool ── PostgreSQL
       ├─ Bun facade
       ├─ lifecycle registry
       └─ validation engine
            ▲
            │
       repository[T]
         ├─ model.Base / Metadata
         ├─ auth principal context
         └─ typed errors
```

Lower packages never import the application container. Application models import only the primitives they use. There are no package globals controlling database, lifecycle, validation, or routing behavior. Huma's upstream error factory is process-wide; Soro installs its envelope factory once and otherwise keeps API state instance-owned.

## Application and database ownership

`soro.App` owns its configuration, structured logger, API, and `database.DB`. `database.DB` owns one `pgxpool.Pool`, builds a `database/sql` facade with `pgx/v5/stdlib.OpenDBFromPool`, and gives that facade to Bun. This avoids a second connection pool and leaves a pgx-native pool available for River in Phase 3.

`DB.Bun()` and `DB.Pool()` are deliberate escape hatches. Closing the App closes Bun's SQL facade and then the pgx pool. A database supplied with `soro.WithDatabase` is replacement-friendly for tests and is also owned by the App.

## HTTP and resource flow

`api.API` wraps Huma's standard-library adapter and owns its `http.ServeMux`. `API.Version` creates validated groups such as `/api/v1`; `api.Register` is the generic escape hatch for custom typed Huma handlers. `Handler` adds server-generated request IDs and panic recovery around the mux. Huma continues to own request decoding, JSON Schema validation, OpenAPI 3.1 generation, and documentation endpoints.

A generic resource has four independent types: persistence model, create input, update input, and public response. Explicit create/update functions cross the input-to-model boundary. A serializer crosses the model-to-response boundary. This prevents reflective mass assignment and accidental exposure of metadata, actor fields, or soft-delete state.

Mutating resource callbacks join one Soro transaction covering authorization, resource callbacks, repository lifecycle hooks, and persistence. `DELETE` invokes repository soft deletion; restore and force-delete remain explicit service operations and are not generated as HTTP routes.

List handlers parse router-neutral URL queries against a resource-owned definition, apply filters/search/sorts using fixed Bun identifiers, execute a visible count query, and then execute the paged select. Client field names never enter SQL. Definitions are validated at resource construction and unknown parameters are rejected at request time.

Every framework and Huma validation failure is rendered as the Soro error envelope. Internal errors remain available to server code but are replaced with a safe public message. The API maintains a route registry for inspection and the future CLI.

## Jobs and transactional enqueueing

River uses its `riverdatabasesql` driver against Bun's existing `database/sql` facade. Bun and River therefore share one pgx pool. The Soro transaction coordinator exposes the embedded `*sql.Tx` only through a matching transaction context; `jobs.EnqueueTx` passes that value to River's `InsertTx`. Repository writes and job insertion are one PostgreSQL transaction, and workers cannot see the job before commit.

Job argument structs retain River's explicit stable `Kind()` identifier and normal JSON representation. `jobs.Register` binds an argument type to a typed function closure, allowing applications to capture only their required dependencies without importing the root application from the jobs package. Soro wraps River insertion options and lifecycle but exposes River's durable implementation rather than reimplementing a queue.

Trace context is injected into River metadata and extracted by the typed worker. Job telemetry includes kind, queue, attempt, duration, and outcome; job IDs and argument values are excluded from metric labels.

## Mail delivery

Mail messages are transport-neutral values. The SMTP, console, and capture transports implement one `Send(context.Context, *Message)` interface. Standard-library text and HTML templates render into messages; HTML data is escaped by `html/template`.

Immediate delivery records a client span, bounded transport metric, and structured outcome log. `Delivery.SendLater` inserts an internal typed River job through the same job client, so calling it from a repository/resource transaction makes mail visibility atomic with application data. BCC recipients are passed to SMTP envelope commands and omitted from MIME headers.

## Observability and infrastructure endpoints

Each App owns its tracer provider, meter provider, propagator, and Prometheus registry. OTLP HTTP export is enabled only when configured; `/metrics` always serves the local registry. Soro passes providers directly instead of relying on process-global OTel state.

HTTP middleware extracts W3C trace context, creates server spans, records method/status metrics, and emits one `slog` record with request ID, duration, remote IP, and trace ID when available. Database instrumentation records only a fixed operation class and duration; it never records SQL text or bind values. Job and mail instrumentation follow the same bounded-label rule.

`/health` performs no dependency work. `/ready` runs registered checks under a deadline and includes PostgreSQL by default. `App.Serve` owns HTTP timeouts, starts enabled workers, and coordinates graceful shutdown. `App.Close` is idempotent and stops workers before flushing telemetry and closing PostgreSQL.

## Repository contract

`repository.New[T]` requires `T` to be a struct whose pointer implements `model.Entity`, which embedding `model.Base` provides. The constructor retains a contract error and normal operations return it as `invalid_argument`; this preserves the intended single-return construction:

```go
users := repository.New[User](app.DB)
```

Reflection is restricted to constructor-time field metadata and dirty snapshots. CRUD execution remains ordinary Bun queries against explicit `bun.IDB` values. Query column names supplied through `FindBy` are resolved through cached field metadata; unknown names are rejected. Repository deletion predicates and mutation columns are framework-owned, never derived from untrusted request strings.

`Query(ctx)` returns a scoped Bun select query. `DB(ctx)` returns the current Bun database or transaction, enabling advanced SQL without bypassing transaction context.

## Model state

`model.Base` defines the required UUID, names, description, JSONB metadata, timestamps, soft-delete timestamp, and actor UUIDs. Repositories generate UUIDv7 before validation. UUIDv7 gives hooks immediate access to identity and improves B-tree insertion locality over random UUIDv4.

Repository clocks and UUID sources are injectable. Create uses one UTC instant for `created_at` and `updated_at`. Mutations update `updated_at`; deletion also sets `deleted_at`, and restoration clears it. A principal placed in context through `auth.WithPrincipal` populates actor fields without coupling models to an identity provider.

`model.Metadata` is a natural JSON map and implements `driver.Valuer`/`sql.Scanner` for JSONB. Typed getters reject missing keys and incorrect types instead of coercing them.

## Transaction coordinator

The database transaction is carried in the callback's derived `context.Context`. Reads and writes resolve `bun.IDB` from that context. Repository mutations open a transaction when needed and join one when present.

Nested `Transaction` calls do not create a second transaction or savepoint. If a nested callback returns an error, shared state marks the outer transaction rollback-only. The outer boundary performs the actual commit or rollback and then invokes terminal callbacks in registration order.

`AfterCommit` callbacks run only after a successful outer commit. `AfterRollback` callbacks run after rollback, using `context.WithoutCancel` so cleanup/audit behavior still executes when request cancellation caused the failure while retaining context values. A commit error triggers a rollback attempt and rollback callbacks. An `AfterCommit` error is observable by the caller but cannot undo committed data.

## Lifecycle ordering

Each hook is a small optional interface. The registry also supports global and model-specific handlers with numeric priorities. Equal priorities retain registration order.

Before stages run:

```text
global → registered model handler → model method
```

After stages run:

```text
model method → registered model handler → global
```

Create and update use the exact ordering defined in `PHASE1.md`. Delete, restore, and force delete have distinct stages. Terminal hooks are registered before the first operation hook, so validation, hook, and SQL failures all take the rollback path. `SkipHooks` targets named stages; `SkipAllHooks` remains explicit and does not bypass validation.

## Dirty tracking

Update selects the persisted active row inside the transaction before invoking hooks. Immutable identity/creation/deletion fields are restored from that snapshot, framework update fields are applied, and `lifecycle.Compare` produces deep-copied old/new values keyed by Go field name. The repository refreshes changes after before-stage hooks, so after-hooks observe the values actually written.

Deletion and restoration also expose changes to framework-managed timestamp and actor fields. Map, slice, pointer, and interface values are cloned so later model mutations cannot rewrite recorded old state.

## Validation and errors

The application-owned validation engine supports a model's `Validate(context.Context)` method and declarative `validate` tags. Failures normalize into `errors.Error` with stable code `validation_failed` and field message lists. Database uniqueness violations map to `conflict`; missing rows map to `not_found`. Internal causes remain available through Go error unwrapping but are excluded from JSON.

## Migrations

Migrations contain ordered names plus readable `Up` and `Down` PostgreSQL statements. Each migration runs in one Soro transaction and is recorded in `soro_schema_migrations`. Statement slices are intentional: pgx's extended query protocol executes one statement at a time while the surrounding transaction preserves migration atomicity.

The example schema uses UUID, JSONB, TIMESTAMPTZ, and a partial unique index over active rows. Integration tests execute both migration directions against PostgreSQL rather than parsing SQL heuristically.

## Known Phase 1 limits

- Nested transactions join and become rollback-only; savepoints are not implemented.
- Bulk operations are not yet part of the typed repository API.
- Migration discovery and generation wait for the CLI phase; the runner itself is complete.
- Only concrete Phase 1 configuration sections are exported.
- Lifecycle changes use Go field names; a change also retains its Bun column name for audit integrations.
- APIs are pre-v1 and may change based on real application use.
