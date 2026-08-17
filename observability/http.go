package observability

import (
	"context"
	"log/slog"
	"net"
	"net/http"
	"time"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"
)

type HTTPOptions struct {
	Logger     *slog.Logger
	RequestID  func(context.Context) string
	Fields     func(context.Context) []slog.Attr
	RedactPath func(string) string
}

func (provider *Provider) HTTPMiddleware(options HTTPOptions) func(http.Handler) http.Handler {
	logger := options.Logger
	if logger == nil {
		logger = slog.Default()
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
			started := time.Now()
			ctx := provider.propagator.Extract(request.Context(), propagation.HeaderCarrier(request.Header))
			ctx, span := provider.Tracer().Start(ctx, "HTTP "+request.Method,
				trace.WithSpanKind(trace.SpanKindServer),
				trace.WithAttributes(attribute.String("http.request.method", request.Method)))
			defer span.End()
			response := &statusWriter{ResponseWriter: writer, status: http.StatusOK}
			next.ServeHTTP(response, request.WithContext(ctx))
			duration := time.Since(started)
			provider.metrics.RecordHTTP(ctx, request.Method, response.status, duration)
			span.SetAttributes(attribute.Int("http.response.status_code", response.status))
			if response.status >= 500 {
				span.SetStatus(codes.Error, http.StatusText(response.status))
			}
			path := request.URL.Path
			if options.RedactPath != nil {
				path = options.RedactPath(path)
			}
			fields := []any{
				"method", request.Method, "path", path, "status", response.status,
				"duration", duration, "remote_ip", remoteIP(request.RemoteAddr),
			}
			if options.RequestID != nil {
				fields = append(fields, "request_id", options.RequestID(ctx))
			}
			if spanContext := trace.SpanContextFromContext(ctx); spanContext.IsValid() {
				fields = append(fields, "trace_id", spanContext.TraceID().String())
			}
			if options.Fields != nil {
				for _, field := range options.Fields(ctx) {
					fields = append(fields, field)
				}
			}
			logger.Log(ctx, levelForStatus(response.status), "HTTP request", fields...)
		})
	}
}

type statusWriter struct {
	http.ResponseWriter
	status      int
	wroteHeader bool
}

func (writer *statusWriter) WriteHeader(status int) {
	if writer.wroteHeader {
		return
	}
	writer.status = status
	writer.wroteHeader = true
	writer.ResponseWriter.WriteHeader(status)
}

func (writer *statusWriter) Write(body []byte) (int, error) {
	if !writer.wroteHeader {
		writer.WriteHeader(http.StatusOK)
	}
	return writer.ResponseWriter.Write(body)
}

func levelForStatus(status int) slog.Level {
	if status >= 500 {
		return slog.LevelError
	}
	if status >= 400 {
		return slog.LevelWarn
	}
	return slog.LevelInfo
}

func remoteIP(remoteAddress string) string {
	host, _, err := net.SplitHostPort(remoteAddress)
	if err == nil {
		return host
	}
	return remoteAddress
}
