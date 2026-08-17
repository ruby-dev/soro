# Routing

`soro.App` creates an `api.API` backed by Huma and `http.ServeMux`. Register versioned routes during application startup:

```go
err := app.API.Version("v1", func(v1 *api.Router) {
	if err := v1.Resource("/users", users); err != nil {
		panic(err)
	}
})
```

The default prefix is `/api`, so the resource is mounted at `/api/v1/users`. Version names accept lower-case letters, digits, `_`, and `-`, and must begin with a letter. Resource paths must begin with `/` and cannot have a trailing slash.

Custom typed operations use `api.Register`, since Go does not support generic methods:

```go
type MeOutput struct {
	Body struct {
		ID uuid.UUID `json:"id"`
	}
}

operation := huma.Operation{
	Method:      http.MethodGet,
	Path:        "/users/me",
	OperationID: "get-current-user",
	Tags:        []string{"Users"},
}
api.WithAudience(api.ThirdParty("users.profile.read"))(&operation)
err := api.Register(v1, operation, func(ctx context.Context, input *struct{}) (*MeOutput, error) {
	// Return a typed output or a Soro error.
	return output, nil
})
```

Every custom operation must declare a first-, second-, or third-party audience.
Protected audiences are enforced through the application's configured audience
authorizer. See [API audiences](api-audiences.md).

Huma serves OpenAPI JSON and YAML at `/openapi.json` and `/openapi.yaml`, schemas below `/schemas`, and interactive documentation at `/docs`. `app.API.Routes()` returns a copy of registered method, path, operation ID, tags, audience, scopes, and client-audience metadata. `Huma()` and `Mux()` are escape hatches for advanced integration.

Serve `app.API.Handler()`, not the raw mux, to retain request IDs and panic recovery. Request IDs are server-generated, available through `api.RequestID(ctx)`, and returned in `X-Request-ID`.
