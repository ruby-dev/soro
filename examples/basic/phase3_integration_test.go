package basic_test

import (
	"bytes"
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/datasoro/soro"
	"github.com/datasoro/soro/api"
	"github.com/datasoro/soro/config"
	"github.com/datasoro/soro/examples/basic"
	"github.com/datasoro/soro/internal/testdb"
	"github.com/datasoro/soro/mail"
	"github.com/datasoro/soro/migrate"
	"github.com/datasoro/soro/repository"
)

func TestCreateUserAtomicallyEnqueuesAndSendsWelcomeMail(t *testing.T) {
	db := testdb.Open(t)
	settings := config.Defaults()
	settings.App.Environment = config.Test
	settings.Database.URL = "injected"
	settings.Jobs.Enabled = true
	settings.Jobs.Workers = 1
	settings.Mail.Transport = "capture"
	capture := mail.NewCaptureTransport()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	app, err := soro.New(t.Context(), soro.WithConfig(&settings), soro.WithDatabase(db), soro.WithMailTransport(capture), soro.WithLogger(logger))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := app.Close(); err != nil {
			t.Error(err)
		}
	})
	if err := migrate.New(db).Apply(t.Context(), basic.Migrations); err != nil {
		t.Fatal(err)
	}
	if err := app.Jobs.Migrate(t.Context()); err != nil {
		t.Fatal(err)
	}
	users := repository.New[basic.User](db)
	if err := basic.RegisterJobs(app.Jobs, users, app.Mailer); err != nil {
		t.Fatal(err)
	}
	resource, err := basic.NewUserResourceWithJobs(users, app.Jobs)
	if err != nil {
		t.Fatal(err)
	}
	if err := app.API.Version("v1", func(v1 *api.Router) {
		if routeErr := v1.Resource("/users", resource); routeErr != nil {
			t.Fatal(routeErr)
		}
	}); err != nil {
		t.Fatal(err)
	}
	workerContext, cancel := context.WithCancel(t.Context())
	defer cancel()
	if err := app.Jobs.Start(workerContext); err != nil {
		t.Fatal(err)
	}

	request := httptest.NewRequest(http.MethodPost, "/api/v1/users", bytes.NewBufferString(`{"name":"Dustin","email":"dustin@example.com","active":true}`))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	app.API.Handler().ServeHTTP(response, request)
	if response.Code != http.StatusCreated {
		t.Fatalf("create status = %d body=%s", response.Code, response.Body.String())
	}

	deadline := time.Now().Add(5 * time.Second)
	for len(capture.Messages()) == 0 {
		if time.Now().After(deadline) {
			t.Fatal("welcome mail was not sent")
		}
		time.Sleep(25 * time.Millisecond)
	}
	message := capture.Messages()[0]
	if message.To[0] != "dustin@example.com" || message.Subject != "Welcome to Soro, Dustin" {
		t.Fatalf("welcome message = %+v", message)
	}
}
