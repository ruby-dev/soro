# Soro

Soro is an opinionated Go application framework for building production REST APIs quickly, combining convention-driven development with idiomatic Go.

Soro is developed by DataSoro. Its developer experience is inspired by the productivity of Rails API applications, while its implementation keeps normal Go structs, interfaces, generics, `context.Context`, explicit dependencies, PostgreSQL, and Bun available to application code.

> Status: pre-release. Phases 1 through 3 are implemented; the public API is not stable.

## Implemented foundation

The current foundation includes:

- a typed application container and strict layered configuration;
- one shared pgx pool bridged to Bun, ready to be shared with River later;
- UUIDv7 IDs, UTC timestamps, actor fields, and JSONB metadata;
- typed generic repositories and Bun escape hatches;
- transactional create, update, soft delete, restore, and force delete;
- joined nested transactions with outermost `AfterCommit`/`AfterRollback` behavior;
- all required optional model hooks plus deterministic global and registered hooks;
- persisted-state dirty tracking;
- contextual and declarative validation with normalized errors;
- readable PostgreSQL migrations and partial unique indexes;
- PostgreSQL integration tests and a compiling example.
- Huma-backed versioned routing, OpenAPI 3.1, and API documentation;
- typed serializers and generic REST resources with explicit input mappers;
- pagination, allowlisted filtering, literal `ILIKE` search, and sorting;
- standard error envelopes, server-generated request IDs, and safe panic recovery;
- resource authorization/callback/scope hooks and route introspection.
- River-backed typed jobs sharing Bun's pool and transaction;
- SMTP, console, and capture mail with transaction-safe `SendLater`;
- structured HTTP/job/mail logging and W3C trace propagation;
- OpenTelemetry tracing, OTLP HTTP export, and Prometheus metrics;
- `/health`, `/ready`, and `/metrics` infrastructure endpoints;
- configured HTTP timeouts and graceful server/worker shutdown.

CLI commands, generators, scaffolding, seed tooling, and developer-experience utilities belong to later phases.

## Requirements

- Go 1.26+
- PostgreSQL 17+ for integration tests and the example

The repository pins Go 1.26.6 through `mise.toml`.

## Model and repository

```go
type User struct {
	model.Base
	Email  string `bun:"email,notnull" validate:"required,email"`
	Active bool   `bun:"active,notnull,default:true"`
}

func (u *User) BeforeCreate(ctx context.Context, lc *lifecycle.Context) error {
	u.Email = strings.ToLower(strings.TrimSpace(u.Email))
	return nil
}

func (u *User) AfterUpdate(ctx context.Context, lc *lifecycle.Context) error {
	if lc.Changes.Changed("Email") {
		oldEmail, newEmail, _ := lc.Changes.Values("Email")
		_ = oldEmail
		_ = newEmail
	}
	return nil
}
```

```go
users := repository.New[User](app.DB)
user := &User{
	Base:  model.Base{Name: "Dustin"},
	Email: "USER@EXAMPLE.COM",
}

if err := users.Create(ctx, user); err != nil { /* handle */ }
found, err := users.Find(ctx, user.ID)
if err != nil { /* handle */ }
found.Active = true
if err := users.Update(ctx, found); err != nil { /* handle */ }
if err := users.Delete(ctx, found.ID); err != nil { /* soft delete */ }

deleted, err := users.OnlyDeleted().Find(ctx, found.ID)
if err != nil { /* handle */ }
if err := users.Restore(ctx, deleted.ID); err != nil { /* handle */ }
if err := users.ForceDelete(ctx, deleted.ID); err != nil { /* explicit physical delete */ }
```

Normal reads exclude deleted rows. `WithDeleted()` includes both states, and `OnlyDeleted()` returns deleted rows. Scope methods return repository copies and do not mutate shared state.

## HTTP resources

Application input, model, and response types remain separate. The compiling basic example configures a user resource with explicit mapping and serialization:

```go
users, err := basic.NewUserResource(repository.New[basic.User](app.DB))
if err != nil { /* handle */ }

err = app.API.Version("v1", func(v1 *api.Router) {
	if err := v1.Resource("/users", users); err != nil { /* handle */ }
})
```

This registers:

```text
GET    /api/v1/users
GET    /api/v1/users/{id}
POST   /api/v1/users
PATCH  /api/v1/users/{id}
DELETE /api/v1/users/{id}
```

`DELETE` is a soft delete. OpenAPI is served at `/openapi.json` and `/openapi.yaml`, with interactive documentation at `/docs`. List resources accept `page`, `per_page`, `search`, allowlisted `filter[...]` parameters, and `sort` fields configured by the resource.

## Transactions

Repository methods join a Soro transaction carried by the callback context:

```go
err := users.Transaction(ctx, func(txCtx context.Context, txUsers *repository.Repository[User]) error {
	if err := txUsers.Create(txCtx, user); err != nil {
		return err
	}
	return txUsers.Create(txCtx, anotherUser)
})
```

Nested calls join the outer SQL transaction. A nested error marks the outer transaction rollback-only, even if an intermediate callback catches it. Phase 1 does not implement savepoints. `AfterCommit` executes only after the outer commit succeeds; an error from it is returned after data has committed and cannot roll the transaction back.

## Jobs and mail

Job arguments use a stable kind and ordinary JSON fields:

```go
type SendWelcomeEmail struct {
	UserID uuid.UUID `json:"user_id" river:"unique"`
}

func (SendWelcomeEmail) Kind() string { return "send_welcome_email" }

err := jobs.Register(app.Jobs, func(ctx context.Context, args SendWelcomeEmail) error {
	return sendWelcome(ctx, args.UserID)
})
```

Enqueue normally or inside the current Soro transaction:

```go
_, err := app.Jobs.Enqueue(ctx, SendWelcomeEmail{UserID: user.ID},
	jobs.Queue("mailers"), jobs.Priority(2), jobs.UniqueByArgs())
```

When `ctx` carries a Soro transaction, `Enqueue` automatically uses River's transactional insertion. `EnqueueTx` is available when transactional context must be required explicitly.

Mail delivery is immediate or queued:

```go
delivery := app.Mailer.Delivery(&mail.Message{
	To: []string{user.Email}, Subject: "Welcome", Text: "Hello",
})
err = delivery.Send(ctx)
_, err = delivery.SendLater(ctx, jobs.Delay(5*time.Minute))
```

## Configuration

Configuration precedence is:

```text
framework defaults
config/application.yaml
config/{SORO_ENV}.yaml
environment variables
```

Supported variables include `SORO_ENV`, `SORO_APP_NAME`, `DATABASE_URL`, HTTP timeout variables, `SORO_JOBS_*`, `SORO_MAIL_*`, `SMTP_*`, `SORO_OTEL_ENABLED`, `OTEL_EXPORTER_OTLP_ENDPOINT`, and the database pool variables. Unknown YAML fields fail startup. Production requires `DATABASE_URL` and SMTP mail configuration. See [configuration](docs/configuration.md).

## Run the example

Start PostgreSQL using any local installation, or Docker when its daemon is available:

```sh
docker run --rm --name soro-postgres \
  -e POSTGRES_PASSWORD=postgres \
  -e POSTGRES_DB=soro \
  -p 5432:5432 postgres:17-alpine
```

In another shell:

```sh
export DATABASE_URL='postgres://postgres:postgres@localhost:5432/soro?sslmode=disable'
mise exec -- go run ./examples/basic/cmd/demo
```

The Phase 1 persistence demonstration applies its migration, creates and updates a user, soft-deletes it, restores it, and explicitly force-deletes it. Run the Phase 2 HTTP example instead with:

```sh
mise exec -- go run ./examples/basic/cmd/server
```

Then open `http://localhost:8080/docs` or call `http://localhost:8080/api/v1/users`.

Set `SORO_JOBS_ENABLED=true` to work the example's transactionally enqueued welcome-mail jobs in the server process. Production deployments can run the same library worker lifecycle from a dedicated process; the `soro jobs work` CLI arrives in Phase 4.

## Tests

Unit tests do not require external services. PostgreSQL integration tests use schema isolation and run when `SORO_TEST_DATABASE_URL` is present:

```sh
mise exec -- go test ./...

SORO_TEST_DATABASE_URL='postgres://postgres:postgres@localhost:5432/soro_test?sslmode=disable' \
  mise exec -- go test ./...

SORO_TEST_DATABASE_URL='postgres://postgres:postgres@localhost:5432/soro_test?sslmode=disable' \
  mise exec -- go test -race ./...
```

CI always supplies PostgreSQL, so integration tests cannot silently skip there.
CI also generates an aggregate coverage profile and enforces a 70% statement floor.

## Design documents

- [Phase 1 plan](PHASE1.md)
- [Phase 2 plan and implementation](PHASE2.md)
- [Phase 3 plan and implementation](PHASE3.md)
- [Architecture](ARCHITECTURE.md)
- [Routing](docs/routing.md)
- [Resources](docs/resources.md)
- [Querying](docs/querying.md)
- [Serialization](docs/serialization.md)
- [Configuration](docs/configuration.md)
- [Jobs](docs/jobs.md)
- [Mail](docs/mail.md)
- [Observability](docs/observability.md)

## License

Apache License 2.0. See [LICENSE](LICENSE).
