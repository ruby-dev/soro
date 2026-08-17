# Serialization

Soro keeps database models separate from public API responses:

```go
type UserResponse struct {
	ID    uuid.UUID `json:"id"`
	Name  string    `json:"name"`
	Email string    `json:"email"`
}

userSerializer := serializer.Func[User, UserResponse](
	func(ctx context.Context, user *User) (UserResponse, error) {
		return UserResponse{ID: user.ID, Name: user.Name, Email: user.Email}, nil
	},
)
```

The core contract is:

```go
type Serializer[T, R any] interface {
	Serialize(context.Context, *T) (R, error)
}
```

Resources use the serializer for create, show, update, and list responses. Fields absent from the response type—including metadata, actor IDs, and deletion state—are never exposed.

`serializer.Collection` preserves input order and serializes one item at a time. A serializer may implement `SerializeCollection(context.Context, []*T) ([]R, error)` to batch related data and prevent N+1 queries.

