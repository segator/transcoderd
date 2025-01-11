package tracing

import (
	"context"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/exporters/stdout/stdouttrace"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.21.0"
	"go.opentelemetry.io/otel/trace"
	"go.opentelemetry.io/otel/trace/noop"
	"io"
)

type TracerProvider struct {
	tp trace.TracerProvider
}

func NewTracing(ctx context.Context, config *TracingConfig) (tracerProvider trace.TracerProvider, err error) {
	if !config.Enabled {
		return noop.NewTracerProvider(), nil
	}

	// Change default HTTPS -> HTTP
	insecureOpt := otlptracehttp.WithInsecure()

	// Update default OTLP reciver endpoint
	endpointOpt := otlptracehttp.WithEndpoint(config.Endpoint)
	var spanExporter sdktrace.SpanExporter
	spanExporter, err = stdouttrace.New(stdouttrace.WithWriter(io.Discard))
	if err != nil {
		return nil, err
	}
	if config.Enabled {
		spanExporter, err = otlptracehttp.New(ctx, insecureOpt, endpointOpt)
		if err != nil {
			return nil, err
		}
	}
	resources, err := resource.Merge(
		resource.Default(),
		resource.NewWithAttributes(
			semconv.SchemaURL,
			semconv.ServiceName(config.ServiceName),
		),
	)

	if err != nil {
		panic(err)
	}
	opTracerProvider := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(spanExporter),
		sdktrace.WithResource(resources),
	)
	go func() {
		<-ctx.Done()
		opTracerProvider.Shutdown(context.Background())
	}()

	return tracerProvider, nil
}
