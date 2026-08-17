package database

import (
	"context"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
)

// CreateDatabase creates the database named by databaseURL by connecting to
// PostgreSQL's maintenance database. The database name is parsed by pgx and
// quoted as an identifier rather than accepted as SQL.
func CreateDatabase(ctx context.Context, databaseURL string) error {
	config, name, err := administrationConfig(databaseURL)
	if err != nil {
		return err
	}
	connection, err := pgx.ConnectConfig(ctx, config)
	if err != nil {
		return fmt.Errorf("database: connect to maintenance database: %w", err)
	}
	defer connection.Close(context.Background())
	if _, err := connection.Exec(ctx, "CREATE DATABASE "+pgx.Identifier{name}.Sanitize()); err != nil {
		return fmt.Errorf("database: create %s: %w", name, err)
	}
	return nil
}

// DropDatabase physically removes the database named by databaseURL. Callers
// are expected to require an explicit confirmation before invoking it.
func DropDatabase(ctx context.Context, databaseURL string) error {
	config, name, err := administrationConfig(databaseURL)
	if err != nil {
		return err
	}
	connection, err := pgx.ConnectConfig(ctx, config)
	if err != nil {
		return fmt.Errorf("database: connect to maintenance database: %w", err)
	}
	defer connection.Close(context.Background())
	if _, err := connection.Exec(ctx, "DROP DATABASE "+pgx.Identifier{name}.Sanitize()+" WITH (FORCE)"); err != nil {
		return fmt.Errorf("database: drop %s: %w", name, err)
	}
	return nil
}

func administrationConfig(databaseURL string) (*pgx.ConnConfig, string, error) {
	if strings.TrimSpace(databaseURL) == "" {
		return nil, "", fmt.Errorf("database: URL is required")
	}
	config, err := pgx.ParseConfig(databaseURL)
	if err != nil {
		return nil, "", fmt.Errorf("database: parse URL: %w", err)
	}
	name := config.Database
	if name == "" {
		return nil, "", fmt.Errorf("database: URL must name a database")
	}
	switch name {
	case "postgres", "template0", "template1":
		return nil, "", fmt.Errorf("database: refusing to administer protected database %q", name)
	}
	config.Database = "postgres"
	return config, name, nil
}
