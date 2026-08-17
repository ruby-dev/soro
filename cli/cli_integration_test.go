package cli

import (
	"bytes"
	"context"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

func TestDatabaseMigrationCommands(t *testing.T) {
	baseURL := os.Getenv("SORO_TEST_DATABASE_URL")
	if baseURL == "" {
		t.Skip("set SORO_TEST_DATABASE_URL to run PostgreSQL integration tests")
	}
	schema := "soro_cli_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	admin, err := pgx.Connect(t.Context(), baseURL)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := admin.Exec(t.Context(), "CREATE SCHEMA "+pgx.Identifier{schema}.Sanitize()); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if _, err := admin.Exec(context.Background(), "DROP SCHEMA "+pgx.Identifier{schema}.Sanitize()+" CASCADE"); err != nil {
			t.Errorf("drop CLI test schema: %v", err)
		}
		if err := admin.Close(context.Background()); err != nil {
			t.Errorf("close CLI test admin connection: %v", err)
		}
	})
	databaseURL := databaseURLWithSearchPath(t, baseURL, schema)

	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "config"), 0o755); err != nil {
		t.Fatal(err)
	}
	configuration := "app:\n  name: cli-test\ndatabase:\n  url: " + databaseURL + "\nmail:\n  transport: capture\n"
	if err := os.WriteFile(filepath.Join(root, "config", "application.yaml"), []byte(configuration), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, "db", "migrations"), 0o755); err != nil {
		t.Fatal(err)
	}
	migration := "-- +soro Up\nCREATE TABLE widgets (id UUID PRIMARY KEY);\n-- +soro Down\nDROP TABLE widgets;\n"
	if err := os.WriteFile(filepath.Join(root, "db", "migrations", "001_create_widgets.sql"), []byte(migration), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("SORO_ENV", "test")
	t.Setenv("DATABASE_URL", databaseURL)

	if err := executeForTest(root, ioDiscardBuffer(), "db", "migrate"); err != nil {
		t.Fatal(err)
	}
	var status bytes.Buffer
	if err := executeForTest(root, &status, "db", "status"); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(status.String(), "up   001_create_widgets") {
		t.Fatalf("unexpected migration status %q", status.String())
	}
	if err := executeForTest(root, ioDiscardBuffer(), "db", "rollback"); err != nil {
		t.Fatal(err)
	}
	connection, err := pgx.Connect(t.Context(), databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	defer connection.Close(context.Background())
	var exists bool
	if err := connection.QueryRow(t.Context(), "SELECT to_regclass('public.widgets') IS NOT NULL").Scan(&exists); err != nil {
		t.Fatal(err)
	}
	if exists {
		t.Fatal("rollback did not remove generated table")
	}
}

func executeForTest(root string, output *bytes.Buffer, arguments ...string) error {
	command := New(Settings{Directory: root, Stdout: output, Stderr: output})
	command.SetArgs(arguments)
	return command.Execute()
}

func ioDiscardBuffer() *bytes.Buffer { return &bytes.Buffer{} }

func databaseURLWithSearchPath(t *testing.T, baseURL, schema string) string {
	t.Helper()
	parsed, err := url.Parse(baseURL)
	if err != nil {
		t.Fatal(err)
	}
	query := parsed.Query()
	query.Set("search_path", schema)
	parsed.RawQuery = query.Encode()
	return parsed.String()
}
