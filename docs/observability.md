# Observability and health

Every application owns an OpenTelemetry tracer provider, meter provider, W3C propagator, and Prometheus registry. Soro passes these providers explicitly and does not require process-global OTel configuration.

Set an OTLP HTTP base endpoint to export traces and metrics:

```sh
export SORO_OTEL_ENABLED=true
export OTEL_EXPORTER_OTLP_ENDPOINT=http://localhost:4318
```

Without an endpoint, Prometheus metrics remain available locally and trace export performs no network calls. Applications can create child spans with:

```go
ctx, span := app.Observability.Tracer().Start(ctx, "recalculate account")
defer span.End()
```

## HTTP logging and tracing

The API middleware extracts W3C trace context and records server spans, request counts, and durations. Each request produces one `slog` event containing method, path, status, duration, request ID, remote IP, and trace ID when present. `HTTPOptions.Fields` adds application context, and `RedactPath` can remove sensitive path segments.

Soro does not log request bodies, authorization headers, SQL, bind values, email addresses, or job arguments. Database spans contain only a fixed operation class such as `SELECT` or `UPDATE`.

Prometheus exports include:

```text
soro_http_requests_total
soro_http_request_duration_seconds
soro_db_query_duration_seconds
soro_jobs_processed_total
soro_jobs_failed_total
soro_job_duration_seconds
soro_mail_sent_total
soro_mail_failed_total
soro_mail_duration_seconds
```

Labels are bounded to method, status, operation, job kind, queue, and transport. Arbitrary IDs and paths are not metric labels.

## Infrastructure endpoints

- `GET /health` confirms the process and HTTP stack are alive without querying dependencies.
- `GET /ready` runs registered readiness checks under `http.readiness_timeout`; PostgreSQL is registered by default.
- `GET /metrics` serves the application-owned Prometheus registry.

Register another required dependency with:

```go
err := app.Health.Register("billing", func(ctx context.Context) error {
	return billingClient.Ping(ctx)
})
```

A failed readiness check returns HTTP 503 with only named `ok`/`failed` states; internal dependency errors are not exposed.
