# Changelog

Soro follows semantic versioning. Before v1, minor releases may contain
documented breaking changes as described in `docs/api-stability.md`.

## Unreleased

### Added

- Typed first-, second-, and third-party API audience policies with additive
  scope and software-client enforcement plus OpenAPI metadata.
- Phase 1 PostgreSQL persistence, transactions, lifecycle, validation, dirty
  tracking, metadata, and soft deletion.
- Phase 2 Huma REST resources, OpenAPI, serialization, pagination, filtering,
  search, sorting, versioning, and standard errors.
- Phase 3 River jobs, mail, structured logging, OpenTelemetry, Prometheus,
  health/readiness, and graceful lifecycle management.
- Phase 4 Cobra CLI, SQL migrations, application/component generators, database
  commands, workers, routes, and OpenAPI generation.
- Phase 5 test applications, typed factories, captured-mail assertions,
  synchronous handlers, safe generator customization, development logging,
  examples, benchmarks, and stabilization documentation.

### Compatibility

- No API stability guarantee is made before v1.0.0.
