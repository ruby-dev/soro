# API stability and compatibility

Soro is pre-v1. The current target is a useful `v0.1.0`, not a promise that the
public API is finished. Real applications are expected to reveal changes that
cannot be inferred from framework tests alone.

## Compatibility policy

- `v0.x` releases may contain breaking changes.
- Breaking changes must be called out in `CHANGELOG.md` with direct migration
  instructions when an automated migration is not possible.
- Patch releases should remain backward-compatible unless a security or data
  integrity issue makes that unsafe.
- Deprecations should remain for at least one minor release when retaining them
  does not compromise correctness or security.
- No `v1.0.0` release occurs until multiple real applications have exercised
  repositories, resources, transactions, jobs, mail, generators, and test
  utilities.

## Exported-surface audit

The Phase 5 audit retained these deliberate boundaries:

- `soro.App` exposes initialized dependencies and functional replacement
  options; it is the composition root rather than a service locator global.
- `database.DB` exposes Bun, pgxpool, and the shared `database/sql` facade.
- `api.API` and `api.Router` expose Huma for custom typed operations.
- `jobs.Client` wraps River insertion/lifecycle while job arguments remain River
  `JobArgs` values.
- repositories expose scoped Bun queries and the active Bun executor.
- `factory` remains persistence-agnostic; `testutil` is the only public package
  coupled to Go's `testing.TB`.
- generator transformers can alter content but cannot alter output paths or
  disable safety checks.

Internals for schema-isolated database setup, transaction state, repository
reflection metadata, job workers, and mail delivery jobs remain unexported.

The audit also removed the hard-coded `0.0.0` application version. `app.version`
or `SORO_APP_VERSION` now supplies OpenAPI and OpenTelemetry service versioning.

## Known pre-v1 decisions still open

- Whether nested transactions need savepoints instead of join/rollback-only.
- Whether cursor pagination joins the page/per-page API or ships as a separate
  resource configuration.
- Whether route/OpenAPI inspection should support schema-only application boot
  without PostgreSQL.
- Whether generator transformations need a declarative, application-owned
  configuration after real customization patterns emerge.

These are documented limits, not commitments to add complexity before evidence
requires it.
