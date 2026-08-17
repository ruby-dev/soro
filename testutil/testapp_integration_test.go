package sorotest_test

import (
	"fmt"
	"net/http"
	"testing"

	"github.com/ruby-dev/soro"
	"github.com/ruby-dev/soro/factory"
	"github.com/ruby-dev/soro/mail"
	"github.com/ruby-dev/soro/migrate"
	"github.com/ruby-dev/soro/model"
	"github.com/ruby-dev/soro/testutil"
)

type user struct {
	model.Base
	Email string `bun:"email,notnull"`
}

var migrations = []migrate.Migration{{
	Name: "001_users",
	Up: []string{`CREATE TABLE users (
id UUID PRIMARY KEY, name VARCHAR(255), description TEXT,
metadata JSONB NOT NULL DEFAULT '{}'::jsonb,
created_at TIMESTAMPTZ NOT NULL, updated_at TIMESTAMPTZ NOT NULL,
deleted_at TIMESTAMPTZ, created_by UUID, updated_by UUID, deleted_by UUID,
email TEXT NOT NULL)`},
	Down: []string{"DROP TABLE users"},
}}

func TestAppHTTPFactoryAndCapturedMail(t *testing.T) {
	app := sorotest.New(t, sorotest.WithMigrations(migrations...))
	response, err := app.Client().Request(t.Context(), http.MethodGet, "/health", nil)
	if err != nil {
		t.Fatal(err)
	}
	if response.StatusCode != http.StatusOK {
		t.Fatalf("health status = %d body=%s", response.StatusCode, response.Body)
	}

	users, err := sorotest.NewFactory(app, func(sequence uint64) *user {
		return &user{Base: model.Base{Name: fmt.Sprintf("User %d", sequence)}, Email: fmt.Sprintf("user-%d@example.com", sequence)}
	})
	if err != nil {
		t.Fatal(err)
	}
	inactive := factory.Trait[user](func(value *user) { value.Metadata = model.Metadata{"active": false} })
	created, err := users.Create(t.Context(), inactive)
	if err != nil {
		t.Fatal(err)
	}
	if created.ID.String() == "00000000-0000-0000-0000-000000000000" || created.Metadata["active"] != false {
		t.Fatalf("unexpected factory entity: %#v", created)
	}

	if err := app.Mailer.Send(t.Context(), &mail.Message{To: []string{created.Email}, Subject: "Welcome", Text: "Hello"}); err != nil {
		t.Fatal(err)
	}
	messages := app.Messages()
	if len(messages) != 1 || messages[0].To[0] != created.Email {
		t.Fatalf("captured messages = %#v", messages)
	}
	messages[0].Subject = "mutated"
	if app.Messages()[0].Subject != "Welcome" {
		t.Fatal("captured message state was mutated by caller")
	}
	app.ResetMail()
	if len(app.Messages()) != 0 {
		t.Fatal("expected captured mail reset")
	}
}

func TestSetupCallback(t *testing.T) {
	called := false
	app := sorotest.New(t, sorotest.Setup(func(application *soro.App) error {
		called = application != nil
		return nil
	}))
	_ = app
	if !called {
		t.Fatal("expected setup callback")
	}
}
