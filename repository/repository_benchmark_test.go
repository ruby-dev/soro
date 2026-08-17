package repository_test

import (
	"fmt"
	"testing"

	"github.com/ruby-dev/soro/examples/basic"
	"github.com/ruby-dev/soro/internal/testdb"
	"github.com/ruby-dev/soro/migrate"
	"github.com/ruby-dev/soro/model"
	"github.com/ruby-dev/soro/repository"
)

func BenchmarkCreateFindForceDelete(b *testing.B) {
	db := testdb.Open(b)
	if err := migrate.New(db).Apply(b.Context(), basic.Migrations); err != nil {
		b.Fatal(err)
	}
	users := repository.New[basic.User](db)
	b.ResetTimer()
	for index := 0; b.Loop(); index++ {
		user := &basic.User{Base: model.Base{Name: "Benchmark"}, Email: fmt.Sprintf("benchmark-%d@example.com", index), Active: true}
		if err := users.Create(b.Context(), user); err != nil {
			b.Fatal(err)
		}
		if _, err := users.Find(b.Context(), user.ID); err != nil {
			b.Fatal(err)
		}
		if err := users.ForceDelete(b.Context(), user.ID); err != nil {
			b.Fatal(err)
		}
	}
}
