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
	// Always start a lightweight HTTP server for Kubernetes probes (/healthz).
	// When METRICS_ADDR is set, /metrics is also exposed on the same server.
	// Defaults to 127.0.0.1:8080; override via METRICS_ADDR.
	healthAddr := os.Getenv("METRICS_ADDR")
	if healthAddr == "" {
		healthAddr = "127.0.0.1:8080"
	}
	startMetricsServer(healthAddr)

	cmd.RunWebhookServer(GroupName, newSolver())
}
