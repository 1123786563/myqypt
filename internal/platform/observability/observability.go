// Package observability constructs the process-wide observability
// resources — the JSON slog logger and the OpenTelemetry SDK tracer and
// meter providers — behind one explicit composition function. Nothing here
// touches a global: no slog.SetDefault and no otel global providers; every
// consumer receives its resource by injection.
package observability

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"os"
	"sync"

	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetricgrpc"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	// semconv v1.26.0 is the newest stable vocabulary that still carries
	// deployment.environment under its long-standing key; newer sets rename
	// it to deployment.environment.name.
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"
)

// Config is the composition input for New. OTLPEndpoint is the collector's
// host:port for the OTLP gRPC protocol; when empty, the providers are
// constructed without exporters (spans and metrics stay in-process, which
// keeps local and test runs network-free). LogWriter defaults to os.Stdout
// so tests can capture the JSON records.
type Config struct {
	ServiceName           string
	ServiceVersion        string
	DeploymentEnvironment string
	OTLPEndpoint          string
	LogWriter             io.Writer
}

// Resources is the outcome of New: the explicitly injected logger, tracer
// provider and meter provider, the identity resource both providers were
// built on (the SDK providers expose no Resource accessor of their own),
// plus an idempotent Shutdown that aggregates both providers' shutdown
// errors. Call Shutdown once the consumers are done (after the HTTP server
// has drained) with a context carrying a timeout.
type Resources struct {
	Logger         *slog.Logger
	TracerProvider *sdktrace.TracerProvider
	MeterProvider  *sdkmetric.MeterProvider
	Resource       *resource.Resource
	Shutdown       func(ctx context.Context) error
}

// New constructs the observability resources. Any construction failure is
// returned as an error so the composition root can fail startup instead of
// serving blind. The SDK providers are always real (never the otel
// globals): without an endpoint they simply have no exporters.
func New(ctx context.Context, config Config) (Resources, error) {
	writer := config.LogWriter
	if writer == nil {
		writer = os.Stdout
	}
	logger := slog.New(slog.NewJSONHandler(writer, nil))

	res, err := resource.New(ctx, resource.WithAttributes(
		semconv.ServiceName(config.ServiceName),
		semconv.ServiceVersion(config.ServiceVersion),
		semconv.DeploymentEnvironment(config.DeploymentEnvironment),
	))
	if err != nil {
		return Resources{}, err
	}

	var spanOptions []sdktrace.TracerProviderOption
	var metricOptions []sdkmetric.Option
	if config.OTLPEndpoint != "" {
		traceExporter, err := otlptracegrpc.New(ctx, otlptracegrpc.WithEndpoint(config.OTLPEndpoint))
		if err != nil {
			return Resources{}, err
		}
		spanOptions = append(spanOptions, sdktrace.WithBatcher(traceExporter))
		metricExporter, err := otlpmetricgrpc.New(ctx, otlpmetricgrpc.WithEndpoint(config.OTLPEndpoint))
		if err != nil {
			return Resources{}, err
		}
		metricOptions = append(metricOptions, sdkmetric.WithReader(sdkmetric.NewPeriodicReader(metricExporter)))
	}

	tracerProvider := sdktrace.NewTracerProvider(append(spanOptions, sdktrace.WithResource(res))...)
	meterProvider := sdkmetric.NewMeterProvider(append(metricOptions, sdkmetric.WithResource(res))...)

	var once sync.Once
	var shutdownErr error
	return Resources{
		Logger:         logger,
		TracerProvider: tracerProvider,
		MeterProvider:  meterProvider,
		Resource:       res,
		Shutdown: func(ctx context.Context) error {
			// Idempotent by construction: the second and later calls
			// return the cached aggregate instead of re-shutting-down
			// the providers.
			once.Do(func() {
				shutdownErr = errors.Join(tracerProvider.Shutdown(ctx), meterProvider.Shutdown(ctx))
			})
			return shutdownErr
		},
	}, nil
}
