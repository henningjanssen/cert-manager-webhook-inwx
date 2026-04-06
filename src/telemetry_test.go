package main

import (
	"context"
	"testing"

	"go.opentelemetry.io/otel"
)

func TestInitTracer_EmptyEndpoint_ReturnsNilError(t *testing.T) {
	shutdown, err := initTracer(context.Background(), "")
	if err != nil {
		t.Fatalf("initTracer with empty endpoint must not fail: %v", err)
	}
	if shutdown == nil {
		t.Fatal("expected non-nil shutdown function")
	}
	// Shutdown of a no-op path must also succeed.
	if err := shutdown(context.Background()); err != nil {
		t.Fatalf("shutdown must not fail: %v", err)
	}
}

func TestInitTracer_EmptyEndpoint_GlobalTracerRemainsNoop(t *testing.T) {
	// Calling initTracer with no endpoint must NOT replace the global noop
	// TracerProvider that OTel installs by default.
	_, _ = initTracer(context.Background(), "")

	tracer := otel.Tracer(instrumentationName)
	_, span := tracer.Start(context.Background(), "test-op")
	defer span.End()

	// A noop span's SpanContext is invalid (all-zero trace/span IDs).
	if span.SpanContext().IsValid() {
		t.Error("expected invalid (noop) SpanContext when no OTLP endpoint is configured")
	}
}

func TestInitTracer_EmptyEndpoint_IsIdempotent(t *testing.T) {
	// Multiple calls with an empty endpoint should not conflict.
	for i := 0; i < 3; i++ {
		shutdown, err := initTracer(context.Background(), "")
		if err != nil {
			t.Fatalf("call %d: unexpected error: %v", i, err)
		}
		if err := shutdown(context.Background()); err != nil {
			t.Fatalf("call %d: unexpected shutdown error: %v", i, err)
		}
	}
}
