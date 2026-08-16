# Soro

Soro is an opinionated Go application framework for building production REST APIs quickly, combining convention-driven development with idiomatic Go.

Soro is developed by DataSoro. Its developer experience is inspired by the productivity of Rails API applications, while its implementation keeps normal Go structs, interfaces, generics, `context.Context`, explicit dependencies, PostgreSQL, and Bun available to application code.

> Status: pre-release. Phase 1 is implemented; the public API is not stable.

## Phase 1

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

HTTP resources, Huma, serialization, River jobs, mail, observability, and CLI generators belong to later phases.

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

## Configuration

Configuration precedence is:

```text
framework defaults
config/application.yaml
config/{SORO_ENV}.yaml
environment variables
```

Supported Phase 1 variables include `SORO_ENV`, `SORO_APP_NAME`, `DATABASE_URL`, `SORO_DATABASE_MIN_CONNS`, `SORO_DATABASE_MAX_CONNS`, and the documented duration fields in `config.Config`. Unknown YAML fields fail startup. Production requires `DATABASE_URL`.

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

The example applies its migration, creates and updates a user, soft-deletes it, finds it through `OnlyDeleted`, restores it, and explicitly force-deletes it.

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

## Design documents

- [Phase 1 plan](PHASE1.md)
- [Architecture](ARCHITECTURE.md)

## License

Apache License 2.0. See [LICENSE](LICENSE).
