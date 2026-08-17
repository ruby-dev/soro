# CLI and generators

The `soro` executable is a Cobra command that operates on the application in the current directory.

## Installation

During source development:

```sh
go install ./cmd/soro
```

Release builds set the version displayed by `soro version`. Before the first tagged release, use `--soro-replace` when creating an application so its module points to a local Soro checkout.

## New applications

```sh
soro new customer-api \
  --module example.com/customer-api \
  --soro-replace /path/to/soro
```

`soro new` writes the application, configuration, server and control binaries, route/job registries, seeds, migration directory, and documentation. It then runs `go mod tidy`; a dependency resolution error makes the command fail instead of reporting a ready application.

The generated application uses these entry points:

- `cmd/server` boots the HTTP server.
- `cmd/app` exposes application-owned route, worker, seed, and OpenAPI behavior to the global CLI.
- `app/application.go` constructs the Soro App and registers generated components.

## Resource generation

```sh
soro generate resource User \
  email:string:unique:index \
  first_name:string \
  last_name:string \
  active:bool:default=true
```

This creates:

```text
app/models/user.go
app/models/user_test.go
app/serializers/user.go
app/validators/user.go
app/api/v1/user_resource.go
app/api/v1/user_resource_test.go
db/migrations/TIMESTAMP_create_users.sql
```

It also regenerates the owned v1 route registry. Existing user-owned files cause the operation to fail before writes begin. `--force` is available for intentional replacement.

Supported field types are `string`, `text`, `bool`, `uuid`, `int`, `float`, `time`, and `json`. Supported options are `index`, `unique`, `null`, and `default=VALUE`. Base fields such as `id`, `name`, `metadata`, and timestamps cannot be redeclared.

A unique field produces a partial PostgreSQL index:

```sql
CREATE UNIQUE INDEX users_email_unique
ON users (email)
WHERE deleted_at IS NULL;
```

If both `unique` and `index` are specified, only the unique index is emitted because it already supports indexed lookups.

Other component commands are:

```text
soro generate model NAME [FIELD...]
soro generate migration NAME
soro generate serializer NAME [FIELD...]
soro generate validator NAME [FIELD...]
soro generate job NAME
soro generate mailer NAME
```

Generated jobs are added to the application job registry. Generated mailers use Soro templates and return a normal `mail.Delivery` for immediate or queued sending.

## Database commands

```text
soro db create
soro db drop --force
soro db migrate
soro db rollback --count 1
soro db status
soro db seed
```

Migrations are loaded lexically from `db/migrations` and contain explicit sections:

```sql
-- +soro Up
CREATE TABLE examples (...);

-- +soro Down
DROP TABLE examples;
```

Application migrations are transactional. `db migrate` also applies River's official migrations. Rollback affects application migrations only. `db drop` is physically destructive, requires `--force`, and refuses PostgreSQL maintenance databases.

Edit `app/seeds/seeds.go` to insert application seed data. The generated function receives `context.Context` and `*soro.App`, so it can use ordinary repositories and transactions.

## Runtime and inspection

```text
soro server
soro routes
soro jobs work
soro openapi generate --output openapi.json
soro version
```

`server`, `routes`, `jobs work`, and OpenAPI generation use the generated application's normal Go entry points. Routes and OpenAPI currently boot the application, so configured dependencies must be reachable. OpenAPI generation refuses to overwrite an existing document unless `--force` is provided.
