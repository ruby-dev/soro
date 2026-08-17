# Jobs

Soro uses River for durable PostgreSQL-backed jobs while preserving River's normal argument and worker model.

Define arguments with a stable kind:

```go
type SendWelcomeEmail struct {
	UserID uuid.UUID `json:"user_id" river:"unique"`
}

func (SendWelcomeEmail) Kind() string { return "send_welcome_email" }
```

Register a typed handler before starting workers:

```go
err := jobs.Register(app.Jobs, func(ctx context.Context, args SendWelcomeEmail) error {
	user, err := users.Find(ctx, args.UserID)
	if err != nil {
		return err
	}
	return sendWelcome(ctx, user)
})
```

The function closure captures explicit dependencies. The jobs package does not import the root application container, and job argument structs do not need persistence methods.

Apply River's official migrations during deployment or application setup:

```go
if err := app.Jobs.Migrate(ctx); err != nil {
	return err
}
```

Enqueue with Soro options:

```go
result, err := app.Jobs.Enqueue(ctx, SendWelcomeEmail{UserID: user.ID},
	jobs.Queue("mailers"),
	jobs.Delay(5*time.Minute),
	jobs.Priority(2),
	jobs.MaxAttempts(10),
	jobs.Unique(jobs.UniqueConfig{ByArgs: true, ByQueue: true}),
)
```

River priorities are 1 through 4, with 1 highest. Invalid options fail before SQL execution. `result.Duplicate` reports when uniqueness prevented insertion.

## Transactions

`Enqueue` detects the active Soro transaction in `ctx` and automatically calls River's transactional insertion. Application writes and the job row commit or roll back together:

```go
err := users.Transaction(ctx, func(txCtx context.Context, txUsers *repository.Repository[User]) error {
	if err := txUsers.Create(txCtx, user); err != nil {
		return err
	}
	_, err := app.Jobs.EnqueueTx(txCtx, SendWelcomeEmail{UserID: user.ID})
	return err
})
```

`EnqueueTx` fails outside a Soro transaction. A committed job cannot be worked before the outer transaction commits.

## Workers and telemetry

`App.Serve` starts workers when `jobs.enabled` is true and stops them during graceful shutdown. A standalone worker process can call `app.Jobs.Start(ctx)` and `Stop(ctx)` directly. Phase 4 will expose this through `soro jobs work`.

Soro propagates W3C trace context in River metadata. Worker logs and spans contain job kind, queue, duration, attempts, and outcome. Metric labels never include job IDs or argument values.
