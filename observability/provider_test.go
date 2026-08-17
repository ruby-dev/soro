package observability_test

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/ruby-dev/soro/observability"
	"github.com/uptrace/bun"
)

func TestHTTPMiddlewareRecordsMetricsAndSafeLog(t *testing.T) {
	provider, err := observability.New(t.Context(), observability.Config{ServiceName: "test", Environment: "test"})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := provider.Shutdown(context.Background()); err != nil {
			t.Error(err)
		}
	})
	var logs bytes.Buffer
	middleware := provider.HTTPMiddleware(observability.HTTPOptions{
		Logger:    slog.New(slog.NewJSONHandler(&logs, nil)),
		RequestID: func(context.Context) string { return "request-1" },
	})
	recorder := httptest.NewRecorder()
	middleware(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.WriteHeader(http.StatusCreated)
	})).ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/users/secret@example.com", nil))
	if recorder.Code != http.StatusCreated {
		t.Fatalf("status = %d", recorder.Code)
	}
	if !strings.Contains(logs.String(), `"request_id":"request-1"`) || strings.Contains(logs.String(), "Authorization") {
		t.Fatalf("unexpected log: %s", logs.String())
	}

	metrics := httptest.NewRecorder()
	provider.Metrics().RecordDatabase(t.Context(), "SELECT", "ok", time.Millisecond)
	provider.Metrics().RecordJob(t.Context(), "example", "default", 1, time.Millisecond, nil)
	provider.Metrics().RecordMail(t.Context(), "capture", time.Millisecond, nil)
	provider.Handler().ServeHTTP(metrics, httptest.NewRequest(http.MethodGet, "/metrics", nil))
	for _, name := range []string{
		"soro_http_requests_total", "soro_http_request_duration_seconds", "soro_db_query_duration_seconds",
		"soro_jobs_processed_total", "soro_job_duration_seconds", "soro_mail_sent_total", "soro_mail_duration_seconds",
	} {
		if !strings.Contains(metrics.Body.String(), name) {
			t.Fatalf("metrics missing %s:\n%s", name, metrics.Body.String())
		}
	}
}

func TestConfigAndShutdown(t *testing.T) {
	if _, err := observability.New(t.Context(), observability.Config{}); err == nil {
		t.Fatal("expected service name error")
	}
	provider, err := observability.New(t.Context(), observability.Config{ServiceName: "test"})
	if err != nil {
		t.Fatal(err)
	}
	if err := provider.Shutdown(t.Context()); err != nil {
		t.Fatal(err)
	}
	if err := provider.Shutdown(t.Context()); err != nil {
		t.Fatal(err)
	}
}

func TestDatabaseHookRecordsOperationWithoutStatement(t *testing.T) {
	provider, err := observability.New(t.Context(), observability.Config{ServiceName: "test"})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = provider.Shutdown(context.Background()) })
	hook := provider.DatabaseHook()
	event := &bun.QueryEvent{Query: "SELECT secret FROM users", StartTime: time.Now().Add(-time.Millisecond)}
	ctx := hook.BeforeQuery(t.Context(), event)
	event.Err = errors.New("database error")
	hook.AfterQuery(ctx, event)
	_ = provider.Meter()
	metrics := httptest.NewRecorder()
	provider.Handler().ServeHTTP(metrics, httptest.NewRequest(http.MethodGet, "/metrics", nil))
	if !strings.Contains(metrics.Body.String(), `db_operation_name="SELECT"`) || strings.Contains(metrics.Body.String(), "secret") {
		t.Fatalf("unexpected database metrics:\n%s", metrics.Body.String())
	}
}
