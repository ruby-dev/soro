// Package sorotest provides schema-isolated Soro application test helpers.
package sorotest

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http/httptest"
	"testing"

	"github.com/ruby-dev/soro"
	"github.com/ruby-dev/soro/config"
	"github.com/ruby-dev/soro/factory"
	"github.com/ruby-dev/soro/internal/testdb"
	"github.com/ruby-dev/soro/mail"
	"github.com/ruby-dev/soro/migrate"
	"github.com/ruby-dev/soro/repository"
)

type settings struct {
	configure  []func(*config.Config)
	migrations []migrate.Migration
	setup      []func(*soro.App) error
}

type Option func(*settings) error

// Configure mutates the test configuration before validation and app startup.
func Configure(configure func(*config.Config)) Option {
	return func(settings *settings) error {
		if configure == nil {
			return fmt.Errorf("sorotest config callback is required")
		}
		settings.configure = append(settings.configure, configure)
		return nil
	}
}

// WithMigrations applies migrations before the test app is returned.
func WithMigrations(migrations ...migrate.Migration) Option {
	return func(settings *settings) error {
		settings.migrations = append(settings.migrations, migrations...)
		return nil
	}
}

// Setup runs after migrations and before the HTTP server starts. Applications
// normally register resources, custom routes, hooks, and job handlers here.
func Setup(callback func(*soro.App) error) Option {
	return func(settings *settings) error {
		if callback == nil {
			return fmt.Errorf("sorotest setup callback is required")
		}
		settings.setup = append(settings.setup, callback)
		return nil
	}
}

type App struct {
	*soro.App
	client  *Client
	capture *mail.CaptureTransport
}

// New boots a test-environment application in an isolated PostgreSQL schema.
// It skips the test when SORO_TEST_DATABASE_URL is not configured.
func New(t testing.TB, options ...Option) *App {
	t.Helper()
	configured := settings{}
	for _, option := range options {
		if option == nil {
			t.Fatal("sorotest option cannot be nil")
		}
		if err := option(&configured); err != nil {
			t.Fatal(err)
		}
	}

	db := testdb.Open(t)
	appConfig := config.Defaults()
	appConfig.App.Name = "soro-test"
	appConfig.App.Environment = config.Test
	appConfig.Jobs.Enabled = false
	appConfig.Mail.Transport = "capture"
	appConfig.Observability.Enabled = false
	for _, configure := range configured.configure {
		configure(&appConfig)
	}
	capture := mail.NewCaptureTransport()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	application, err := soro.New(t.Context(),
		soro.WithConfig(&appConfig),
		soro.WithDatabase(db),
		soro.WithLogger(logger),
		soro.WithMailTransport(capture),
	)
	if err != nil {
		t.Fatalf("boot sorotest application: %v", err)
	}
	if len(configured.migrations) > 0 {
		if err := migrate.New(application.DB).Apply(t.Context(), configured.migrations); err != nil {
			_ = application.Close()
			t.Fatalf("migrate sorotest application: %v", err)
		}
	}
	for _, setup := range configured.setup {
		if err := setup(application); err != nil {
			_ = application.Close()
			t.Fatalf("setup sorotest application: %v", err)
		}
	}
	server := httptest.NewServer(application.API.Handler())
	result := &App{
		App: application,
		client: &Client{
			baseURL: server.URL,
			http:    server.Client(),
		},
		capture: capture,
	}
	t.Cleanup(func() {
		server.Close()
		if err := application.Close(); err != nil {
			t.Errorf("close sorotest application: %v", err)
		}
	})
	return result
}

func (app *App) Client() *Client {
	if app == nil {
		return nil
	}
	return app.client
}

func (app *App) Messages() []*mail.Message {
	if app == nil || app.capture == nil {
		return nil
	}
	return app.capture.Messages()
}

func (app *App) ResetMail() {
	if app != nil && app.capture != nil {
		app.capture.Reset()
	}
}

// NewFactory binds a typed factory to the test app's repository for T.
func NewFactory[T any](app *App, builder factory.Builder[T], options ...repository.OperationOption) (*factory.Factory[T], error) {
	if app == nil || app.DB == nil {
		return nil, fmt.Errorf("sorotest app is required")
	}
	repo := repository.New[T](app.DB)
	return factory.New(builder, func(ctx context.Context, entity *T) error {
		return repo.Create(ctx, entity, options...)
	})
}
