package main

import (
	"context"
	"fmt"
	"os"

	"github.com/datasoro/soro"
	"github.com/datasoro/soro/config"
	"github.com/datasoro/soro/examples/basic"
	"github.com/datasoro/soro/migrate"
	"github.com/datasoro/soro/model"
	"github.com/datasoro/soro/repository"
)

func main() {
	if err := run(context.Background()); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(ctx context.Context) error {
	settings := config.Defaults()
	settings.App.Name = "soro-phase1-example"
	settings.Database.URL = os.Getenv("DATABASE_URL")
	app, err := soro.New(ctx, soro.WithConfig(&settings))
	if err != nil {
		return err
	}
	defer app.Close()

	if err := migrate.New(app.DB).Apply(ctx, basic.Migrations); err != nil {
		return err
	}
	users := repository.New[basic.User](app.DB)
	if err := basic.Seed(ctx, users); err != nil {
		return err
	}
	user := &basic.User{
		Base:  model.Base{Name: "Dustin", Metadata: model.Metadata{"source": "phase1-demo"}},
		Email: "USER@EXAMPLE.COM", Active: true,
	}
	if err := users.Create(ctx, user); err != nil {
		return err
	}
	found, err := users.Find(ctx, user.ID)
	if err != nil {
		return err
	}
	found.Email = "updated@example.com"
	if err := users.Update(ctx, found); err != nil {
		return err
	}
	if err := users.Delete(ctx, found.ID); err != nil {
		return err
	}
	deleted, err := users.OnlyDeleted().Find(ctx, found.ID)
	if err != nil {
		return err
	}
	if err := users.Restore(ctx, deleted.ID); err != nil {
		return err
	}
	fmt.Printf("restored user %s (%s)\n", found.Email, found.ID)
	return users.ForceDelete(ctx, found.ID)
}
