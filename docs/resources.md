# CRUD resources

A resource binds one repository to independent create input, update input, and response types:

```go
users, err := api.NewResource(api.ResourceConfig[
	User, CreateUserInput, UpdateUserInput, UserResponse,
]{
	Name:       "Users",
	Repository: repository.New[User](app.DB),
	Serializer: userSerializer,
	CreateEntity: func(ctx context.Context, input CreateUserInput) (*User, error) {
		return &User{Base: model.Base{Name: input.Name}, Email: input.Email}, nil
	},
	UpdateEntity: func(ctx context.Context, user *User, input UpdateUserInput) error {
		if input.Name != nil {
			user.Name = *input.Name
		}
		return nil
	},
	Query: userQuery,
})
```

Registering it at `/users` creates index, show, create, update, and destroy operations. Destroy performs a repository soft delete. Soro never generates restore or force-delete HTTP endpoints.

Create and update input structs use Huma field tags and JSON Schema validation. Resource mapping functions are explicit; Soro does not reflectively copy an input into a model. Model validation and all Phase 1 lifecycle hooks still run inside repository persistence.

Use `Disabled` to omit standard operations:

```go
Disabled: []api.Action{api.Destroy},
```

`Authorize`, `Before`, and `After` callbacks receive the context, action, and current entity. For index authorization the entity is nil. Mutating callbacks, mapping, and persistence share one transaction. `Scope` can add application-owned Bun predicates to index queries. `ModifyOperation` can customize the Huma operation for a specific action.

Errors returned by handlers and callbacks should use the typed constructors in `github.com/ruby-dev/soro/errors`. They become the standard envelope without exposing internal causes:

```json
{
  "error": {
    "code": "not_found",
    "message": "User not found",
    "request_id": "..."
  }
}
```
