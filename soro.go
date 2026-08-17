// Package soro provides the application container for the Soro framework.
package soro

import (
	"context"
	"fmt"
	"log/slog"
	"os"

	"github.com/datasoro/soro/api"
	"github.com/datasoro/soro/config"
	"github.com/datasoro/soro/database"
)

type App struct {
	Config *config.Config
	DB     *database.DB
	API    *api.API
	Logger *slog.Logger
}

type appSettings struct {
	config *config.Config
	db     *database.DB
	api    *api.API
	logger *slog.Logger
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
		settings.logger = slog.New(slog.NewTextHandler(os.Stderr, nil))
	}
	if settings.db == nil {
		opened, err := database.Open(ctx, settings.config.Database)
		if err != nil {
			return nil, fmt.Errorf("initialize Soro: %w", err)
		}
		settings.db = opened
	}
	if settings.api == nil {
		httpAPI, err := api.New(api.Config{
			Title: settings.config.App.Name, Version: "0.0.0",
			BasePath: settings.config.HTTP.APIBasePath, MaxBodyBytes: settings.config.HTTP.MaxRequestBody,
		}, api.WithLogger(settings.logger))
		if err != nil {
			_ = settings.db.Close()
			return nil, fmt.Errorf("initialize Soro API: %w", err)
		}
		settings.api = httpAPI
	}
	return &App{Config: settings.config, DB: settings.db, API: settings.api, Logger: settings.logger}, nil
}

func (app *App) Close() error {
	if app == nil || app.DB == nil {
		return nil
	}
	return app.DB.Close()
}
