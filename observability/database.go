package observability

import (
	"context"
	"strings"
	"time"

	"github.com/uptrace/bun"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

type DatabaseHook struct{ provider *Provider }

func (provider *Provider) DatabaseHook() *DatabaseHook { return &DatabaseHook{provider: provider} }

func (hook *DatabaseHook) BeforeQuery(ctx context.Context, event *bun.QueryEvent) context.Context {
	operation := safeOperation(event.Operation())
	ctx, _ = hook.provider.Tracer().Start(ctx, "db "+operation, trace.WithSpanKind(trace.SpanKindClient), trace.WithAttributes(
		attribute.String("db.system.name", "postgresql"), attribute.String("db.operation.name", operation),
	))
	return ctx
}

func (hook *DatabaseHook) AfterQuery(ctx context.Context, event *bun.QueryEvent) {
	span := trace.SpanFromContext(ctx)
	status := "ok"
	if event.Err != nil {
		status = "error"
		span.RecordError(event.Err)
		span.SetStatus(codes.Error, "database query failed")
	}
	span.End()
	hook.provider.metrics.RecordDatabase(ctx, safeOperation(event.Operation()), status, time.Since(event.StartTime))
}

func safeOperation(operation string) string {
	operation = strings.ToUpper(operation)
	switch operation {
	case "SELECT", "INSERT", "UPDATE", "DELETE", "CREATE", "ALTER", "DROP", "TRUNCATE", "BEGIN", "COMMIT", "ROLLBACK":
		return operation
	default:
		return "OTHER"
	}
}
