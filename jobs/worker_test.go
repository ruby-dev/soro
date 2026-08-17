package jobs_test

import (
	"context"
	"errors"
	"testing"

	"github.com/ruby-dev/soro/jobs"
)

type performArgs struct{ Value string }

func (performArgs) Kind() string { return "perform_test" }

type invalidPerformArgs struct{}

func (invalidPerformArgs) Kind() string { return "" }

func TestPerformExecutesTypedHandler(t *testing.T) {
	called := false
	err := jobs.Perform(t.Context(), performArgs{Value: "expected"}, func(_ context.Context, args performArgs) error {
		called = args.Value == "expected"
		return nil
	})
	if err != nil || !called {
		t.Fatalf("Perform() called=%v err=%v", called, err)
	}
}

func TestPerformValidationAndErrors(t *testing.T) {
	want := errors.New("handler failed")
	if err := jobs.Perform(t.Context(), performArgs{}, func(context.Context, performArgs) error { return want }); !errors.Is(err, want) {
		t.Fatalf("expected handler error, got %v", err)
	}
	if err := jobs.Perform[performArgs](t.Context(), performArgs{}, nil); err == nil {
		t.Fatal("expected nil handler error")
	}
	if err := jobs.Perform(t.Context(), invalidPerformArgs{}, func(context.Context, invalidPerformArgs) error { return nil }); err == nil {
		t.Fatal("expected empty kind error")
	}
	cancelled, cancel := context.WithCancel(t.Context())
	cancel()
	if err := jobs.Perform(cancelled, performArgs{}, func(context.Context, performArgs) error { return nil }); !errors.Is(err, context.Canceled) {
		t.Fatalf("expected cancellation, got %v", err)
	}
}
