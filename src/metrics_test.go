package main

import (
	"errors"
	"testing"
	"time"

	"github.com/nrdcg/goinwx"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
	k8sruntime "k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/fake"
)

// buildSolverWithMetrics returns a solver wired to a fresh Prometheus
// registry so tests can assert recorded metric values without touching the
// global DefaultRegisterer.
func buildSolverWithMetrics(fc *fakeInwxClient, objs ...k8sruntime.Object) (*inwxDNSSolver, *prometheus.Registry) {
	reg := prometheus.NewRegistry()
	return &inwxDNSSolver{
		kubeClient: fake.NewSimpleClientset(objs...),
		newClient:  func(_, _ string) inwxClient { return fc },
		metrics:    newSolverMetrics(reg),
	}, reg
}

// ---------------------------------------------------------------------------
// record() helper
// ---------------------------------------------------------------------------

func TestRecord_SuccessIncrementsTotalAndDuration(t *testing.T) {
	reg := prometheus.NewRegistry()
	m := newSolverMetrics(reg)

	m.record(opPresent, nil, 10*time.Millisecond)

	if got := testutil.ToFloat64(m.operationTotal.WithLabelValues(opPresent, outcomeSuccess)); got != 1 {
		t.Errorf("operations_total{operation=present,outcome=success}: got %v, want 1", got)
	}
	mfs, err := reg.Gather()
	if err != nil {
		t.Fatalf("failed to gather metrics: %v", err)
	}
	var durationSum float64
	for _, mf := range mfs {
		if mf.GetName() == "inwx_webhook_operation_duration_seconds" {
			for _, metric := range mf.GetMetric() {
				durationSum += metric.GetHistogram().GetSampleSum()
			}
		}
	}
	if durationSum <= 0 {
		t.Errorf("expected positive duration observation, got %v", durationSum)
	}
}

func TestRecord_ErrorIncrementsErrorLabel(t *testing.T) {
	reg := prometheus.NewRegistry()
	m := newSolverMetrics(reg)

	m.record(opCleanup, errors.New("boom"), 5*time.Millisecond)

	if got := testutil.ToFloat64(m.operationTotal.WithLabelValues(opCleanup, outcomeError)); got != 1 {
		t.Errorf("operations_total{cleanup/error}: got %v, want 1", got)
	}
	if got := testutil.ToFloat64(m.operationTotal.WithLabelValues(opCleanup, outcomeSuccess)); got != 0 {
		t.Errorf("expected 0 success observations, got %v", got)
	}
}

func TestRecord_NilReceiverIsNoop(t *testing.T) {
	var m *solverMetrics
	// Must not panic.
	m.record(opPresent, nil, time.Second)
}

func TestRecord_AccumulatesMultipleCalls(t *testing.T) {
	reg := prometheus.NewRegistry()
	m := newSolverMetrics(reg)

	for i := 0; i < 5; i++ {
		m.record(opPresent, nil, time.Millisecond)
	}
	m.record(opPresent, errors.New("err"), time.Millisecond)

	if got := testutil.ToFloat64(m.operationTotal.WithLabelValues(opPresent, outcomeSuccess)); got != 5 {
		t.Errorf("expected 5 success observations, got %v", got)
	}
	if got := testutil.ToFloat64(m.operationTotal.WithLabelValues(opPresent, outcomeError)); got != 1 {
		t.Errorf("expected 1 error observation, got %v", got)
	}
}

// ---------------------------------------------------------------------------
// metrics recorded through Present / CleanUp
// ---------------------------------------------------------------------------

func TestPresentRecordsSuccessMetric(t *testing.T) {
	fc := &fakeInwxClient{infoResp: &goinwx.NameserverInfoResponse{}}
	s, _ := buildSolverWithMetrics(fc, testSecret("inwx-credentials", "default"))

	if err := s.Present(testChallenge("default", "_acme-challenge.example.com.", "example.com.", "tok")); err != nil {
		t.Fatalf("Present() unexpected error: %v", err)
	}

	if got := testutil.ToFloat64(s.metrics.operationTotal.WithLabelValues(opPresent, outcomeSuccess)); got != 1 {
		t.Errorf("expected 1 present/success, got %v", got)
	}
	if got := testutil.ToFloat64(s.metrics.operationTotal.WithLabelValues(opPresent, outcomeError)); got != 0 {
		t.Errorf("expected 0 present/error, got %v", got)
	}
}

func TestPresentRecordsErrorMetric(t *testing.T) {
	fc := &fakeInwxClient{createErr: errors.New("API error")}
	s, _ := buildSolverWithMetrics(fc, testSecret("inwx-credentials", "default"))

	if err := s.Present(testChallenge("default", "_acme-challenge.example.com.", "example.com.", "tok")); err == nil {
		t.Fatal("expected error, got nil")
	}

	if got := testutil.ToFloat64(s.metrics.operationTotal.WithLabelValues(opPresent, outcomeError)); got != 1 {
		t.Errorf("expected 1 present/error, got %v", got)
	}
	if got := testutil.ToFloat64(s.metrics.operationTotal.WithLabelValues(opPresent, outcomeSuccess)); got != 0 {
		t.Errorf("expected 0 present/success, got %v", got)
	}
}

func TestCleanUpRecordsSuccessMetric(t *testing.T) {
	fc := &fakeInwxClient{
		infoResp: &goinwx.NameserverInfoResponse{
			Records: []goinwx.NameserverRecord{{ID: 1, Content: "tok"}},
		},
	}
	s, _ := buildSolverWithMetrics(fc, testSecret("inwx-credentials", "default"))

	if err := s.CleanUp(testChallenge("default", "_acme-challenge.example.com.", "example.com.", "tok")); err != nil {
		t.Fatalf("CleanUp() unexpected error: %v", err)
	}

	if got := testutil.ToFloat64(s.metrics.operationTotal.WithLabelValues(opCleanup, outcomeSuccess)); got != 1 {
		t.Errorf("expected 1 cleanup/success, got %v", got)
	}
}

func TestCleanUpRecordsErrorMetric(t *testing.T) {
	fc := &fakeInwxClient{infoErr: errors.New("info error")}
	s, _ := buildSolverWithMetrics(fc, testSecret("inwx-credentials", "default"))

	if err := s.CleanUp(testChallenge("default", "_acme-challenge.example.com.", "example.com.", "tok")); err == nil {
		t.Fatal("expected error, got nil")
	}

	if got := testutil.ToFloat64(s.metrics.operationTotal.WithLabelValues(opCleanup, outcomeError)); got != 1 {
		t.Errorf("expected 1 cleanup/error, got %v", got)
	}
}

// ---------------------------------------------------------------------------
// registry behaviour
// ---------------------------------------------------------------------------

func TestNewSolverMetrics_DoubleRegistrationPanics(t *testing.T) {
	reg := prometheus.NewRegistry()
	_ = newSolverMetrics(reg)
	defer func() {
		if r := recover(); r == nil {
			t.Error("expected panic on double registration in the same registry, got none")
		}
	}()
	_ = newSolverMetrics(reg) // must panic with AlreadyRegisteredError
}

func TestNewSolverMetrics_IndependentRegistriesDoNotConflict(t *testing.T) {
	reg1 := prometheus.NewRegistry()
	reg2 := prometheus.NewRegistry()
	// Should not panic.
	_ = newSolverMetrics(reg1)
	_ = newSolverMetrics(reg2)
}

