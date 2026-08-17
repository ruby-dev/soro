package basic_test

import (
	"testing"

	"github.com/datasoro/soro/examples/basic"
	"github.com/datasoro/soro/repository"
	"github.com/datasoro/soro/testutil"
)

func TestUserFactoryAndSeedAreUsable(t *testing.T) {
	app := sorotest.New(t, sorotest.WithMigrations(basic.Migrations...))
	users := repository.New[basic.User](app.DB)
	usersFactory, err := basic.NewUserFactory(users)
	if err != nil {
		t.Fatal(err)
	}
	created, err := usersFactory.Create(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if created.ID.String() == "00000000-0000-0000-0000-000000000000" || created.Email != "user-1@example.com" {
		t.Fatalf("unexpected factory user: %#v", created)
	}
	if err := basic.Seed(t.Context(), users); err != nil {
		t.Fatal(err)
	}
	if err := basic.Seed(t.Context(), users); err != nil {
		t.Fatal(err)
	}
	count, err := users.Count(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if count != 2 {
		t.Fatalf("user count = %d, want factory user plus one idempotent seed", count)
	}
}
