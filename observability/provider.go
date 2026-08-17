// Package observability owns Soro tracing, metrics, and HTTP instrumentation.
package observability

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetrichttp"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	otelprom "go.opentelemetry.io/otel/exporters/prometheus"
	metricapi "go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/trace"
)

const instrumentationName = "github.com/datasoro/soro"

type Config struct {
	ServiceName  string
	Environment  string
	Version      string
	OTLPEndpoint string
	OTLPTimeout  time.Duration
}

func (config Config) Validate() error {
	if strings.TrimSpace(config.ServiceName) == "" {
		return fmt.Errorf("observability service name is required")
	}
	if config.OTLPTimeout < 0 {
		return fmt.Errorf("observability OTLP timeout cannot be negative")
	}
	return nil
}

type Provider struct {
	tracerProvider *sdktrace.TracerProvider
	meterProvider  *metric.MeterProvider
	registry       *prometheus.Registry
	metrics        *Metrics
	propagator     propagation.TextMapPropagator
	closeOnce      sync.Once
	closeErr       error
}

func New(ctx context.Context, config Config) (*Provider, error) {
	if config.OTLPTimeout == 0 {
		config.OTLPTimeout = 10 * time.Second
	}
	if err := config.Validate(); err != nil {
		return nil, err
	}
	attributes := []attribute.KeyValue{
		attribute.String("service.name", config.ServiceName),
		attribute.String("deployment.environment.name", config.Environment),
	}
	if config.Version != "" {
		attributes = append(attributes, attribute.String("service.version", config.Version))
	}
	serviceResource := resource.NewWithAttributes("", attributes...)

	registry := prometheus.NewRegistry()
	prometheusReader, err := otelprom.New(otelprom.WithRegisterer(registry), otelprom.WithoutScopeInfo(), otelprom.WithoutTargetInfo())
	if err != nil {
		return nil, fmt.Errorf("observability: create Prometheus exporter: %w", err)
	}
	meterOptions := []metric.Option{metric.WithResource(serviceResource), metric.WithReader(prometheusReader)}
	traceOptions := []sdktrace.TracerProviderOption{sdktrace.WithResource(serviceResource), sdktrace.WithSampler(sdktrace.NeverSample())}
	if config.OTLPEndpoint != "" {
		traceExporter, exportErr := otlptracehttp.New(ctx,
			otlptracehttp.WithEndpointURL(config.OTLPEndpoint), otlptracehttp.WithTimeout(config.OTLPTimeout))
		if exportErr != nil {
			return nil, fmt.Errorf("observability: create OTLP trace exporter: %w", exportErr)
		}
		metricExporter, exportErr := otlpmetrichttp.New(ctx,
			otlpmetrichttp.WithEndpointURL(config.OTLPEndpoint), otlpmetrichttp.WithTimeout(config.OTLPTimeout))
		if exportErr != nil {
			_ = traceExporter.Shutdown(ctx)
			return nil, fmt.Errorf("observability: create OTLP metric exporter: %w", exportErr)
		}
		traceOptions = []sdktrace.TracerProviderOption{sdktrace.WithResource(serviceResource), sdktrace.WithBatcher(traceExporter)}
		meterOptions = append(meterOptions, metric.WithReader(metric.NewPeriodicReader(metricExporter)))
	}

	tracerProvider := sdktrace.NewTracerProvider(traceOptions...)
	meterProvider := metric.NewMeterProvider(meterOptions...)
	instruments, err := newMetrics(meterProvider.Meter(instrumentationName))
	if err != nil {
		_ = tracerProvider.Shutdown(ctx)
		_ = meterProvider.Shutdown(ctx)
		return nil, err
	}
	return &Provider{
		tracerProvider: tracerProvider, meterProvider: meterProvider, registry: registry, metrics: instruments,
		propagator: propagation.NewCompositeTextMapPropagator(propagation.TraceContext{}, propagation.Baggage{}),
	}, nil
}

func (provider *Provider) Tracer() trace.Tracer {
	return provider.tracerProvider.Tracer(instrumentationName)
}

func (provider *Provider) Meter() metricapi.Meter {
	return provider.meterProvider.Meter(instrumentationName)
}

func (provider *Provider) Metrics() *Metrics { return provider.metrics }

func (provider *Provider) Propagator() propagation.TextMapPropagator { return provider.propagator }

func (provider *Provider) Handler() http.Handler {
	return promhttp.HandlerFor(provider.registry, promhttp.HandlerOpts{})
}

func (provider *Provider) Shutdown(ctx context.Context) error {
	if provider == nil {
		return nil
	}
	provider.closeOnce.Do(func() {
		provider.closeErr = errors.Join(provider.meterProvider.Shutdown(ctx), provider.tracerProvider.Shutdown(ctx))
	})
	return provider.closeErr
}
