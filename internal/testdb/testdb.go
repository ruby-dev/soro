// Package testdb provides schema-isolated PostgreSQL databases to Soro tests.
package testdb

import (
	"context"
	"net/url"
	"os"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/ruby-dev/soro/config"
	"github.com/ruby-dev/soro/database"
)

func Open(t testing.TB) *database.DB {
	t.Helper()
	databaseURL := os.Getenv("SORO_TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("set SORO_TEST_DATABASE_URL to run PostgreSQL integration tests")
	}
	admin, err := pgx.Connect(t.Context(), databaseURL)
	if err != nil {
		t.Fatalf("connect test PostgreSQL: %v", err)
	}
	schema := "soro_test_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	if _, err := admin.Exec(t.Context(), "CREATE SCHEMA "+pgx.Identifier{schema}.Sanitize()); err != nil {
		admin.Close(context.Background())
		t.Fatalf("create test schema: %v", err)
	}

	isolatedURL, err := withSearchPath(databaseURL, schema)
	if err != nil {
		t.Fatal(err)
	}
	settings := config.Defaults().Database
	settings.URL = isolatedURL
	db, err := database.Open(t.Context(), settings)
	if err != nil {
		t.Fatalf("open isolated test database: %v", err)
	}
	t.Cleanup(func() {
		if err := db.Close(); err != nil {
			t.Errorf("close test database: %v", err)
		}
		if _, err := admin.Exec(context.Background(), "DROP SCHEMA "+pgx.Identifier{schema}.Sanitize()+" CASCADE"); err != nil {
			t.Errorf("drop test schema: %v", err)
		}
		if err := admin.Close(context.Background()); err != nil {
			t.Errorf("close test admin connection: %v", err)
		}
	})
	return db
}

func withSearchPath(databaseURL, schema string) (string, error) {
	parsed, err := url.Parse(databaseURL)
	if err != nil {
		return "", err
	}
	query := parsed.Query()
	query.Set("search_path", schema)
	parsed.RawQuery = query.Encode()
	return parsed.String(), nil
}
