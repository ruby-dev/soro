package jobs

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/riverqueue/river"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"
)

type Handler[T river.JobArgs] func(context.Context, T) error

// Perform executes a typed handler synchronously without inserting a River job.
// It is intended for focused handler tests; queue semantics remain the job
// client's responsibility.
func Perform[T river.JobArgs](ctx context.Context, args T, handler Handler[T]) error {
	if handler == nil {
		return fmt.Errorf("jobs handler is required")
	}
	if args.Kind() == "" {
		return fmt.Errorf("jobs kind is required")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	return handler(ctx, args)
}

func Register[T river.JobArgs](client *Client, handler Handler[T]) error {
	if client == nil || handler == nil {
		return fmt.Errorf("jobs client and handler are required")
	}
	return river.AddWorkerSafely(client.workers, &worker[T]{client: client, handler: handler})
}

type worker[T river.JobArgs] struct {
	river.WorkerDefaults[T]
	client  *Client
	handler Handler[T]
}

func (worker *worker[T]) Work(ctx context.Context, job *river.Job[T]) (workErr error) {
	ctx = worker.extractTrace(ctx, job.Metadata)
	ctx, span := worker.client.observer.Tracer().Start(ctx, "job "+job.Kind,
		trace.WithSpanKind(trace.SpanKindConsumer),
		trace.WithAttributes(attribute.String("job.kind", job.Kind), attribute.String("job.queue", job.Queue), attribute.Int("job.attempt", job.Attempt)))
	started := time.Now()
	defer func() {
		if recovered := recover(); recovered != nil {
			workErr = fmt.Errorf("job panic: %v", recovered)
			worker.record(ctx, span, job, started, workErr)
			panic(recovered)
		}
		worker.record(ctx, span, job, started, workErr)
	}()
	workErr = worker.handler(ctx, job.Args)
	return workErr
}

func (worker *worker[T]) record(ctx context.Context, span trace.Span, job *river.Job[T], started time.Time, err error) {
	duration := time.Since(started)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "job failed")
	}
	span.End()
	worker.client.observer.Metrics().RecordJob(ctx, job.Kind, job.Queue, job.Attempt, duration, err)
	fields := []any{"job_name", job.Kind, "queue", job.Queue, "duration", duration, "attempts", job.Attempt}
	if spanContext := trace.SpanContextFromContext(ctx); spanContext.IsValid() {
		fields = append(fields, "trace_id", spanContext.TraceID().String())
	}
	if err != nil {
		worker.client.logger.ErrorContext(ctx, "job failed", append(fields, "error", err)...)
		return
	}
	worker.client.logger.InfoContext(ctx, "job completed", fields...)
}

func (worker *worker[T]) extractTrace(ctx context.Context, metadata []byte) context.Context {
	if len(metadata) == 0 {
		return ctx
	}
	var value struct {
		Trace map[string]string `json:"soro_trace"`
	}
	if err := json.Unmarshal(metadata, &value); err != nil {
		return ctx
	}
	return worker.client.observer.Propagator().Extract(ctx, propagation.MapCarrier(value.Trace))
}
