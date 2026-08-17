package database

import (
	"context"
	"errors"
	"net/url"
	"os"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

func TestCreateAndDropDatabase(t *testing.T) {
	baseURL := os.Getenv("SORO_TEST_DATABASE_URL")
	if baseURL == "" {
		t.Skip("set SORO_TEST_DATABASE_URL to run PostgreSQL integration tests")
	}
	parsed, err := url.Parse(baseURL)
	if err != nil {
		t.Fatal(err)
	}
	name := "soro_admin_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	parsed.Path = "/" + name
	query := parsed.Query()
	query.Del("search_path")
	parsed.RawQuery = query.Encode()
	targetURL := parsed.String()

	if err := CreateDatabase(t.Context(), targetURL); err != nil {
		var postgresError *pgconn.PgError
		if errors.As(err, &postgresError) && postgresError.Code == "42501" {
			t.Skip("test PostgreSQL role does not have CREATEDB")
		}
		t.Fatal(err)
	}
	dropped := false
	t.Cleanup(func() {
		if !dropped {
			_ = DropDatabase(context.Background(), targetURL)
		}
	})
	connection, err := pgx.Connect(t.Context(), targetURL)
	if err != nil {
		t.Fatal(err)
	}
	if err := connection.Close(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := DropDatabase(t.Context(), targetURL); err != nil {
		t.Fatal(err)
	}
	dropped = true
}
