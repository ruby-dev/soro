# Mail

Messages are transport-neutral values:

```go
message := &mail.Message{
	From:    "support@example.com",
	To:      []string{"user@example.com"},
	Subject: "Welcome",
	Text:    "Welcome to the application.",
	HTML:    "<p>Welcome to the application.</p>",
}
```

When `From` is empty, the client uses `mail.from` from configuration. Messages require at least one recipient, a newline-free subject, and text or HTML content. BCC addresses are SMTP envelope recipients and are not written to MIME headers.

Send immediately or through River:

```go
err := app.Mailer.Delivery(message).Send(ctx)

result, err := app.Mailer.Delivery(message).SendLater(ctx,
	jobs.Delay(5*time.Minute),
	jobs.UniqueByArgs(),
)
```

`SendLater` uses the configured mail queue and inherits Soro's transactional enqueue behavior. When called from a resource or repository transaction, the mail job becomes visible only after commit.

## Templates

Templates use the Go standard library:

```go
templates, err := mail.ParseTemplates(
	"welcome",
	"Welcome, {{.Name}}",
	"Hello {{.Name}}",
	`<p>Hello <strong>{{.Name}}</strong></p>`,
)
subject, textBody, htmlBody, err := templates.Render(user)
```

Text uses `text/template`; HTML uses `html/template` and escapes untrusted values. Missing map keys are errors.

## Transports

- `SMTPTransport` supports implicit TLS or STARTTLS, optional authentication, context deadlines, and TLS 1.2 or newer.
- `ConsoleTransport` writes messages as JSON for development.
- `CaptureTransport` stores cloned messages safely for tests and exposes `Messages` and `Reset`.

Applications can replace the transport by implementing:

```go
type Transport interface {
	Send(context.Context, *mail.Message) error
}
```

Soro logs transport, duration, and outcome, but never recipients, message bodies, or SMTP credentials.
