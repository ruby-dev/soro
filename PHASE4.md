# Soro Phase 4: CLI and Generators

Status: implemented

Phase 4 turns the Phase 1–3 libraries into a convention-driven application workflow through the `soro` command. Generated source remains ordinary Go, migrations remain readable PostgreSQL SQL, and every generated application is expected to compile immediately.

## Outcomes

Phase 4 is complete when a developer can run:

```text
soro new example
cd example
soro generate resource User email:string:unique:index first_name:string last_name:string active:bool
soro db create
soro db migrate
soro server
```

and receive a compiling application with versioned CRUD routes and OpenAPI documentation.

## Package boundaries

| Package | Responsibility |
| --- | --- |
| `cmd/soro` | Minimal executable entry point |
| `cli` | Cobra command tree, process lifecycle, and injectable command dependencies |
| `generate` | Field grammar, naming, templates, conflict-safe file generation |
| `migrate` | Existing migration execution plus convention-based SQL migration loading |
| `database` | PostgreSQL database create/drop administration |

The CLI depends on public framework packages. Framework packages never import the CLI or generator packages. Generator output imports public Soro APIs and does not depend on generator internals.

## Command model

The root command is `soro`. Runtime commands load configuration from the current application directory and honor `SORO_ENV` and normal environment overrides.

- `soro new NAME` scaffolds a module, configuration, server registration, migration registry, and application directories.
- `soro server` runs the generated application entry point through `go run ./cmd/server`.
- `soro routes` asks the application-specific command entry point to print its registered routes.
- `soro generate ...` writes model, resource, migration, serializer, validator, job, or mailer source.
- `soro db ...` performs PostgreSQL administration and SQL migrations.
- `soro jobs work` runs the application-specific worker process.
- `soro openapi generate` asks the application-specific entry point to write OpenAPI JSON.
- `soro version` prints the CLI version.

Application-specific route, worker-registration, and OpenAPI state cannot be discovered by a globally installed binary through runtime package loading. Generated applications therefore contain a small `cmd/app` control binary. Framework commands delegate to that explicit Go entry point for application-owned behavior. This keeps normal imports and avoids Go plugins, reflection registries, or package globals.

## Generator grammar

Fields use `name:type[:option...]`. Initial types are `string`, `text`, `bool`, `uuid`, `int`, `float`, `time`, and `json`. Supported options are `index`, `unique`, `null`, and `default=VALUE`. Invalid names, duplicate fields, unknown types, conflicting options, and unsupported defaults fail before files are written.

Names are converted deterministically between exported Go identifiers, snake case columns, and plural snake case table/resource paths. Generated identifiers are validated so client input cannot create arbitrary paths or malformed Go.

## File safety

Generators create files atomically and refuse to overwrite existing paths unless an explicit force flag is provided. Resource generation validates its entire output set before writing any file. Templates are embedded in the generator package and formatted with `go/format` before persistence.

## SQL migration format

Generated migrations live in `db/migrations` and use timestamp-prefixed names:

```sql
-- +soro Up
CREATE TABLE users (...);

-- +soro Down
DROP TABLE users;
```

The loader rejects missing markers, empty directions, duplicate names, malformed SQL splitting, and unsorted/duplicate migration files. Statements are executed by the existing transactional migrator. A unique field is emitted as a PostgreSQL partial unique index with `WHERE deleted_at IS NULL`; ordinary indexes are emitted separately. UUID primary keys, base fields, JSONB metadata, and `TIMESTAMPTZ` timestamps are always present.

## Database administration

`db create` and `db drop` parse `DATABASE_URL`, connect to PostgreSQL's maintenance database, and quote the resolved database identifier. They never interpolate an unvalidated URL fragment. Drop is explicit and refuses the maintenance databases. Migrate, rollback, and status use the configured application database and SQL migration directory.

River's schema is migrated alongside application migrations on `db migrate`. Application rollback affects only application migrations.

## Testing

- Unit tests cover command help, argument validation, naming, field parsing, SQL loading, templates, overwrite protection, and PostgreSQL administration query construction.
- Golden-style generator tests parse generated Go and verify migration properties.
- A generated application test runs `go test ./...` in a temporary module using a local `replace` directive for the Soro checkout.
- PostgreSQL integration tests generate and apply a resource migration, exercise partial uniqueness after soft deletion, inspect status, and roll it back.
- Final gates remain formatting, full PostgreSQL tests, aggregate coverage, vet, staticcheck, govulncheck, and the amd64 CI race detector.

## Phase boundary

Factories, richer test application bootstrapping, development mail UI, generator customization hooks, broad benchmarks, and API stabilization remain Phase 5. Phase 4 includes only the testing support necessary to prove generated output and commands work.

## Implemented slices

1. Cobra root command, executable, help, versioning, and injectable process/database dependencies.
2. Safe PostgreSQL create/drop plus runtime SQL migration discovery, splitting, status, rollback, and River migration integration.
3. Validated field grammar, deterministic naming, atomic conflict-safe writes, and PostgreSQL-aware migration rendering.
4. Application, model, resource, serializer, validator, migration, job, and mailer generation. Phase 5 additionally emits typed factories for models and resources.
5. Generated route/job registries and the application-owned control entry point for routes, workers, seeds, and OpenAPI.
6. Unit tests, generated-module compilation, and generated-resource PostgreSQL integration coverage.

## Selected dependency and adjustments

- Cobra v1.10.2 is the only new direct Phase 4 runtime dependency.
- SQL files are the discoverable CLI migration convention. The existing Go `migrate.Migration` API remains supported for application-compiled migrations.
- Application-owned behavior is exposed through generated `cmd/app` code rather than Go plugins or a reflection registry. This is the smallest explicit adjustment required by Go's static linking model.
- A source-development CLI cannot infer a releasable module version or locate its original checkout after installation. Before Soro's first tagged release, `soro new` therefore accepts `--soro-replace`; tagged builds use `--soro-version` or the release default.
- Route and OpenAPI inspection currently boots the application container, so PostgreSQL must be reachable. Decoupling schema-only API construction is a possible Phase 5 developer-experience improvement.
