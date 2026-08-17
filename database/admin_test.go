package database

import (
	"strings"
	"testing"
)

func TestAdministrationConfig(t *testing.T) {
	config, name, err := administrationConfig("postgres://user:pass@localhost:5432/customer-test?sslmode=disable")
	if err != nil {
		t.Fatal(err)
	}
	if name != "customer-test" || config.Database != "postgres" {
		t.Fatalf("unexpected administration target: name=%q maintenance=%q", name, config.Database)
	}
	if config.User != "user" || config.Password != "pass" {
		t.Fatal("connection credentials were not preserved")
	}
}

func TestAdministrationConfigRejectsProtectedAndMissingDatabases(t *testing.T) {
	for _, databaseURL := range []string{"", "postgres://localhost/postgres", "postgres://localhost/template0", "://bad"} {
		if _, _, err := administrationConfig(databaseURL); err == nil {
			t.Fatalf("expected %q to fail", databaseURL)
		}
	}
}

func TestAdministrationIdentifierQuoting(t *testing.T) {
	_, name, err := administrationConfig("postgres://localhost/customer-test")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(name, "-") {
		t.Fatal("test database should exercise identifier quoting")
	}
}
