package migrate_test

import (
	"testing"

	"github.com/ruby-dev/soro/migrate"
)

func TestApplyRejectsUnorderedMigrations(t *testing.T) {
	migrations := []migrate.Migration{
		{Name: "002_second", Up: []string{"SELECT 1"}, Down: []string{"SELECT 1"}},
		{Name: "001_first", Up: []string{"SELECT 1"}, Down: []string{"SELECT 1"}},
	}
	if err := migrate.New(nil).Apply(t.Context(), migrations); err == nil {
		t.Fatal("expected ordering error")
	}
}
