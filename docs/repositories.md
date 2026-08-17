# Repositories

Construct repositories with a model type and the application database:

```go
users := repository.New[User](app.DB)
```

The pointer to `User` must implement `model.Entity`, normally by embedding
`model.Base`. Contract errors are returned by operations so construction stays
a conventional single-return expression.

## CRUD and scopes

```go
user := &User{Base: model.Base{Name: "Dustin"}, Email: "user@example.com"}
if err := users.Create(ctx, user); err != nil { return err }

found, err := users.Find(ctx, user.ID)
found.Email = "updated@example.com"
err = users.Update(ctx, found)
err = users.Delete(ctx, found.ID)       // soft delete

deleted, err := users.OnlyDeleted().Find(ctx, found.ID)
err = users.Restore(ctx, deleted.ID)
err = users.ForceDelete(ctx, deleted.ID) // physical delete
```

`WithDeleted` includes both live and deleted rows. `OnlyDeleted` includes only
deleted rows. Scope methods return copies and are safe to share.

Other common operations are `FindBy`, `First`, `All`, `Exists`, `Count`, and
`Save`. `FindBy` resolves Go field or Bun column names against model metadata;
unknown names are rejected rather than interpolated into SQL.

## Transactions

```go
err := users.Transaction(ctx, func(txCtx context.Context, txUsers *repository.Repository[User]) error {
    if err := txUsers.Create(txCtx, first); err != nil { return err }
    return txUsers.Create(txCtx, second)
})
```

Repository operations join a transaction in `txCtx`. Nested transactions join
the outer transaction and become rollback-only after any nested failure;
savepoints are not implemented. Transaction-safe River enqueueing uses the same
context and database transaction.

## Escape hatches

`Query(ctx)` returns a deletion-scoped `*bun.SelectQuery`. `DB(ctx)` returns the
current `bun.IDB`, which is the active Bun transaction when one exists. These
are normal escape hatches for advanced SQL; callers remain responsible for
parameterization and preserving framework soft-delete semantics where needed.
