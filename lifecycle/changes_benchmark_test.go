package lifecycle_test

import (
	"testing"
	"time"

	"github.com/ruby-dev/soro/lifecycle"
	"github.com/ruby-dev/soro/model"
)

type benchmarkEntity struct {
	model.Base
	Email  string
	Active bool
}

func BenchmarkCompare(b *testing.B) {
	now := time.Now().UTC()
	before := benchmarkEntity{Base: model.Base{Name: "Dustin", Metadata: model.Metadata{"role": "user"}, CreatedAt: now, UpdatedAt: now}, Email: "old@example.com", Active: true}
	after := before
	after.Email = "new@example.com"
	after.Metadata = model.Metadata{"role": "admin"}
	for b.Loop() {
		changes, err := lifecycle.Compare(&before, &after)
		if err != nil || !changes.Changed("Email") {
			b.Fatalf("Compare() changes=%#v err=%v", changes, err)
		}
	}
}
