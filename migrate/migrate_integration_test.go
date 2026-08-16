package migrate_test

import (
	"testing"

	"github.com/datasoro/soro/examples/basic"
	"github.com/datasoro/soro/internal/testdb"
	"github.com/datasoro/soro/migrate"
)

func TestGeneratedPostgreSQLMigrationAppliesAndRollsBack(t *testing.T) {
	db := testdb.Open(t)
	migrator := migrate.New(db)
	if err := migrator.Apply(t.Context(), basic.Migrations); err != nil {
		t.Fatal(err)
	}
	statuses, err := migrator.Status(t.Context(), basic.Migrations)
	if err != nil {
		t.Fatal(err)
	}
	if len(statuses) != 1 || !statuses[0].Applied || statuses[0].AppliedAt == nil {
		t.Fatalf("unexpected status: %+v", statuses)
	}
	if err := migrator.Rollback(t.Context(), basic.Migrations, 1); err != nil {
		t.Fatal(err)
	}
	exists, err := db.Bun().NewSelect().Table("information_schema.tables").
		Where("table_schema = current_schema()").Where("table_name = 'users'").Exists(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if exists {
		t.Fatal("users table still exists after rollback")
	}
}
