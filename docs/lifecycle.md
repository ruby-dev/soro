# Lifecycle

Models implement only the hook interfaces they need:

```go
func (user *User) BeforeCreate(ctx context.Context, lc *lifecycle.Context) error {
    user.Email = strings.ToLower(strings.TrimSpace(user.Email))
    return nil
}

func (user *User) AfterUpdate(ctx context.Context, lc *lifecycle.Context) error {
    if lc.Changes.Changed("Email") {
        oldEmail, newEmail, _ := lc.Changes.Values("Email")
        _ = oldEmail
        _ = newEmail
    }
    return nil
}
```

The context exposes the operation, active Bun transaction, actor ID, operation
metadata, and persisted-old versus requested-new changes.

## Ordering

Create:

```text
Begin → BeforeValidate → Validate → AfterValidate → BeforeSave → BeforeCreate
→ INSERT → AfterCreate → AfterSave → Commit → AfterCommit
```

Update follows the same validation/save stages with `BeforeUpdate`, `UPDATE`,
and `AfterUpdate`. Delete, restore, and force-delete each have distinct before
and after stages. A failed operation rolls back and then invokes
`AfterRollback`; `AfterCommit` never runs before the actual outer commit.

For a before stage, ordering is global hook, registered model hook, then model
method. After stages reverse ownership: model method, registered hook, then
global hook. Numeric priorities sort registered/global hooks ascending; equal
priorities retain registration order.

```go
err := app.DB.Hooks().RegisterGlobal(lifecycle.AfterCommit, 100,
    func(ctx context.Context, entity any, lc *lifecycle.Context) error {
        return audit.Record(ctx, entity, lc)
    },
)
```

## Dirty tracking

Updates compare the row selected inside the transaction with the values that
will be persisted. `Changed`, `Fields`, `Values`, `Was`, `Is`, and `Change`
return cloned state so later mutations cannot rewrite audit history.

## Explicit skipping

```go
err := users.Create(ctx, user,
    repository.SkipHooks(lifecycle.AfterCreate, lifecycle.AfterCommit),
)
```

`SkipAllHooks` exists for repair/import operations and is intentionally named.
Validation still runs. `WithLifecycleMetadata` supplies operation-local values
to hooks without package globals.
