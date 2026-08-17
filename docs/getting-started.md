# Getting started

Soro requires Go 1.26 or newer and PostgreSQL 17 or newer. During pre-release
development, install the CLI from a Soro checkout:

```sh
go install ./cmd/soro
```

Create an application and point its module at the local framework checkout:

```sh
soro new customer-api \
  --module example.com/customer-api \
  --soro-replace /path/to/soro
cd customer-api
```

Generate the first resource:

```sh
soro generate resource User \
  email:string:unique:index \
  first_name:string \
  last_name:string \
  active:bool:default=true
```

The generated model embeds `model.Base`; its migration uses UUID, JSONB,
`TIMESTAMPTZ`, soft deletion, and a partial unique email index. The resource
includes separate request/response types, a serializer, allowlisted queries, a
typed factory, tests, and v1 route registration.

Create `.env` from the example, then prepare and run the application:

```sh
cp .env.example .env
set -a; . ./.env; set +a
soro db create
soro db migrate
soro server
```

The generated endpoints are:

```text
GET    /api/v1/users
GET    /api/v1/users/{id}
POST   /api/v1/users
PATCH  /api/v1/users/{id}
DELETE /api/v1/users/{id}
```

Open `http://localhost:8080/docs` for interactive OpenAPI documentation.
`DELETE` soft-deletes. Restore and force-delete remain explicit repository or
service operations and are never generated as public routes.

Useful next commands:

```sh
soro routes
soro openapi generate
soro db status
soro db seed
soro jobs work
go test ./...
```

Before a tagged release, generated modules use the `replace` directive supplied
to `soro new`. Remove it and select a released Soro version when one exists.
