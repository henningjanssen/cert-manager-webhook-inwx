package main

import (
	"errors"
	"net/http"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"k8s.io/klog/v2"
)

const (
	labelOperation = "operation"
	labelOutcome   = "outcome"
	outcomeSuccess = "success"
	outcomeError   = "error"
	opPresent      = "present"
	opCleanup      = "cleanup"
)

// solverMetrics holds Prometheus metric vectors for the INWX DNS01 solver.
type solverMetrics struct {
	operationTotal    *prometheus.CounterVec
	operationDuration *prometheus.HistogramVec
}

// newSolverMetrics creates and registers solver metrics with reg.
func newSolverMetrics(reg prometheus.Registerer) *solverMetrics {
	m := &solverMetrics{
		operationTotal: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: "inwx",
				Subsystem: "webhook",
				Name:      "operations_total",
				Help:      "Total number of DNS01 webhook operations partitioned by operation and outcome.",
			},
			[]string{labelOperation, labelOutcome},
		),
		operationDuration: prometheus.NewHistogramVec(
			prometheus.HistogramOpts{
				Namespace: "inwx",
				Subsystem: "webhook",
				Name:      "operation_duration_seconds",
				Help:      "Duration in seconds of DNS01 webhook operations partitioned by operation and outcome.",
				Buckets:   prometheus.DefBuckets,
			},
			[]string{labelOperation, labelOutcome},
		),
	}
	reg.MustRegister(m.operationTotal, m.operationDuration)
	return m
}

// record records an operation outcome and its duration.
// It is safe to call on a nil *solverMetrics (no-op).
func (m *solverMetrics) record(op string, err error, d time.Duration) {
	if m == nil {
		return
	}
	outcome := outcomeSuccess
	if err != nil {
		outcome = outcomeError
	}
	m.operationTotal.WithLabelValues(op, outcome).Inc()
	m.operationDuration.WithLabelValues(op, outcome).Observe(d.Seconds())
}

// defaultRegisterer returns prometheus.DefaultRegisterer.
// It exists as an indirection so tests can verify wiring without relying on
// global state.
func defaultRegisterer() prometheus.Registerer {
	return prometheus.DefaultRegisterer
}

// startMetricsServer starts an HTTP server on addr exposing /metrics and
// /healthz. The call is non-blocking; the server runs until the process exits.
func startMetricsServer(addr string) {
	mux := http.NewServeMux()
	mux.Handle("/metrics", promhttp.Handler())
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	go func() {
		if err := http.ListenAndServe(addr, mux); err != nil && !errors.Is(err, http.ErrServerClosed) { //nolint:gosec
			klog.Errorf("metrics server error: %v", err)
		}
	}()
}
