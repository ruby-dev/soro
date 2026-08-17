# Testing Soro applications

Soro separates unit tests from PostgreSQL integration tests. Pure validation,
serialization, query parsing, lifecycle registration, and factory construction
need no external service. Persistence and full application tests use a real
PostgreSQL database because Soro intentionally relies on PostgreSQL behavior.

## Test application

Import `github.com/ruby-dev/soro/testutil`; its package name is `sorotest`.
`sorotest.New` creates a unique schema inside `SORO_TEST_DATABASE_URL`, boots a
test-environment application, installs capture mail, and starts an in-process
HTTP server. The schema, server, App, and connection pool are cleaned up by the
test.

```go
func TestUsers(t *testing.T) {
    testapp := sorotest.New(t,
        sorotest.WithMigrations(application.Migrations...),
        sorotest.Setup(func(app *soro.App) error {
            return routes.Register(app)
        }),
    )

    response, err := testapp.Client().Request(
        t.Context(),
        http.MethodPost,
        "/api/v1/users",
        map[string]any{
            "name": "Dustin",
            "email": "dustin@example.com",
        },
    )
    if err != nil {
        t.Fatal(err)
    }
    if response.StatusCode != http.StatusCreated {
        t.Fatalf("status=%d body=%s", response.StatusCode, response.Body)
    }
}
```

`Client().HTTP()` exposes the underlying `*http.Client`, and `Client().URL`
resolves a local path when a test needs normal `net/http` behavior. Request
helpers reject external or scheme-relative paths so a typo cannot send test
payloads outside the in-process server.

## Typed factories

Factories have no hidden persistence. A builder receives a concurrency-safe,
one-based sequence. `Build` returns an in-memory entity; `Create` calls the
explicit persister.

```go
users, err := factory.New(
    func(sequence uint64) *User {
        return &User{
            Base:  model.Base{Name: fmt.Sprintf("User %d", sequence)},
            Email: fmt.Sprintf("user-%d@example.com", sequence),
        }
    },
    repository.New[User](app.DB).Create,
)
```

Inside a `sorotest` application, `NewFactory` binds the repository:

```go
users, err := sorotest.NewFactory(testapp, func(sequence uint64) *User {
    return &User{Email: fmt.Sprintf("user-%d@example.com", sequence)}
})

admin := factory.Trait[User](func(user *User) {
    user.Metadata.Set("role", "admin")
})

created, err := users.Create(t.Context(), admin)
```

Traits run in the order supplied. `BuildList` and `CreateList` apply the same
traits to each entity. List creation is not implicitly transactional; put the
factory persister or calling context inside an application transaction when
all-or-nothing behavior is required.

## Captured mail

The test app always uses `mail.CaptureTransport`:

```go
if err := testapp.Mailer.Send(t.Context(), message); err != nil {
    t.Fatal(err)
}

messages := testapp.Messages()
if len(messages) != 1 {
    t.Fatalf("messages=%d", len(messages))
}
testapp.ResetMail()
```

Messages are cloned on capture and retrieval, so assertions cannot mutate the
transport's stored state.

## Synchronous job handlers

Use `jobs.Perform` to test the same typed handler function registered with
River:

```go
handler := func(ctx context.Context, args SendWelcomeEmail) error {
    return service.SendWelcome(ctx, args.UserID)
}

if err := jobs.Perform(t.Context(), SendWelcomeEmail{UserID: id}, handler); err != nil {
    t.Fatal(err)
}
```

This deliberately tests handler behavior only. It does not insert a River row,
simulate retries, or test uniqueness and scheduling. Those behaviors require
the normal job client and PostgreSQL integration tests.

## Running tests

Unit tests:

```sh
go test ./...
```

All tests with PostgreSQL:

```sh
SORO_TEST_DATABASE_URL='postgres://postgres:postgres@localhost:5432/soro_test?sslmode=disable' \
  go test ./...
```

When the variable is absent, PostgreSQL tests skip locally. CI always supplies
it, so integration coverage cannot silently disappear there.

Race detection uses the same URL with `go test -race ./...`. Go's ARM64 race
runtime requires a compatible 48-bit virtual address layout; kernels exposing a
47-bit VMA fail before tests start with a ThreadSanitizer diagnostic. Do not
weaken host-wide ASLR settings to work around that. Soro's amd64 CI race job is
the required fallback for such hosts.

## Benchmarks

Pure lifecycle, query, and factory baselines run without services:

```sh
go test -run '^$' -bench . ./lifecycle ./query ./factory
```

Repository CRUD and in-process HTTP baselines require PostgreSQL:

```sh
SORO_TEST_DATABASE_URL='postgres://postgres:postgres@localhost:5432/soro_test?sslmode=disable' \
  go test -run '^$' -bench . ./repository ./testutil
```

Benchmarks report allocations and latency for comparison during development;
they are not CI pass/fail thresholds because host variance has not yet been
characterized.
