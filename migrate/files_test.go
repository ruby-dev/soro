package migrate

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadDir(t *testing.T) {
	directory := t.TempDir()
	writeMigration(t, directory, "002_second.sql", `-- +soro Up
CREATE INDEX example_name_idx ON examples (name);
-- +soro Down
DROP INDEX example_name_idx;
`)
	writeMigration(t, directory, "001_first.sql", `-- +soro Up
CREATE TABLE examples (id UUID, note TEXT DEFAULT 'semi;colon');
-- comment ;
CREATE FUNCTION example_fn() RETURNS void AS $$ BEGIN PERFORM 1; END $$ LANGUAGE plpgsql;
-- +soro Down
DROP FUNCTION example_fn();
DROP TABLE examples;
`)
	migrations, err := LoadDir(directory)
	if err != nil {
		t.Fatal(err)
	}
	if len(migrations) != 2 || migrations[0].Name != "001_first" || len(migrations[0].Up) != 2 {
		t.Fatalf("unexpected migrations: %#v", migrations)
	}
}

func TestLoadFileRejectsMalformedMigrations(t *testing.T) {
	for name, contents := range map[string]string{
		"missing_down.sql": "-- +soro Up\nSELECT 1;\n",
		"empty.sql":        "-- +soro Up\n-- +soro Down\nSELECT 1;\n",
		"before.sql":       "SELECT 1;\n-- +soro Up\nSELECT 1;\n-- +soro Down\nSELECT 1;\n",
		"unterminated.sql": "-- +soro Up\nSELECT 'bad;\n-- +soro Down\nSELECT 1;\n",
	} {
		t.Run(name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), name)
			if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
				t.Fatal(err)
			}
			if _, err := LoadFile(path); err == nil {
				t.Fatal("expected migration to fail")
			}
		})
	}
}

func TestLoadDirMissingIsEmpty(t *testing.T) {
	migrations, err := LoadDir(filepath.Join(t.TempDir(), "missing"))
	if err != nil || len(migrations) != 0 {
		t.Fatalf("expected empty migrations, got %#v, %v", migrations, err)
	}
}

func writeMigration(t *testing.T, directory, name, contents string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(directory, name), []byte(contents), 0o600); err != nil {
		t.Fatal(err)
	}
}
