package factory_test

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"

	"github.com/ruby-dev/soro/factory"
)

type user struct {
	Email  string
	Active bool
}

func TestFactoryBuildTraitsAndPersistence(t *testing.T) {
	var persisted []*user
	users, err := factory.New(func(sequence uint64) *user {
		return &user{Email: fmt.Sprintf("user-%d@example.com", sequence), Active: true}
	}, func(_ context.Context, value *user) error {
		persisted = append(persisted, value)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	inactive := func(value *user) { value.Active = false }
	built, err := users.Build(inactive)
	if err != nil {
		t.Fatal(err)
	}
	if built.Email != "user-1@example.com" || built.Active || len(persisted) != 0 {
		t.Fatalf("unexpected built user: %#v persisted=%d", built, len(persisted))
	}
	created, err := users.Create(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if created.Email != "user-2@example.com" || len(persisted) != 1 || persisted[0] != created {
		t.Fatalf("unexpected created user: %#v persisted=%#v", created, persisted)
	}
}

func TestFactoryListsValidationAndErrors(t *testing.T) {
	persistErr := errors.New("database unavailable")
	users, err := factory.New(func(sequence uint64) *user { return &user{Email: fmt.Sprint(sequence)} }, func(context.Context, *user) error { return persistErr })
	if err != nil {
		t.Fatal(err)
	}
	values, err := users.BuildList(3)
	if err != nil || len(values) != 3 || values[2].Email != "3" {
		t.Fatalf("BuildList() = %#v, %v", values, err)
	}
	if _, err := users.BuildList(-1); err == nil {
		t.Fatal("expected negative count error")
	}
	if _, err := users.Build(nil); err == nil {
		t.Fatal("expected nil trait error")
	}
	if _, err := users.Create(t.Context()); !errors.Is(err, persistErr) {
		t.Fatalf("expected wrapped persistence error, got %v", err)
	}
	cancelled, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := users.Create(cancelled); !errors.Is(err, context.Canceled) {
		t.Fatalf("expected cancellation, got %v", err)
	}
	withoutPersistence, err := factory.New(func(uint64) *user { return &user{} }, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := withoutPersistence.Create(t.Context()); err == nil {
		t.Fatal("expected missing persister error")
	}
}

func TestFactorySequenceIsConcurrent(t *testing.T) {
	users, err := factory.New(func(sequence uint64) *user { return &user{Email: fmt.Sprint(sequence)} }, nil)
	if err != nil {
		t.Fatal(err)
	}
	const count = 100
	values := make(chan string, count)
	var group sync.WaitGroup
	for range count {
		group.Add(1)
		go func() {
			defer group.Done()
			built, buildErr := users.Build()
			if buildErr != nil {
				t.Error(buildErr)
				return
			}
			values <- built.Email
		}()
	}
	group.Wait()
	close(values)
	seen := make(map[string]bool, count)
	for value := range values {
		seen[value] = true
	}
	if len(seen) != count {
		t.Fatalf("got %d unique sequences, want %d", len(seen), count)
	}
}

func TestFactoryRejectsInvalidConstruction(t *testing.T) {
	if _, err := factory.New[user](nil, nil); err == nil {
		t.Fatal("expected missing builder error")
	}
	nilBuilder, err := factory.New(func(uint64) *user { return nil }, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := nilBuilder.Build(); err == nil {
		t.Fatal("expected nil entity error")
	}
	var uninitialized *factory.Factory[user]
	if _, err := uninitialized.Build(); err == nil {
		t.Fatal("expected uninitialized factory error")
	}
}
