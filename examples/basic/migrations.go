package basic

import "github.com/datasoro/soro/migrate"

var Migrations = []migrate.Migration{
	{
		Name: "001_create_users",
		Up: []string{
			`CREATE TABLE users (
    id UUID PRIMARY KEY,
    name VARCHAR(255) NOT NULL DEFAULT '',
    description TEXT NOT NULL DEFAULT '',
    metadata JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    deleted_at TIMESTAMPTZ NULL,
    created_by UUID NULL,
    updated_by UUID NULL,
    deleted_by UUID NULL,
    email VARCHAR(255) NOT NULL,
    active BOOLEAN NOT NULL DEFAULT TRUE
)`,
			`CREATE UNIQUE INDEX users_email_unique ON users (email) WHERE deleted_at IS NULL`,
		},
		Down: []string{`DROP TABLE users`},
	},
}
