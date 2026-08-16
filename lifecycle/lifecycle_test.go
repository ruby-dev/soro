package lifecycle_test

import (
	"context"
	"reflect"
	"testing"

	"github.com/datasoro/soro/lifecycle"
	"github.com/datasoro/soro/model"
)

type orderedModel struct {
	model.Base
	Events *[]string `bun:"-"`
}

func (m *orderedModel) BeforeCreate(context.Context, *lifecycle.Context) error {
	*m.Events = append(*m.Events, "model")
	return nil
}

func (m *orderedModel) AfterCreate(context.Context, *lifecycle.Context) error {
	*m.Events = append(*m.Events, "model")
	return nil
}

func TestRegistryOrdering(t *testing.T) {
	registry := lifecycle.NewRegistry()
	events := []string{}
	record := func(name string) lifecycle.Handler {
		return func(context.Context, any, *lifecycle.Context) error {
			events = append(events, name)
			return nil
		}
	}
	if err := registry.RegisterGlobal(lifecycle.BeforeCreate, 0, record("global")); err != nil {
		t.Fatal(err)
	}
	if err := registry.Register(orderedModel{}, lifecycle.BeforeCreate, 10, record("registered-10")); err != nil {
		t.Fatal(err)
	}
	if err := registry.Register(orderedModel{}, lifecycle.BeforeCreate, 0, record("registered-0")); err != nil {
		t.Fatal(err)
	}
	entity := &orderedModel{Events: &events}
	if err := registry.Run(context.Background(), lifecycle.BeforeCreate, entity, &lifecycle.Context{}); err != nil {
		t.Fatal(err)
	}
	want := []string{"global", "registered-0", "registered-10", "model"}
	if !reflect.DeepEqual(events, want) {
		t.Fatalf("before order = %v, want %v", events, want)
	}

	events = events[:0]
	if err := registry.RegisterGlobal(lifecycle.AfterCreate, 0, record("global")); err != nil {
		t.Fatal(err)
	}
	if err := registry.Register(orderedModel{}, lifecycle.AfterCreate, 0, record("registered")); err != nil {
		t.Fatal(err)
	}
	if err := registry.Run(context.Background(), lifecycle.AfterCreate, entity, &lifecycle.Context{}); err != nil {
		t.Fatal(err)
	}
	want = []string{"model", "registered", "global"}
	if !reflect.DeepEqual(events, want) {
		t.Fatalf("after order = %v, want %v", events, want)
	}
}

func TestCompareTracksPersistedAndRequestedValues(t *testing.T) {
	type account struct {
		model.Base
		Email string         `bun:"email"`
		Data  map[string]any `bun:"data"`
	}
	oldValue := &account{Email: "old@example.com", Data: map[string]any{"role": "viewer"}}
	newValue := &account{Email: "new@example.com", Data: map[string]any{"role": "admin"}}
	changes, err := lifecycle.Compare(oldValue, newValue)
	if err != nil {
		t.Fatal(err)
	}
	if !changes.Changed("Email") || !changes.Was("Email", "old@example.com") || !changes.Is("Email", "new@example.com") {
		t.Fatalf("email change missing: %#v", changes.Fields())
	}
	old, _, ok := changes.Values("Data")
	if !ok {
		t.Fatal("data change missing")
	}
	old.(map[string]any)["role"] = "mutated"
	again, _, _ := changes.Values("Data")
	if again.(map[string]any)["role"] != "viewer" {
		t.Fatal("change snapshots alias returned maps")
	}
}
