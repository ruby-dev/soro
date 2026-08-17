// Package soro provides the application container for the Soro framework.
package soro

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"sync"

	"github.com/datasoro/soro/api"
	"github.com/datasoro/soro/auth"
	"github.com/datasoro/soro/config"
	"github.com/datasoro/soro/database"
	"github.com/datasoro/soro/health"
	"github.com/datasoro/soro/jobs"
	"github.com/datasoro/soro/mail"
	"github.com/datasoro/soro/observability"
)

type App struct {
	Config        *config.Config
	DB            *database.DB
	API           *api.API
	Logger        *slog.Logger
	Jobs          *jobs.Client
	Mailer        *mail.Client
	Observability *observability.Provider
	Health        *health.Registry
	closeOnce     sync.Once
	closeErr      error
}

type appSettings struct {
	config        *config.Config
	db            *database.DB
	api           *api.API
	logger        *slog.Logger
	jobs          *jobs.Client
	mailer        *mail.Client
	observability *observability.Provider
	health        *health.Registry
	mailTransport mail.Transport
}

type Option func(*appSettings)

func WithConfig(settings *config.Config) Option {
	return func(app *appSettings) { app.config = settings }
}

// WithDatabase replaces database initialization. The App owns and closes db.
func WithDatabase(db *database.DB) Option {
	return func(app *appSettings) { app.db = db }
}

func WithLogger(logger *slog.Logger) Option {
	return func(app *appSettings) { app.logger = logger }
}

// WithAPI replaces the default HTTP API, primarily for tests and advanced setup.
func WithAPI(httpAPI *api.API) Option {
	return func(app *appSettings) { app.api = httpAPI }
}

func WithJobs(client *jobs.Client) Option { return func(app *appSettings) { app.jobs = client } }

func WithMailer(client *mail.Client) Option { return func(app *appSettings) { app.mailer = client } }

func WithMailTransport(transport mail.Transport) Option {
	return func(app *appSettings) { app.mailTransport = transport }
}

func WithObservability(provider *observability.Provider) Option {
	return func(app *appSettings) { app.observability = provider }
}

func WithHealth(registry *health.Registry) Option {
	return func(app *appSettings) { app.health = registry }
}

func New(ctx context.Context, options ...Option) (*App, error) {
	settings := appSettings{}
	for _, option := range options {
		option(&settings)
	}
	if settings.config == nil {
		loaded, err := config.Load()
		if err != nil {
			return nil, err
		}
		settings.config = loaded
	}
	if err := settings.config.Validate(); err != nil {
		return nil, err
	}
	if settings.logger == nil {
		logger, err := observability.NewLogger(observability.LoggerConfig{
			Writer: os.Stderr, Environment: settings.config.App.Environment,
			Level: settings.config.Log.Level, Format: settings.config.Log.Format,
		})
		if err != nil {
			return nil, fmt.Errorf("initialize Soro logger: %w", err)
		}
		settings.logger = logger
	}
	if settings.db == nil {
		opened, err := database.Open(ctx, settings.config.Database)
		if err != nil {
			return nil, fmt.Errorf("initialize Soro: %w", err)
		}
		settings.db = opened
	}
	cleanup := func() {
		if settings.observability != nil {
			_ = settings.observability.Shutdown(context.Background())
		}
		_ = settings.db.Close()
	}
	if settings.observability == nil {
		endpoint := ""
		if settings.config.Observability.Enabled {
			endpoint = settings.config.Observability.OTLPEndpoint
		}
		provider, err := observability.New(ctx, observability.Config{
			ServiceName: settings.config.App.Name, Environment: settings.config.App.Environment,
			Version: settings.config.App.Version, OTLPEndpoint: endpoint, OTLPTimeout: settings.config.Observability.OTLPTimeout,
		})
		if err != nil {
			cleanup()
			return nil, fmt.Errorf("initialize Soro observability: %w", err)
		}
		settings.observability = provider
	}
	settings.db.Bun().AddQueryHook(settings.observability.DatabaseHook())
	if settings.jobs == nil {
		queues := map[string]int{settings.config.Jobs.DefaultQueue: settings.config.Jobs.Workers}
		queues[settings.config.Mail.Queue] = settings.config.Jobs.Workers
		jobClient, err := jobs.New(settings.db, settings.observability, settings.logger, jobs.Config{
			WorkersEnabled: settings.config.Jobs.Enabled, DefaultQueue: settings.config.Jobs.DefaultQueue,
			Queues: queues, ShutdownTimeout: settings.config.Jobs.ShutdownTimeout,
		})
		if err != nil {
			cleanup()
			return nil, fmt.Errorf("initialize Soro jobs: %w", err)
		}
		settings.jobs = jobClient
	}
	if settings.mailer == nil {
		transport := settings.mailTransport
		if transport == nil {
			switch settings.config.Mail.Transport {
			case "console":
				transport = &mail.ConsoleTransport{Writer: os.Stdout}
			case "capture":
				transport = mail.NewCaptureTransport()
			case "smtp":
				var err error
				transport, err = mail.NewSMTPTransport(mail.SMTPConfig{
					Host: settings.config.Mail.SMTP.Host, Port: settings.config.Mail.SMTP.Port,
					Username: settings.config.Mail.SMTP.Username, Password: settings.config.Mail.SMTP.Password,
					StartTLS: settings.config.Mail.SMTP.StartTLS, ImplicitTLS: settings.config.Mail.SMTP.ImplicitTLS,
					InsecureSkipVerify: settings.config.Mail.SMTP.InsecureSkipVerify, Timeout: settings.config.Mail.SMTP.Timeout,
				})
				if err != nil {
					cleanup()
					return nil, fmt.Errorf("initialize Soro mail transport: %w", err)
				}
			}
		}
		mailer, err := mail.New(transport, settings.jobs, settings.observability, settings.logger, mail.Config{DefaultFrom: settings.config.Mail.From, Queue: settings.config.Mail.Queue})
		if err != nil {
			cleanup()
			return nil, fmt.Errorf("initialize Soro mail: %w", err)
		}
		settings.mailer = mailer
	}
	if settings.health == nil {
		registry, err := health.New(settings.config.HTTP.ReadinessTimeout)
		if err != nil {
			cleanup()
			return nil, fmt.Errorf("initialize Soro health: %w", err)
		}
		if err := registry.Register("database", settings.db.Ping); err != nil {
			cleanup()
			return nil, err
		}
		settings.health = registry
	}
	if settings.api == nil {
		httpAPI, err := api.New(api.Config{
			Title: settings.config.App.Name, Version: settings.config.App.Version,
			BasePath: settings.config.HTTP.APIBasePath, MaxBodyBytes: settings.config.HTTP.MaxRequestBody,
		}, api.WithLogger(settings.logger), api.WithMiddleware(settings.observability.HTTPMiddleware(observability.HTTPOptions{
			Logger: settings.logger, RequestID: api.RequestID,
			Fields: func(ctx context.Context) []slog.Attr {
				if actorID := auth.ActorID(ctx); actorID != nil {
					return []slog.Attr{slog.String("user_id", actorID.String())}
				}
				return nil
			},
		})))
		if err != nil {
			cleanup()
			return nil, fmt.Errorf("initialize Soro API: %w", err)
		}
		settings.api = httpAPI
	}
	settings.api.Mux().Handle("GET /health", settings.health.HealthHandler())
	settings.api.Mux().Handle("GET /ready", settings.health.ReadyHandler())
	settings.api.Mux().Handle("GET /metrics", settings.observability.Handler())
	return &App{
		Config: settings.config, DB: settings.db, API: settings.api, Logger: settings.logger,
		Jobs: settings.jobs, Mailer: settings.mailer, Observability: settings.observability, Health: settings.health,
	}, nil
}

func (app *App) Close() error {
	if app == nil {
		return nil
	}
	app.closeOnce.Do(func() {
		ctx, cancel := context.WithTimeout(context.Background(), app.Config.Jobs.ShutdownTimeout)
		defer cancel()
		app.closeErr = errors.Join(app.Jobs.Stop(ctx), app.Observability.Shutdown(ctx), app.DB.Close())
	})
	return app.closeErr
}

func (app *App) Serve(ctx context.Context) error {
	if app == nil || app.API == nil {
		return fmt.Errorf("soro app is not initialized")
	}
	if err := app.Jobs.Start(ctx); err != nil {
		return err
	}
	server := &http.Server{
		Addr: fmt.Sprintf(":%d", app.Config.HTTP.Port), Handler: app.API.Handler(),
		ReadHeaderTimeout: app.Config.HTTP.ReadHeaderTimeout, ReadTimeout: app.Config.HTTP.ReadTimeout,
		WriteTimeout: app.Config.HTTP.WriteTimeout, IdleTimeout: app.Config.HTTP.IdleTimeout,
	}
	serverErrors := make(chan error, 1)
	go func() { serverErrors <- server.ListenAndServe() }()
	select {
	case err := <-serverErrors:
		shutdownContext, cancel := context.WithTimeout(context.Background(), app.Config.Jobs.ShutdownTimeout)
		defer cancel()
		if errors.Is(err, http.ErrServerClosed) {
			err = nil
		}
		return errors.Join(err, app.Jobs.Stop(shutdownContext))
	case <-ctx.Done():
		shutdownContext, cancel := context.WithTimeout(context.WithoutCancel(ctx), app.Config.HTTP.ShutdownTimeout)
		defer cancel()
		shutdownErr := server.Shutdown(shutdownContext)
		jobsErr := app.Jobs.Stop(shutdownContext)
		serverErr := <-serverErrors
		if errors.Is(serverErr, http.ErrServerClosed) {
			serverErr = nil
		}
		return errors.Join(shutdownErr, jobsErr, serverErr)
	}
}
