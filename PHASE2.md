# Soro Phase 2: HTTP/API

Status: implemented

Phase 2 exposes Phase 1 repositories as typed REST resources through Huma without weakening Soro's explicit persistence, validation, transaction, or serialization boundaries. It includes routing, API versioning, serializers, CRUD resources, pagination, filtering, searching, sorting, OpenAPI, request IDs, and the standard error envelope. It does not implement Phase 3 observability exporters, jobs, mail, or server lifecycle management.

## Outcomes

Phase 2 is complete when the example `User` model can be registered under `/api/v1/users` and provides:

```text
GET    /api/v1/users
GET    /api/v1/users/{id}
POST   /api/v1/users
PATCH  /api/v1/users/{id}
DELETE /api/v1/users/{id}
```

The list endpoint supports page/per-page metadata, explicit filters and operators, PostgreSQL `ILIKE` search, and explicit ascending/descending sorts. Every operation is present in generated OpenAPI. Database fields are never exposed unless the configured serializer includes them.

## Packages and dependency direction

| Package | Responsibility | May depend on |
| --- | --- | --- |
| `serializer` | Typed single-item and collection serialization, function adapters | standard library |
| `query` | Parse page/filter/search/sort parameters, validate field/operator allowlists, apply fixed Bun expressions | Bun, Soro errors |
| `api` | Huma/std-library adapter, version groups, request IDs, recovery, error mapping, route registry, generic CRUD resources | Huma, query, serializer, repository, Soro errors |
| root `soro` | Wire the API into `App` and expose it alongside Phase 1 services | `api`, existing foundation packages |
| `examples/basic` | Public response, create/update inputs, serializer, resource configuration, runnable HTTP example | public Soro APIs only |

`serializer` has no persistence dependency. `query` knows Bun but not repositories or HTTP adapters. `api` depends on the Phase 1 public surface; Phase 1 packages do not import `api`. Huma is wrapped rather than forked or reimplemented.

## Huma and router choice

Soro uses Huma v2 with its `humago` adapter and Go's `http.ServeMux`. This avoids an additional router dependency while retaining Huma's typed input/output processing, JSON Schema, OpenAPI 3.1, request validation, documentation UI, body-size enforcement, groups, and middleware.

`api.API` owns the mux and Huma API. `Handler()` returns the production middleware chain. `Huma()` and `Mux()` are explicit escape hatches for advanced routes. A small generic `api.Register` function supports custom typed Huma operations because Go does not permit generic methods.

## Versioning

`API.Version("v1", fn)` creates a Huma group at `/api/v1`. The version name must match a conservative identifier pattern and is used only after validation. Resource operation IDs are explicit and predictable (`list-users`, `get-user`, `create-user`, `update-user`, `delete-user`) rather than derived from Huma internals.

## Serializer contract

```go
type Serializer[T, R any] interface {
	Serialize(context.Context, *T) (R, error)
}
```

`serializer.Func` adapts ordinary functions. Collection serialization uses an optional batch interface when present and otherwise serializes in stable input order. Resource responses are `{"data": ...}` envelopes. Database models never become response bodies directly.

## Resource contract

Go needs all four types to keep input/output boundaries static:

```go
resource, err := api.NewResource(api.ResourceConfig[
	User,
	CreateUserInput,
	UpdateUserInput,
	UserResponse,
]{
	Name:       "Users",
	Repository: users,
	Serializer: userSerializer,
	CreateEntity: newUser,
	UpdateEntity: updateUser,
	Query:      userQuery,
})
```

The create mapper constructs a model from a typed input. The update mapper applies a typed patch to an already-loaded model. This is deliberate: Soro does not mass-assign JSON into persistence structs.

Resource customization includes disabled operations, authorization, before/after callbacks, custom query scopes, custom request types, custom serializers, operation modifiers, and Huma/router escape hatches. Mutating handlers run lookup, authorization, mapping, persistence, and post-behavior inside the repository transaction where relevant.

## Query definitions and SQL safety

Public query names map to immutable definitions:

```go
query.Field{
	Name:      "active",
	Column:    "active",
	Type:      query.Bool,
	Operators: []query.Operator{query.Eq, query.Neq},
}
```

Client values are parameters. Client field names never become SQL identifiers. Bun identifiers come only from validated resource definitions. Resource construction rejects unsafe or duplicate field/column definitions.

Supported operators are `eq`, `neq`, `gt`, `gte`, `lt`, `lte`, `in`, and `contains`. Types are string, boolean, integer, float, UUID, date, and timestamp. Operator/type combinations are validated at resource construction; invalid values become `invalid_query` errors before SQL execution.

`contains` performs literal case-insensitive containment by escaping `%`, `_`, and the escape character before using PostgreSQL `ILIKE`. Search uses the same escaping and builds one parenthesized OR group across fixed searchable columns.

Huma cannot statically declare arbitrary `filter[field][operator]` names in an input struct. The list input therefore uses a router-neutral Huma resolver to capture `Context.URL().Query()`, and Soro performs exhaustive allowlist validation in the handler. Unknown query parameters are rejected for resource list operations.

## Pagination

Defaults are page 1 and 25 records per page, with a configurable maximum of 100. Invalid or excessive values return `invalid_query`; Soro does not silently clamp explicit client input. The response includes page, per-page, total, and pages. The filtered count query is visible and is executed separately before the paged select. Query application is structured so a future cursor strategy can implement the same resource boundary.

## Error envelope

All Huma parsing/validation errors and all Soro handler errors use:

```json
{
  "error": {
    "code": "validation_failed",
    "message": "Validation failed",
    "request_id": "...",
    "fields": {}
  }
}
```

Core Soro codes map to fixed HTTP statuses. PostgreSQL/internal causes remain unwrap-able for logging but are never serialized. Huma exposes custom error construction as a process-wide function, not per API instance. Soro installs one controlled, idempotent process-wide factory that always returns the Soro envelope; this upstream boundary is documented rather than hidden.

## Request IDs and recovery

Every request receives a server-generated UUID request ID in context and the `X-Request-ID` response header. Phase 2 does not trust caller-supplied IDs. Recovery middleware catches panics, emits the standard internal envelope, and logs the request ID without including request bodies or secrets.

Huma's default 1 MiB body limit is retained unless the API/resource configuration explicitly chooses a smaller or larger positive limit.

## OpenAPI and route introspection

Huma serves `/openapi.json`, `/openapi.yaml`, schema documents, and `/docs`. Soro exposes the in-memory OpenAPI document and maintains a typed route registry containing method, path, operation ID, and tags. The registry is the Phase 2 source for tests and the later `soro routes` CLI command.

## Testing strategy

Unit tests cover:

- serializers and collection order/error propagation;
- query parsing for every operator/type;
- page and sort validation;
- rejection of unknown fields, operators, parameters, and unsafe definitions;
- SQL wildcard escaping;
- error code/status/field normalization;
- request ID and recovery middleware;
- version/path and operation ID generation.

PostgreSQL-backed HTTP integration tests use `httptest` with the existing isolated-schema helper and prove:

- all five CRUD routes and soft-delete behavior;
- typed request validation and normalized errors;
- serializers hide database-only fields;
- pagination totals/pages;
- boolean/UUID/date/operator filters;
- search and descending sort;
- unsupported query input cannot alter SQL;
- authorization and disabled operations;
- request IDs on success and failure;
- OpenAPI includes correct paths, schemas, methods, tags, and operation IDs.

The runnable example starts an `http.Server` only in the example command. Production server lifecycle and graceful shutdown remain Phase 3.

## Implemented slices

1. Serializer and query packages with exhaustive unit tests.
2. Huma API wrapper, version groups, request IDs, recovery, errors, and route registry.
3. Generic resources and PostgreSQL-backed HTTP tests.
4. Configuration/App wiring and the example HTTP application.
5. Public documentation and a 70% aggregate CI coverage floor.

## Phase boundary

River, mail, OpenTelemetry, Prometheus metrics, health/readiness, production server lifecycle, Cobra commands, and generators remain later phases. Phase 2 creates no placeholder APIs for them.
