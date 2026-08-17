package factory_test

import (
	"fmt"
	"testing"

	"github.com/ruby-dev/soro/factory"
)

func BenchmarkBuild(b *testing.B) {
	users, err := factory.New(func(sequence uint64) *user {
		return &user{Email: fmt.Sprintf("user-%d@example.com", sequence), Active: true}
	}, nil)
	if err != nil {
		b.Fatal(err)
	}
	for b.Loop() {
		value, buildErr := users.Build()
		if buildErr != nil || value.Email == "" {
			b.Fatalf("Build() value=%#v err=%v", value, buildErr)
		}
	}
}
