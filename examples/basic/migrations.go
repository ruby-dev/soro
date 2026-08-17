package basic

import "github.com/ruby-dev/soro/migrate"

var Migrations = []migrate.Migration{
	{
		Name: "001_create_accounts",
		Up: []string{
			`CREATE TABLE accounts (
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
    slug VARCHAR(255) NOT NULL
)`,
			`CREATE UNIQUE INDEX accounts_slug_unique ON accounts (slug) WHERE deleted_at IS NULL`,
		},
		Down: []string{`DROP TABLE accounts`},
	},
	{
		Name: "002_create_users",
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
    active BOOLEAN NOT NULL DEFAULT TRUE,
    account_id UUID NULL REFERENCES accounts(id)
)`,
			`CREATE UNIQUE INDEX users_email_unique ON users (email) WHERE deleted_at IS NULL`,
		},
		Down: []string{`DROP TABLE users`},
	},
	{
		Name: "003_create_projects",
		Up: []string{
			`CREATE TABLE projects (
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
    account_id UUID NOT NULL REFERENCES accounts(id),
    owner_id UUID NULL REFERENCES users(id),
    status VARCHAR(64) NOT NULL
)`,
			`CREATE INDEX projects_account_id_idx ON projects (account_id)`,
			`CREATE INDEX projects_owner_id_idx ON projects (owner_id)`,
			`CREATE INDEX projects_status_idx ON projects (status)`,
		},
		Down: []string{`DROP TABLE projects`},
	},
}
