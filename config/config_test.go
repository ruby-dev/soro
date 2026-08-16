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
		"SORO_ENV":                "test",
		"SORO_APP_NAME":           "environment",
		"SORO_DATABASE_MAX_CONNS": "7",
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
	if loaded.App.Name != "environment" || loaded.App.Environment != config.Test {
		t.Fatalf("unexpected app config: %+v", loaded.App)
	}
	if loaded.Database.MaxConns != 7 || loaded.Database.ConnectTimeout != 2*time.Second {
		t.Fatalf("unexpected database config: %+v", loaded.Database)
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

func write(t *testing.T, path, contents string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
		t.Fatal(err)
	}
}

func noEnvironment(string) (string, bool) { return "", false }
