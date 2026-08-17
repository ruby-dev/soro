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

func TestSeedApplicationBuildsRelationshipGraphIdempotently(t *testing.T) {
	app := sorotest.New(t, sorotest.WithMigrations(basic.Migrations...))
	if err := basic.SeedApplication(t.Context(), app.DB); err != nil {
		t.Fatal(err)
	}
	if err := basic.SeedApplication(t.Context(), app.DB); err != nil {
		t.Fatal(err)
	}
	accounts := repository.New[basic.Account](app.DB)
	users := repository.New[basic.User](app.DB)
	projects := repository.New[basic.Project](app.DB)
	account, err := accounts.FindBy(t.Context(), "Slug", "datasoro-demo")
	if err != nil {
		t.Fatal(err)
	}
	user, err := users.FindBy(t.Context(), "Email", "demo@example.com")
	if err != nil {
		t.Fatal(err)
	}
	project, err := projects.FindBy(t.Context(), "Name", "Soro")
	if err != nil {
		t.Fatal(err)
	}
	if user.AccountID == nil || *user.AccountID != account.ID || project.AccountID != account.ID || project.OwnerID == nil || *project.OwnerID != user.ID {
		t.Fatalf("relationship graph account=%s user=%#v project=%#v", account.ID, user, project)
	}
	accountCount, accountErr := accounts.Count(t.Context())
	userCount, userErr := users.Count(t.Context())
	projectCount, projectErr := projects.Count(t.Context())
	for name, count := range map[string]int{
		"accounts": mustCount(t, accountCount, accountErr),
		"users":    mustCount(t, userCount, userErr),
		"projects": mustCount(t, projectCount, projectErr),
	} {
		if count != 1 {
			t.Fatalf("%s count = %d, want 1", name, count)
		}
	}
}

func mustCount(t *testing.T, count int, err error) int {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
	return count
}
