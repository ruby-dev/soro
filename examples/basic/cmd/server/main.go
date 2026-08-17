package main

import (
	"context"
	"log"
	"os/signal"
	"syscall"

	"github.com/ruby-dev/soro"
	"github.com/ruby-dev/soro/api"
	"github.com/ruby-dev/soro/examples/basic"
	"github.com/ruby-dev/soro/migrate"
	"github.com/ruby-dev/soro/repository"
)

func main() {
	app, err := soro.New(context.Background())
	if err != nil {
		log.Fatal(err)
	}
	defer app.Close()
	if err := migrate.New(app.DB).Apply(context.Background(), basic.Migrations); err != nil {
		log.Fatal(err)
	}
	if err := app.Jobs.Migrate(context.Background()); err != nil {
		log.Fatal(err)
	}
	userRepository := repository.New[basic.User](app.DB)
	accountRepository := repository.New[basic.Account](app.DB)
	projectRepository := repository.New[basic.Project](app.DB)
	if err := basic.RegisterJobs(app.Jobs, userRepository, app.Mailer); err != nil {
		log.Fatal(err)
	}
	users, err := basic.NewUserResourceWithJobs(userRepository, app.Jobs)
	if err != nil {
		log.Fatal(err)
	}
	accounts, err := basic.NewAccountResource(accountRepository)
	if err != nil {
		log.Fatal(err)
	}
	projects, err := basic.NewProjectResource(projectRepository)
	if err != nil {
		log.Fatal(err)
	}
	if err := app.API.Version("v1", func(v1 *api.Router) {
		if err := v1.Resource("/accounts", accounts); err != nil {
			log.Fatal(err)
		}
		if err := v1.Resource("/users", users); err != nil {
			log.Fatal(err)
		}
		if err := v1.Resource("/projects", projects); err != nil {
			log.Fatal(err)
		}
	}); err != nil {
		log.Fatal(err)
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	app.Logger.Info("starting Soro example", "port", app.Config.HTTP.Port)
	if err := app.Serve(ctx); err != nil {
		log.Fatal(err)
	}
}
