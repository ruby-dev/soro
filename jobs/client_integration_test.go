package jobs_test

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/datasoro/soro/internal/testdb"
	"github.com/datasoro/soro/jobs"
	"github.com/datasoro/soro/observability"
	"go.opentelemetry.io/otel/trace"
)

type exampleArgs struct {
	Value string `json:"value" river:"unique"`
}

func (exampleArgs) Kind() string { return "soro_test_example" }

func TestEnqueueAndTransactionSemantics(t *testing.T) {
	db := testdb.Open(t)
	observer := testObserver(t)
	client, err := jobs.New(db, observer, discardLogger(), jobs.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := client.Migrate(t.Context()); err != nil {
		t.Fatal(err)
	}

	result, err := client.Enqueue(t.Context(), exampleArgs{Value: "normal"}, jobs.Queue("default"), jobs.Priority(2), jobs.Delay(time.Second))
	if err != nil {
		t.Fatal(err)
	}
	if result.ID == 0 || result.Duplicate {
		t.Fatalf("unexpected result: %+v", result)
	}

	if err := db.Transaction(t.Context(), func(ctx context.Context) error {
		_, err := client.EnqueueTx(ctx, exampleArgs{Value: "committed"})
		return err
	}); err != nil {
		t.Fatal(err)
	}

	rollback := errors.New("rollback")
	err = db.Transaction(t.Context(), func(ctx context.Context) error {
		if _, err := client.Enqueue(ctx, exampleArgs{Value: "rolled-back"}); err != nil {
			return err
		}
		return rollback
	})
	if !errors.Is(err, rollback) {
		t.Fatalf("rollback error = %v", err)
	}

	var count int
	if err := db.Bun().NewSelect().Table("river_job").ColumnExpr("count(*)").Scan(t.Context(), &count); err != nil {
		t.Fatal(err)
	}
	if count != 2 {
		t.Fatalf("job count = %d, want 2", count)
	}
	if _, err := client.EnqueueTx(t.Context(), exampleArgs{}); err == nil {
		t.Fatal("expected missing transaction error")
	}

	first, err := client.Enqueue(t.Context(), exampleArgs{Value: "unique"}, jobs.UniqueByArgs())
	if err != nil {
		t.Fatal(err)
	}
	second, err := client.Enqueue(t.Context(), exampleArgs{Value: "unique"}, jobs.UniqueByArgs())
	if err != nil {
		t.Fatal(err)
	}
	if first.Duplicate || !second.Duplicate {
		t.Fatalf("unique results: first=%+v second=%+v", first, second)
	}
}

func TestWorkerExecutesTypedHandler(t *testing.T) {
	db := testdb.Open(t)
	observer := testObserver(t)
	client, err := jobs.New(db, observer, discardLogger(), jobs.Config{
		WorkersEnabled: true, Queues: map[string]int{"default": 1},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := client.Migrate(t.Context()); err != nil {
		t.Fatal(err)
	}
	type performedJob struct {
		value   string
		traceID trace.TraceID
	}
	performed := make(chan performedJob, 1)
	if err := jobs.Register(client, func(ctx context.Context, args exampleArgs) error {
		performed <- performedJob{value: args.Value, traceID: trace.SpanContextFromContext(ctx).TraceID()}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	workerContext, cancel := context.WithCancel(t.Context())
	defer cancel()
	if err := client.Start(workerContext); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		ctx, stop := context.WithTimeout(context.Background(), 5*time.Second)
		defer stop()
		if err := client.Stop(ctx); err != nil {
			t.Error(err)
		}
	})
	traceID := trace.TraceID{1, 2, 3, 4}
	spanID := trace.SpanID{5, 6, 7, 8}
	tracedContext := trace.ContextWithRemoteSpanContext(t.Context(), trace.NewSpanContext(trace.SpanContextConfig{TraceID: traceID, SpanID: spanID, TraceFlags: trace.FlagsSampled, Remote: true}))
	if _, err := client.Enqueue(tracedContext, exampleArgs{Value: "worked"}); err != nil {
		t.Fatal(err)
	}
	select {
	case performed := <-performed:
		if performed.value != "worked" || performed.traceID != traceID {
			t.Fatalf("worked job = %+v", performed)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("job was not performed")
	}
}

func TestOptionsValidation(t *testing.T) {
	for _, option := range []jobs.Option{jobs.Queue(""), jobs.Delay(-time.Second), jobs.Priority(10), jobs.MaxAttempts(0), jobs.Unique(jobs.UniqueConfig{})} {
		db := testdb.Open(t)
		client, err := jobs.New(db, testObserver(t), discardLogger(), jobs.Config{})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := client.Enqueue(t.Context(), exampleArgs{}, option); err == nil {
			t.Fatal("expected option error")
		}
	}
}

func testObserver(t *testing.T) *observability.Provider {
	t.Helper()
	provider, err := observability.New(t.Context(), observability.Config{ServiceName: "jobs-test", Environment: "test"})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := provider.Shutdown(context.Background()); err != nil {
			t.Error(err)
		}
	})
	return provider
}

func discardLogger() *slog.Logger { return slog.New(slog.NewTextHandler(io.Discard, nil)) }
