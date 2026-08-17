// Package migrate applies readable PostgreSQL migrations through Soro transactions.
package migrate

import (
	"context"
	"fmt"
	"sort"
	"time"

	"github.com/ruby-dev/soro/database"
	"github.com/uptrace/bun"
)

type Migration struct {
	Name string
	Up   []string
	Down []string
}

type Status struct {
	Name      string
	Applied   bool
	AppliedAt *time.Time
}

type Migrator struct{ db *database.DB }

func New(db *database.DB) *Migrator { return &Migrator{db: db} }

func (m *Migrator) Apply(ctx context.Context, migrations []Migration) error {
	if err := validateMigrations(migrations); err != nil {
		return err
	}
	if err := m.ensureLedger(ctx); err != nil {
		return err
	}
	for _, migration := range migrations {
		applied, err := m.applied(ctx, migration.Name)
		if err != nil {
			return err
		}
		if applied {
			continue
		}
		if err := m.db.Transaction(ctx, func(txContext context.Context) error {
			if err := execStatements(txContext, m.db.IDB(txContext), migration.Up); err != nil {
				return fmt.Errorf("apply migration %s: %w", migration.Name, err)
			}
			_, err := m.db.IDB(txContext).ExecContext(txContext,
				"INSERT INTO soro_schema_migrations (name) VALUES (?)", migration.Name)
			return err
		}); err != nil {
			return err
		}
	}
	return nil
}

func (m *Migrator) Rollback(ctx context.Context, migrations []Migration, count int) error {
	if count < 1 {
		return fmt.Errorf("migration rollback count must be at least 1")
	}
	if err := validateMigrations(migrations); err != nil {
		return err
	}
	if err := m.ensureLedger(ctx); err != nil {
		return err
	}
	byName := make(map[string]Migration, len(migrations))
	for _, migration := range migrations {
		byName[migration.Name] = migration
	}
	var names []string
	if err := m.db.Bun().NewSelect().Table("soro_schema_migrations").Column("name").
		OrderExpr("applied_at DESC, name DESC").Limit(count).Scan(ctx, &names); err != nil {
		return fmt.Errorf("load applied migrations: %w", err)
	}
	for _, name := range names {
		migration, ok := byName[name]
		if !ok {
			return fmt.Errorf("migration %s is applied but unavailable", name)
		}
		if err := m.db.Transaction(ctx, func(txContext context.Context) error {
			if err := execStatements(txContext, m.db.IDB(txContext), migration.Down); err != nil {
				return fmt.Errorf("rollback migration %s: %w", name, err)
			}
			_, err := m.db.IDB(txContext).ExecContext(txContext,
				"DELETE FROM soro_schema_migrations WHERE name = ?", name)
			return err
		}); err != nil {
			return err
		}
	}
	return nil
}

func (m *Migrator) Status(ctx context.Context, migrations []Migration) ([]Status, error) {
	if err := validateMigrations(migrations); err != nil {
		return nil, err
	}
	if err := m.ensureLedger(ctx); err != nil {
		return nil, err
	}
	type appliedMigration struct {
		Name      string
		AppliedAt time.Time
	}
	var applied []appliedMigration
	if err := m.db.Bun().NewSelect().Table("soro_schema_migrations").Scan(ctx, &applied); err != nil {
		return nil, err
	}
	byName := make(map[string]time.Time, len(applied))
	for _, migration := range applied {
		byName[migration.Name] = migration.AppliedAt
	}
	statuses := make([]Status, 0, len(migrations))
	for _, migration := range migrations {
		status := Status{Name: migration.Name}
		if appliedAt, ok := byName[migration.Name]; ok {
			status.Applied = true
			status.AppliedAt = &appliedAt
		}
		statuses = append(statuses, status)
	}
	return statuses, nil
}

func (m *Migrator) ensureLedger(ctx context.Context) error {
	_, err := m.db.Bun().ExecContext(ctx, `
CREATE TABLE IF NOT EXISTS soro_schema_migrations (
    name TEXT PRIMARY KEY,
    applied_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
)`)
	if err != nil {
		return fmt.Errorf("create migration ledger: %w", err)
	}
	return nil
}

func (m *Migrator) applied(ctx context.Context, name string) (bool, error) {
	return m.db.Bun().NewSelect().Table("soro_schema_migrations").
		Where("name = ?", name).Exists(ctx)
}

func execStatements(ctx context.Context, db bun.IDB, statements []string) error {
	for _, statement := range statements {
		if _, err := db.ExecContext(ctx, statement); err != nil {
			return err
		}
	}
	return nil
}

func validateMigrations(migrations []Migration) error {
	names := make(map[string]struct{}, len(migrations))
	ordered := make([]string, 0, len(migrations))
	for _, migration := range migrations {
		if migration.Name == "" {
			return fmt.Errorf("migration name is required")
		}
		if len(migration.Up) == 0 || len(migration.Down) == 0 {
			return fmt.Errorf("migration %s must define up and down statements", migration.Name)
		}
		if _, exists := names[migration.Name]; exists {
			return fmt.Errorf("duplicate migration %s", migration.Name)
		}
		names[migration.Name] = struct{}{}
		ordered = append(ordered, migration.Name)
	}
	sorted := append([]string(nil), ordered...)
	sort.Strings(sorted)
	for index := range ordered {
		if ordered[index] != sorted[index] {
			return fmt.Errorf("migrations must be ordered by name")
		}
	}
	return nil
}
