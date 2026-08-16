// Package soro provides the application container for the Soro framework.
package soro

import (
	"context"
	"fmt"
	"log/slog"
	"os"

	"github.com/datasoro/soro/config"
	"github.com/datasoro/soro/database"
)

type App struct {
	Config *config.Config
	DB     *database.DB
	Logger *slog.Logger
}

type appSettings struct {
	config *config.Config
	db     *database.DB
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
	return &App{Config: settings.config, DB: settings.db, Logger: settings.logger}, nil
}

func (app *App) Close() error {
	if app == nil || app.DB == nil {
		return nil
	}
	return app.DB.Close()
}
