# Soro Phase 5: Developer Experience and Stabilization

Status: in progress

Phase 5 turns the implemented framework into something an application team can
adopt, test, extend, and evaluate before the first tagged release. It does not
declare the public API stable. The work is deliberately split into coherent
slices so real usage can still change pre-v1 APIs.

## Outcomes

Phase 5 is complete when:

- application tests can boot a schema-isolated Soro app, make HTTP requests,
  run migrations, build and persist factory records, and inspect captured mail;
- job handlers can be exercised synchronously without starting River workers;
- development output is readable and secrets remain redacted;
- generators expose a deterministic customization seam without permitting
  templates to bypass path, formatting, or overwrite safety;
- representative repository, lifecycle, query, and HTTP paths have benchmark
  baselines;
- the example application demonstrates relationships, factories, seed data,
  jobs, mail, metadata, lifecycle changes, and resource querying;
- every public capability has compiling documentation and CI verifies examples;
- the exported API has been reviewed and a pre-v1 compatibility policy is
  documented.

## Package boundaries

| Package | Responsibility |
| --- | --- |
| `testutil` (package `sorotest`) | Test app, schema isolation, HTTP client, captured mail, migrations, and factory attachment |
| `factory` | Typed sequences, traits, builds, and optional persistence callbacks |
| `jobs` | Existing River adapter plus explicit synchronous typed-handler execution for tests |
| `generate` | Safe generated-file transformation after rendering and before formatting/writing |
| `observability` / root `soro` | Environment-aware development logging and existing bounded telemetry |

`testutil` may compose public Soro packages and the repository's internal
PostgreSQL test database helper. Runtime packages must not import `testutil` or
`factory`. The factory package is persistence-agnostic: applications explicitly
provide the create callback, so it does not reflect over repository types.

## Test application model

`sorotest.New(t)` opens a unique PostgreSQL schema through
`SORO_TEST_DATABASE_URL`, constructs a test-environment App with OTLP and River
workers disabled, installs capture mail, and owns an `httptest.Server`. Cleanup
is registered immediately and is safe when setup fails partway through.

The returned client resolves relative paths against the in-process server and
provides JSON request/response helpers without hiding `net/http`. Migrations are
ordinary `migrate.Migration` values. Captured messages are cloned before being
returned so tests cannot mutate transport state.

PostgreSQL remains required for integration tests. Unit tests continue to run
without external services and integration tests skip locally only when
`SORO_TEST_DATABASE_URL` is absent; CI always supplies it.

## Factory design

A factory is typed over the entity it builds:

```go
users := factory.New(
    func(sequence uint64) *User { /* defaults */ },
    func(ctx context.Context, user *User) error { return repo.Create(ctx, user) },
)
```

`Build` never touches the database. `Create` requires the explicit persistence
callback. Traits are ordinary `func(*T)` values applied in call order. Sequence
allocation is concurrency-safe and starts at one. Factory errors identify
whether construction or persistence failed without swallowing the underlying
cause.

## Synchronous jobs

Synchronous execution invokes the same typed handler registered with River but
does not insert a job row or simulate River retries. It is an explicit testing
operation for handler behavior. Queue insertion, uniqueness, scheduling, and
transactional visibility remain covered by PostgreSQL integration tests.

## Generator customization

Rendered files may pass through ordered typed transformers before Go formatting
and atomic persistence. A transformer receives the relative path and a copy of
the rendered bytes and returns replacement bytes. Soro still owns path
selection, duplicate detection, Go formatting, managed-file checks, and atomic
writes. Nil transformers and invalid transformed Go fail before any file is
written.

The CLI initially discovers application-owned transformations only through
normal Go use of the `generate` package. A declarative template language is not
introduced until real generator customization demonstrates a stable need.

## Benchmarks and stabilization

Benchmarks cover high-frequency pure paths on every host and PostgreSQL CRUD
when the integration database is available. Benchmark numbers are baselines,
not release gates, until variance is understood across CI hardware.

API review uses three rules:

1. exported names need an application-facing reason to exist;
2. underlying Go, Bun, Huma, River, and OpenTelemetry escape hatches remain
   available where Soro wraps them;
3. breaking changes remain allowed before v1 but must be documented in release
   notes and migration guidance.

## Implementation slices

1. **Implemented:** Test application, HTTP helpers, migrations, capture mail,
   typed factories, synchronous job handler execution, and tests.
2. **Implemented:** Development logging/mail ergonomics and seed/factory
   integration in generated resources and the basic application.
3. **Implemented:** Safe generator transformation hooks and generator
   customization tests.
4. **Implemented:** Benchmarks, complete documentation set, expanded
   Account/User/Project example, Compose PostgreSQL, and CI example verification.
5. Exported API audit, pre-v1 compatibility policy, release checklist, full
   formatting/test/race/vet/staticcheck/govulncheck gates, and status update.

## Phase boundary

Phase 5 does not add authentication, authorization policy engines, GraphQL,
Redis, an admin UI, a proprietary migration language, or API stability claims.
Those remain outside the initial release or require evidence from real Soro
applications.
