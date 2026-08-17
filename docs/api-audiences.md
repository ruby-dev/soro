# API audiences

Every typed Soro operation declares exactly one audience. Audience describes
who may build a client; it does not replace authentication, authorization,
tenant isolation, rate limiting, or auditing.

## Audience levels

- `first_party` is internal control-plane access. It requires normal principal
  authorization with the declared scopes and an independent software-client
  credential for a named client audience.
- `second_party` is elevated access for vetted external customers or partners.
  It requires at least one explicitly declared trusted-client scope.
- `third_party` is the public developer surface. It requires every declared
  normal scope. An empty scope list explicitly marks an anonymous public
  operation.

Soro checks normal scopes before first-party client proof. A valid client key,
signature, or certificate can never grant a user permission by itself.

## Application authorizer

Applications connect their authentication system at startup:

```go
type AudienceAuthorizer struct{}

func (AudienceAuthorizer) RequireScopes(
    ctx context.Context,
    audience api.Audience,
    scopes []string,
) error {
    // Read the normal authenticated principal from ctx and require every scope.
    return nil
}

func (AudienceAuthorizer) AuthenticateClient(
    ctx context.Context,
    request *http.Request,
    clientAudience string,
) (context.Context, error) {
    // Verify independent HMAC, mTLS, private_key_jwt, or controlled client-key
    // proof and return a context containing the software-client identity.
    return ctx, nil
}

app, err := soro.New(ctx,
    soro.WithAudienceAuthorizer(AudienceAuthorizer{}),
)
```

The application owns credential storage, rotation, revocation, replay
protection, and principal scope semantics. For networked first-party clients,
prefer signed requests, mTLS, or asymmetric client assertions over a reusable
raw secret. If HMAC is used, sign the method, exact request target, timestamp,
nonce, and body digest; enforce a short time window and persist nonce use.

## Marking operations

Custom operations must be marked before registration:

```go
operation := huma.Operation{
    Method:      http.MethodDelete,
    Path:        "/system/tenants/{id}",
    OperationID: "archive-system-tenant",
}
api.WithAudience(api.FirstParty(
    "control_plane",
    "global.tenants.admin",
))(&operation)

if err := api.Register(router, operation, handler); err != nil {
    return err
}
```

Second-party and third-party examples:

```go
api.WithAudience(api.SecondParty("trusted.reports.read"))(&operation)
api.WithAudience(api.ThirdParty("projects.read"))(&operation)
api.WithAudience(api.ThirdParty())(&publicLoginOperation)
```

CRUD resources can set one policy for every action and override selected
actions:

```go
Audience: api.ThirdParty("users.read"),
Audiences: map[api.Action]api.AudiencePolicy{
    api.Create:  api.ThirdParty("users.write"),
    api.Update:  api.ThirdParty("users.write"),
    api.Destroy: api.FirstParty("control_plane", "global.users.admin"),
},
```

Protected operations fail registration when no audience authorizer is
configured. Unclassified custom operations also fail registration. Generated
resources are explicitly classified as third-party unless application code
overrides them.

## Contract and runtime metadata

Soro publishes:

- `x-soro-audience` on every OpenAPI operation;
- `x-soro-required-scopes` when scopes are required;
- `x-soro-client-audience` for first-party software-client proof;
- `X-Soro-API-Audience` on operation responses;
- audience, scopes, and client audience through `app.API.Routes()`.

Applications may additionally define concrete OpenAPI security schemes. A
first-party operation should express normal principal authentication and
software-client authentication as one OpenAPI security requirement object so
the schemes are ANDed rather than presented as alternatives.
