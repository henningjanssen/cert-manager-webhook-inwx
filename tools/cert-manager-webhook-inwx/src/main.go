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
	// Each signal is enabled via OTEL_{TRACES,METRICS,LOGS}_EXPORTER; unset is a no-op.
	otelShutdown, err := initTelemetry(context.Background())
	if err != nil {
		// Non-fatal: continue with whatever signals did initialise.
		fmt.Fprintf(os.Stderr, "warning: could not initialise OTel: %v\n", err)
	}
	defer otelShutdown(context.Background()) //nolint:errcheck

	// ── Health + metrics server ────────────────────────────────────────────
	// Always serve /healthz and /metrics on 0.0.0.0:8080.
	startMetricsServer("0.0.0.0:8080")

	cmd.RunWebhookServer(GroupName, newSolver())
}
