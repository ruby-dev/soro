# Validation

Soro validation is independent of HTTP and runs inside repository create/update
lifecycles between `BeforeValidate` and `AfterValidate`.

Declarative tags use the wrapped validator engine:

```go
type CreateUserInput struct {
    Email string `json:"email" validate:"required,email"`
    Name  string `json:"name" validate:"required,max=255"`
}
```

Models or inputs can also implement contextual validation:

```go
func (input CreateUserInput) Validate(ctx context.Context) error {
    if reserved(input.Email) {
        return soroerrors.FieldError("email", "is reserved")
    }
    return nil
}
```

The central engine invokes contextual validation first, then declarative tags.
Return a Soro validation error to preserve field detail. Other contextual
errors become a `_base` validation message.

```go
if err := app.DB.Validator().Validate(ctx, input); err != nil {
    return err
}
```

Failures normalize to `errors.Error` with code `validation_failed`:

```json
{
  "error": {
    "code": "validation_failed",
    "message": "Validation failed",
    "fields": {
      "email": ["must be a valid email address"]
    }
  }
}
```

Huma also validates request schemas before handlers run. Soro renders those
failures through the same public envelope. Internal causes remain available to
server code through `errors.Is`/`errors.As` and are never serialized.
