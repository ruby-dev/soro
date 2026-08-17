package config_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/datasoro/soro/config"
)

func TestLoadPrecedence(t *testing.T) {
	directory := t.TempDir()
	write(t, filepath.Join(directory, "application.yaml"), `
app:
  name: application
database:
  max_conns: 20
  connect_timeout: 2s
`)
	write(t, filepath.Join(directory, "test.yaml"), `
app:
  name: test-file
database:
  max_conns: 15
`)
	environment := map[string]string{
		"SORO_ENV":                    "test",
		"SORO_APP_NAME":               "environment",
		"SORO_APP_VERSION":            "2.3.4",
		"SORO_HTTP_PORT":              "9090",
		"SORO_LOG_LEVEL":              "debug",
		"SORO_LOG_FORMAT":             "json",
		"SORO_JOBS_ENABLED":           "true",
		"SORO_JOBS_WORKERS":           "4",
		"SORO_MAIL_TRANSPORT":         "capture",
		"OTEL_EXPORTER_OTLP_ENDPOINT": "http://collector:4318",
		"SORO_DATABASE_MAX_CONNS":     "7",
	}
	loaded, err := config.Load(
		config.WithDirectory(directory),
		config.WithEnvLookup(func(key string) (string, bool) {
			value, ok := environment[key]
			return value, ok
		}),
	)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.App.Name != "environment" || loaded.App.Version != "2.3.4" || loaded.App.Environment != config.Test {
		t.Fatalf("unexpected app config: %+v", loaded.App)
	}
	if loaded.Database.MaxConns != 7 || loaded.Database.ConnectTimeout != 2*time.Second {
		t.Fatalf("unexpected database config: %+v", loaded.Database)
	}
	if loaded.HTTP.Port != 9090 {
		t.Fatalf("unexpected HTTP config: %+v", loaded.HTTP)
	}
	if loaded.Log.Level != "debug" || loaded.Log.Format != "json" {
		t.Fatalf("unexpected log config: %+v", loaded.Log)
	}
	if !loaded.Jobs.Enabled || loaded.Jobs.Workers != 4 || loaded.Mail.Transport != "capture" || loaded.Observability.OTLPEndpoint != "http://collector:4318" {
		t.Fatalf("unexpected Phase 3 config: jobs=%+v mail=%+v observability=%+v", loaded.Jobs, loaded.Mail, loaded.Observability)
	}
}

func TestLoggingConfigValidation(t *testing.T) {
	settings := config.Defaults()
	settings.Log.Level = "trace"
	if err := settings.Validate(); err == nil || !strings.Contains(err.Error(), "log.level") {
		t.Fatalf("expected log level error, got %v", err)
	}
	settings = config.Defaults()
	settings.Log.Format = "xml"
	if err := settings.Validate(); err == nil || !strings.Contains(err.Error(), "log.format") {
		t.Fatalf("expected log format error, got %v", err)
	}
}

func TestApplicationVersionIsRequired(t *testing.T) {
	settings := config.Defaults()
	settings.App.Version = ""
	if err := settings.Validate(); err == nil || !strings.Contains(err.Error(), "app.version") {
		t.Fatalf("expected app version error, got %v", err)
	}
}

func TestLoadRejectsUnknownYAML(t *testing.T) {
	directory := t.TempDir()
	write(t, filepath.Join(directory, "application.yaml"), "unknown: true\n")
	_, err := config.Load(config.WithDirectory(directory), config.WithEnvLookup(noEnvironment))
	if err == nil || !strings.Contains(err.Error(), "field unknown not found") {
		t.Fatalf("expected unknown field error, got %v", err)
	}
}

func TestProductionRequiresDatabaseURL(t *testing.T) {
	environment := func(key string) (string, bool) {
		if key == "SORO_ENV" {
			return "production", true
		}
		return "", false
	}
	_, err := config.Load(config.WithDirectory(t.TempDir()), config.WithEnvLookup(environment))
	if err == nil || !strings.Contains(err.Error(), "DATABASE_URL") {
		t.Fatalf("expected DATABASE_URL error, got %v", err)
	}
}

func TestProductionRejectsConsoleMail(t *testing.T) {
	settings := config.Defaults()
	settings.App.Environment = config.Production
	settings.Database.URL = "postgres://production"
	if err := settings.Validate(); err == nil || !strings.Contains(err.Error(), "mail transport") {
		t.Fatalf("expected production mail error, got %v", err)
	}
}

func write(t *testing.T, path, contents string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
		t.Fatal(err)
	}
}

func noEnvironment(string) (string, bool) { return "", false }
