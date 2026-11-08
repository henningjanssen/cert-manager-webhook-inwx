package main

import (
	"context"
	"fmt"
	"os"

	"github.com/cert-manager/cert-manager/pkg/acme/webhook/cmd"
)

// GroupName is the Kubernetes API group name for this webhook solver.
// It must be unique and is set via the GROUP_NAME environment variable.
var GroupName = os.Getenv("GROUP_NAME")

func main() {
	if GroupName == "" {
		panic("GROUP_NAME environment variable must be specified")
	}

	// ── OpenTelemetry ──────────────────────────────────────────────────────
	// Reads OTEL_EXPORTER_OTLP_ENDPOINT. When empty, tracing is a no-op.
	otelShutdown, err := initTracer(context.Background(), os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT"))
	if err != nil {
		fmt.Fprintf(os.Stderr, "warning: could not initialise OTel tracer: %v\n", err)
		// Non-fatal: continue without tracing.
		otelShutdown = func(_ context.Context) error { return nil }
	}
	defer otelShutdown(context.Background()) //nolint:errcheck

	// ── Health + metrics server ────────────────────────────────────────────
	// Always serve /healthz and /metrics on 0.0.0.0:8080.
	startMetricsServer("0.0.0.0:8080")

	cmd.RunWebhookServer(GroupName, newSolver())
}
