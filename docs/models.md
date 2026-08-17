# Models

Normal Soro models are ordinary Go structs embedding `model.Base`:

```go
type User struct {
    model.Base
    AccountID *uuid.UUID `bun:"account_id,nullzero" json:"account_id,omitempty"`
    Email     string     `bun:"email,notnull" validate:"required,email"`
    Active    bool       `bun:"active,notnull,default:true"`
}
```

`Base` supplies UUID identity, name, description, JSONB metadata, create/update
timestamps, soft-delete state, and nullable actor IDs. Repositories generate
UUIDv7 values in the application before validation and insert. Timestamps are
UTC and controlled by the repository clock, which can be replaced in tests.

Soro does not put persistence methods on models. Use a typed repository so the
model remains usable by validation, serialization, jobs, and service code
without hidden database access.

## Metadata

`model.Metadata` is `map[string]any` with JSONB database support and normal JSON
serialization:

```go
metadata := model.Metadata{}
metadata.Set("plan", "enterprise")
metadata.Set("active", true)

plan, err := metadata.GetString("plan")
active, err := metadata.GetBool("active")
```

Typed getters return errors for missing keys and wrong types; they never
silently coerce values. `GetInt` accepts integral Go/JSON numeric forms while
rejecting fractions and overflow.

## Relationships

Relationships stay explicit as UUID columns:

```go
type Project struct {
    model.Base
    AccountID uuid.UUID  `bun:"account_id,notnull"`
    OwnerID   *uuid.UUID `bun:"owner_id,nullzero"`
}
```

Define foreign keys and indexes in PostgreSQL migrations. Use repository/Bun
queries to load related records. Soro does not introduce lazy-loading model
methods, which keeps query count and transaction context visible.

## Soft deletion and uniqueness

Every generated table has `deleted_at`, and normal repositories exclude rows
where it is non-null. Unique values use partial indexes:

```sql
CREATE UNIQUE INDEX users_email_unique
ON users (email)
WHERE deleted_at IS NULL;
```

This permits a value to be reused after its prior row is soft-deleted while
still preventing two live rows from sharing it.
