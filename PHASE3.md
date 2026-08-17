# Soro Phase 3: Application Services

Status: implemented

Phase 3 adds production application infrastructure around the Phase 1 persistence and Phase 2 HTTP foundations: River-backed jobs, transaction-safe enqueueing, mail and delayed delivery, structured logging, OpenTelemetry traces and metrics, Prometheus scraping, health/readiness, and graceful server lifecycle management.

## Outcomes

Phase 3 is complete when an application can:

- enqueue typed jobs normally or in the current Soro transaction;
- start and gracefully stop River workers;
- send mail immediately or enqueue it for delivery after commit;
- expose `/health`, `/ready`, and `/metrics`;
- emit bounded-cardinality HTTP, job, and mail metrics;
- propagate trace context through HTTP and queued jobs;
- log requests, jobs, and mail through `slog` without secrets;
- run an `http.Server` with timeouts and graceful shutdown through `App.Serve`.

## Packages and dependency direction

| Package | Responsibility | May depend on |
| --- | --- | --- |
| `observability` | OTel providers, OTLP/Prometheus exporters, HTTP/job/mail instruments | OTel, Prometheus, `slog` |
| `health` | Named readiness checks and JSON HTTP endpoints | standard library |
| `jobs` | River driver/client, options, typed worker registration, migration, telemetry | River, `database`, `observability` |
| `mail` | Messages, templates, transports, delivery, queued delivery worker | `jobs`, `observability` |
| `api` | Accept composable middleware; retain request IDs and recovery | existing API dependencies |
| root `soro` | Construct services, mount infrastructure endpoints, coordinate start/stop | all Phase 3 public packages |

`jobs` does not import the root application package. Generic worker registration accepts an ordinary typed handler function, normally a closure over explicit application services. This avoids a root-package cycle while preserving dependency injection and test replacement.

`mail` may depend on `jobs`; `jobs` never depends on `mail`. Observability and health do not depend on application or persistence packages.

## PostgreSQL and River transaction model

Soro uses River v0.43 through `riverdatabasesql`, which is built for `database/sql` ORM interoperability. It receives the same `*sql.DB` facade already owned by Bun, so no connection pool is duplicated.

Bun's `bun.Tx` embeds the `*sql.Tx` created by Soro's transaction coordinator. `database.DB.SQLTx(ctx)` exposes that transaction only when the context carries the matching active Soro transaction. `jobs.Client.EnqueueTx` passes it to River's `InsertTx`; the job row and application writes therefore commit or roll back atomically. The job cannot become visible before commit.

`Enqueue` outside a transaction uses River's normal insertion. When called inside a Soro transaction it automatically joins that transaction, making the safe behavior the default. `EnqueueTx` remains available when code wants to require transactional context explicitly.

River migrations are applied through its official migrator and database/sql driver. Soro does not copy or reinterpret River's migration SQL.

## Job contract and registration

Job arguments implement River's stable `Kind() string` contract and serialize through normal JSON. Soro wraps insertion options:

```go
app.Jobs.Enqueue(ctx, SendWelcomeEmail{UserID: user.ID},
	jobs.Queue("mailers"),
	jobs.Delay(5*time.Minute),
	jobs.Priority(2),
	jobs.UniqueByArgs(),
)
```

Typed workers are registered without reflection:

```go
err := jobs.Register(app.Jobs, func(ctx context.Context, args SendWelcomeEmail) error {
	return sendWelcome(ctx, app.DB, app.Mailer, args.UserID)
})
```

Go does not allow generic methods, so registration is a package function. The handler closure makes dependencies explicit and avoids placing framework persistence methods on job argument structs.

Soro records job kind, queue, attempts, duration, and outcome. Job kind and queue are bounded deployment-defined values; IDs and argument values are never metric labels.

## Mail model

`mail.Message` contains sender, recipients, CC, BCC, subject, text, and HTML. Transport is a small interface:

```go
type Transport interface {
	Send(context.Context, *Message) error
}
```

Initial transports are SMTP, console, and in-memory capture for development/tests. SMTP uses a context-aware network dial, STARTTLS when configured, and redacts credentials from errors/logging.

Templates use the standard `html/template` and `text/template` packages. `mail.Delivery` binds a message to a client and provides `Send` and `SendLater`. Delayed delivery inserts a serializable internal mail job; the mail worker sends through the configured transport.

## Observability

`observability.Provider` owns an OTel tracer provider and meter provider. It always exposes a Prometheus handler through an instance-owned registry. When an OTLP HTTP endpoint is configured, it also installs batch trace export and periodic metric export. Providers are passed explicitly to instrumentation; Soro does not require uncontrolled global OTel providers.

HTTP middleware extracts W3C trace context, creates server spans, records status and duration, and writes one structured request log. Metrics use method and status only; request paths, user IDs, account IDs, job IDs, email addresses, and arbitrary errors are not labels.

Standard exported Prometheus names include:

- `soro_http_requests_total`
- `soro_http_request_duration_seconds`
- `soro_jobs_processed_total`
- `soro_jobs_failed_total`
- `soro_job_duration_seconds`
- `soro_mail_sent_total`
- `soro_mail_failed_total`
- `soro_mail_duration_seconds`

Database spans and duration metrics use a Bun query hook. It records only a conservative operation class such as `SELECT`, `INSERT`, or `OTHER`; SQL text and bind values are never attached to spans, metrics, or logs.

## Health and readiness

`/health` reports that the process and HTTP handler are alive without touching dependencies. `/ready` runs registered checks under a short timeout. The default readiness check pings PostgreSQL. Responses are stable JSON and return 503 when any required dependency fails. Readiness does not run migrations or expensive table scans.

`/metrics` serves the instance-owned Prometheus registry and is not included under the versioned API prefix.

## Application lifecycle

`App.Serve(ctx)` creates an `http.Server` from typed timeout configuration, starts River workers when enabled, and shuts the server down when the context is cancelled. `App.Close()` is idempotent and stops jobs before flushing observability and closing PostgreSQL. Shutdown has a configured deadline and joins independent errors.

Applications may replace jobs, mail, observability, health, API, logger, or database through options for deterministic tests.

## Configuration

Phase 3 adds typed `jobs`, `mail`, and `observability` sections plus HTTP read/write/idle/shutdown timeouts. Development defaults use console mail, disabled workers, local Prometheus metrics, and no OTLP network export. Production rejects missing SMTP settings when SMTP is selected and invalid/unsafe timeout or queue configuration.

Environment variables include:

- `SORO_JOBS_ENABLED`, `SORO_JOBS_DEFAULT_QUEUE`, `SORO_JOBS_WORKERS`
- `SORO_MAIL_TRANSPORT`, `SORO_MAIL_FROM`, `SMTP_HOST`, `SMTP_PORT`, `SMTP_USERNAME`, `SMTP_PASSWORD`
- `OTEL_EXPORTER_OTLP_ENDPOINT`, `SORO_OTEL_ENABLED`
- HTTP timeout and shutdown variables under `SORO_HTTP_*`

Secrets are loaded but never formatted into logs or public errors.

## Testing strategy

Unit tests cover configuration, option validation, typed registration, capture/console transports, SMTP message construction, templates, health status aggregation, metrics, trace context, logging, and idempotent shutdown.

PostgreSQL integration tests use isolated schemas and prove:

- River migrations apply;
- normal enqueue persists arguments/options;
- transactional enqueue commits with application data;
- rollback removes the job row;
- a registered worker executes and records success/failure telemetry;
- queued mail is invisible before commit and delivered after workers run;
- readiness reflects database availability.

The full gate runs PostgreSQL tests, aggregate coverage, formatting, vet, vulnerability scanning, and the amd64 CI race detector.

## Phase boundary

Cobra commands (`soro jobs work` included), generators, application scaffolding, seed commands, and CLI migration commands remain Phase 4. Phase 3 exposes the library APIs those commands will call without adding placeholder CLI packages.

## Implemented slices

1. Instance-owned OpenTelemetry providers, OTLP HTTP export, Prometheus metrics, safe Bun instrumentation, and structured HTTP middleware.
2. Health/readiness registries and infrastructure endpoints.
3. River database/sql integration, official migrations, typed handlers, options, trace propagation, and atomic Bun transaction enqueueing.
4. Message validation, standard-library templates, SMTP/console/capture transports, immediate delivery, and transaction-safe `SendLater`.
5. Application construction, service replacement options, graceful HTTP/job shutdown, and the working example.
6. Unit and isolated-schema PostgreSQL integration coverage plus public documentation.

## Selected dependencies and adjustments

- River v0.43.0 plus its `riverdatabasesql` driver and official migrator.
- OpenTelemetry Go v1.45.0 with OTLP HTTP exporters.
- OpenTelemetry Prometheus exporter v0.67.0 and Prometheus client v1.24.1.

The conceptual `Perform(ctx, app *soro.App)` job method was adjusted to `jobs.Register(client, typedHandler)`. Importing the root App from `jobs` would create a package cycle, while a typed closure keeps normal dependency injection and test replacement. River v0.43 priorities are 1 through 4, so Soro validates that supported range rather than documenting the conceptual priority 10 example.

OTLP export initially uses HTTP/protobuf only; applications can still replace the provider for another protocol. River migrations are explicit through `Jobs.Migrate` rather than silently changing production schemas during `soro.New`. Attachments remain an explicitly future mail capability as specified.
