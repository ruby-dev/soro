package generate

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/datasoro/soro/internal/testdb"
	"github.com/datasoro/soro/migrate"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
)

func TestGeneratedResourceMigrationOnPostgreSQL(t *testing.T) {
	db := testdb.Open(t)
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte("module example.com/generated\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	generator, err := Open(root, false)
	if err != nil {
		t.Fatal(err)
	}
	generator.Now = func() time.Time { return time.Date(2026, 8, 16, 12, 0, 0, 0, time.UTC) }
	if _, err := generator.GenerateResource("User", []string{"email:string:unique:index", "active:bool:default=true"}); err != nil {
		t.Fatal(err)
	}
	migrations, err := migrate.LoadDir(filepath.Join(root, "db", "migrations"))
	if err != nil {
		t.Fatal(err)
	}
	migrator := migrate.New(db)
	if err := migrator.Apply(t.Context(), migrations); err != nil {
		t.Fatal(err)
	}

	firstID := uuid.New()
	if _, err := db.Bun().ExecContext(t.Context(), "INSERT INTO users (id, email) VALUES (?, ?)", firstID, "user@example.com"); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Bun().ExecContext(t.Context(), "INSERT INTO users (id, email) VALUES (?, ?)", uuid.New(), "user@example.com"); !isConstraintViolation(err) {
		t.Fatalf("expected live duplicate to violate partial index, got %v", err)
	}
	if _, err := db.Bun().ExecContext(t.Context(), "UPDATE users SET deleted_at = NOW() WHERE id = ?", firstID); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Bun().ExecContext(t.Context(), "INSERT INTO users (id, email) VALUES (?, ?)", uuid.New(), "user@example.com"); err != nil {
		t.Fatalf("soft-deleted value should be reusable: %v", err)
	}
	statuses, err := migrator.Status(t.Context(), migrations)
	if err != nil || len(statuses) != 1 || !statuses[0].Applied {
		t.Fatalf("unexpected migration status: %#v, %v", statuses, err)
	}
	if err := migrator.Rollback(t.Context(), migrations, 1); err != nil {
		t.Fatal(err)
	}
}

func isConstraintViolation(err error) bool {
	var postgresError *pgconn.PgError
	return errors.As(err, &postgresError) && postgresError.Code == "23505"
}
