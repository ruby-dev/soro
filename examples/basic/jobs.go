package basic

import (
	"context"

	"github.com/datasoro/soro/jobs"
	"github.com/datasoro/soro/mail"
	"github.com/datasoro/soro/repository"
	"github.com/google/uuid"
)

type SendWelcomeEmail struct {
	UserID uuid.UUID `json:"user_id" river:"unique"`
}

func (SendWelcomeEmail) Kind() string { return "send_welcome_email" }

func RegisterJobs(jobClient *jobs.Client, users *repository.Repository[User], mailer *mail.Client) error {
	templates, err := mail.ParseTemplates(
		"welcome", "Welcome to Soro, {{.Name}}", "Hello {{.Name}}, welcome to Soro.",
		`<p>Hello <strong>{{.Name}}</strong>, welcome to Soro.</p>`,
	)
	if err != nil {
		return err
	}
	return jobs.Register(jobClient, func(ctx context.Context, args SendWelcomeEmail) error {
		user, err := users.Find(ctx, args.UserID)
		if err != nil {
			return err
		}
		subject, textBody, htmlBody, err := templates.Render(user)
		if err != nil {
			return err
		}
		return mailer.Delivery(&mail.Message{
			To: []string{user.Email}, Subject: subject, Text: textBody, HTML: htmlBody,
		}).Send(ctx)
	})
}
