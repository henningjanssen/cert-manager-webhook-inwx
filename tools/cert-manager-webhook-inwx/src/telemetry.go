package main

import (
	"context"
	"fmt"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/propagation"
	sdkresource "go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
)

// instrumentationName is the OTel tracer/meter name used for all
// instrumentation emitted by this process.
const instrumentationName = "github.com/henningjanssen/cert-manager-webhook-inwx"

// initTracer configures the global OpenTelemetry tracer provider.
//
// When endpoint is empty, no provider is installed and the default no-op
// implementation remains in effect — spans are silently discarded.
// When endpoint is set (e.g. "localhost:4317"), an OTLP/gRPC exporter is
// created and the global provider is updated. Standard OTel environment
// variables (OTEL_SERVICE_NAME, OTEL_RESOURCE_ATTRIBUTES, etc.) are honoured.
//
// The returned shutdown function must be called before program exit to flush
// pending spans and free resources.
func initTracer(ctx context.Context, endpoint string) (func(context.Context) error, error) {
	if endpoint == "" {
		return func(_ context.Context) error { return nil }, nil
	}

	exporter, err := otlptracegrpc.New(ctx, otlptracegrpc.WithEndpoint(endpoint))
	if err != nil {
		return nil, fmt.Errorf("failed to create OTLP/gRPC trace exporter: %w", err)
	}

	// WithFromEnv reads OTEL_SERVICE_NAME and OTEL_RESOURCE_ATTRIBUTES.
	// Errors from individual detectors are non-fatal; the resource is still
	// usable with whatever data was successfully collected.
	res, _ := sdkresource.New(ctx,
		sdkresource.WithFromEnv(),
		sdkresource.WithProcess(),
	)

	tp := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exporter, sdktrace.WithBatchTimeout(5*time.Second)),
		sdktrace.WithResource(res),
	)

	otel.SetTracerProvider(tp)
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	))

	return tp.Shutdown, nil
}
