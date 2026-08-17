package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/datasoro/soro"
	"github.com/datasoro/soro/api"
	"github.com/datasoro/soro/examples/basic"
	"github.com/datasoro/soro/migrate"
	"github.com/datasoro/soro/repository"
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
	users, err := basic.NewUserResource(repository.New[basic.User](app.DB))
	if err != nil {
		log.Fatal(err)
	}
	if err := app.API.Version("v1", func(v1 *api.Router) {
		if err := v1.Resource("/users", users); err != nil {
			log.Fatal(err)
		}
	}); err != nil {
		log.Fatal(err)
	}

	server := &http.Server{
		Addr:              fmt.Sprintf(":%d", app.Config.HTTP.Port),
		Handler:           app.API.Handler(),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       60 * time.Second,
	}
	app.Logger.Info("starting Soro example", "address", server.Addr)
	if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Fatal(err)
	}
}
