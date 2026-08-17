package observability

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
)

type Metrics struct {
	httpRequests  metric.Int64Counter
	httpDuration  metric.Float64Histogram
	dbDuration    metric.Float64Histogram
	jobsProcessed metric.Int64Counter
	jobsFailed    metric.Int64Counter
	jobDuration   metric.Float64Histogram
	mailSent      metric.Int64Counter
	mailFailed    metric.Int64Counter
	mailDuration  metric.Float64Histogram
}

func newMetrics(meter metric.Meter) (*Metrics, error) {
	metrics := &Metrics{}
	var err error
	if metrics.httpRequests, err = meter.Int64Counter("soro.http.requests", metric.WithDescription("HTTP requests processed")); err != nil {
		return nil, fmt.Errorf("create HTTP request counter: %w", err)
	}
	if metrics.httpDuration, err = meter.Float64Histogram("soro.http.request.duration", metric.WithUnit("s"), metric.WithDescription("HTTP request duration")); err != nil {
		return nil, fmt.Errorf("create HTTP duration histogram: %w", err)
	}
	if metrics.dbDuration, err = meter.Float64Histogram("soro.db.query.duration", metric.WithUnit("s"), metric.WithDescription("Database query duration")); err != nil {
		return nil, fmt.Errorf("create database duration histogram: %w", err)
	}
	if metrics.jobsProcessed, err = meter.Int64Counter("soro.jobs.processed", metric.WithDescription("Jobs processed")); err != nil {
		return nil, fmt.Errorf("create jobs counter: %w", err)
	}
	if metrics.jobsFailed, err = meter.Int64Counter("soro.jobs.failed", metric.WithDescription("Job attempts failed")); err != nil {
		return nil, fmt.Errorf("create failed jobs counter: %w", err)
	}
	if metrics.jobDuration, err = meter.Float64Histogram("soro.job.duration", metric.WithUnit("s"), metric.WithDescription("Job run duration")); err != nil {
		return nil, fmt.Errorf("create job duration histogram: %w", err)
	}
	if metrics.mailSent, err = meter.Int64Counter("soro.mail.sent", metric.WithDescription("Messages sent")); err != nil {
		return nil, fmt.Errorf("create mail counter: %w", err)
	}
	if metrics.mailFailed, err = meter.Int64Counter("soro.mail.failed", metric.WithDescription("Message sends failed")); err != nil {
		return nil, fmt.Errorf("create failed mail counter: %w", err)
	}
	if metrics.mailDuration, err = meter.Float64Histogram("soro.mail.duration", metric.WithUnit("s"), metric.WithDescription("Mail send duration")); err != nil {
		return nil, fmt.Errorf("create mail duration histogram: %w", err)
	}
	return metrics, nil
}

func (metrics *Metrics) RecordDatabase(ctx context.Context, operation, status string, duration time.Duration) {
	attrs := metric.WithAttributes(attribute.String("db.operation.name", operation), attribute.String("db.response.status", status))
	metrics.dbDuration.Record(ctx, duration.Seconds(), attrs)
}

func (metrics *Metrics) RecordHTTP(ctx context.Context, method string, status int, duration time.Duration) {
	attrs := metric.WithAttributes(attribute.String("http.request.method", method), attribute.String("http.response.status_code", strconv.Itoa(status)))
	metrics.httpRequests.Add(ctx, 1, attrs)
	metrics.httpDuration.Record(ctx, duration.Seconds(), attrs)
}

func (metrics *Metrics) RecordJob(ctx context.Context, kind, queue string, attempts int, duration time.Duration, err error) {
	attrs := metric.WithAttributes(attribute.String("job.kind", kind), attribute.String("job.queue", queue))
	metrics.jobsProcessed.Add(ctx, 1, attrs)
	if err != nil {
		metrics.jobsFailed.Add(ctx, 1, attrs)
	}
	metrics.jobDuration.Record(ctx, duration.Seconds(), attrs)
}

func (metrics *Metrics) RecordMail(ctx context.Context, transport string, duration time.Duration, err error) {
	attrs := metric.WithAttributes(attribute.String("mail.transport", transport))
	if err != nil {
		metrics.mailFailed.Add(ctx, 1, attrs)
	} else {
		metrics.mailSent.Add(ctx, 1, attrs)
	}
	metrics.mailDuration.Record(ctx, duration.Seconds(), attrs)
}
