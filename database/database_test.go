package database_test

import (
	"context"
	"testing"

	"github.com/ruby-dev/soro/config"
	"github.com/ruby-dev/soro/database"
)

func TestOpenRequiresURL(t *testing.T) {
	_, err := database.Open(context.Background(), config.Defaults().Database)
	if err == nil {
		t.Fatal("expected missing URL error")
	}
}

func TestWrapRequiresBun(t *testing.T) {
	_, err := database.Wrap(nil)
	if err == nil {
		t.Fatal("expected missing Bun error")
	}
}
