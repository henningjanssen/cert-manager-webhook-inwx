package main

import (
	"context"
	"os"
	"sync"
	"testing"

	"go.opentelemetry.io/otel"
	otellog "go.opentelemetry.io/otel/log"
	"go.opentelemetry.io/otel/log/embedded"
)

func TestInitTelemetry_NothingEnabled_StaysNoop(t *testing.T) {
	for _, s := range []string{"TRACES", "METRICS", "LOGS"} {
		t.Setenv("OTEL_"+s+"_EXPORTER", "")
	}
	shutdown, err := initTelemetry(context.Background())
	if err != nil {
		t.Fatalf("initTelemetry with nothing enabled must not fail: %v", err)
	}
	if err := shutdown(context.Background()); err != nil {
		t.Fatalf("shutdown must not fail: %v", err)
	}

	_, span := otel.Tracer(instrumentationName).Start(context.Background(), "test-op")
	defer span.End()
	if span.SpanContext().IsValid() {
		t.Error("expected noop span when traces are not enabled")
	}
}

func TestInitTelemetry_InvalidProtocol_ReturnsError(t *testing.T) {
	t.Setenv("OTEL_TRACES_EXPORTER", "otlp")
	t.Setenv("OTEL_EXPORTER_OTLP_TRACES_PROTOCOL", "carrier-pigeon")
	shutdown, err := initTelemetry(context.Background())
	if err == nil {
		t.Fatal("expected error for unsupported protocol")
	}
	if err := shutdown(context.Background()); err != nil {
		t.Fatalf("shutdown must not fail: %v", err)
	}
}

func TestSignalEnabled(t *testing.T) {
	for v, want := range map[string]bool{"": false, "none": false, "otlp": true} {
		t.Setenv("OTEL_LOGS_EXPORTER", v)
		if got := signalEnabled("LOGS"); got != want {
			t.Errorf("OTEL_LOGS_EXPORTER=%q: got %v, want %v", v, got, want)
		}
	}
}

func TestKlogSeverity(t *testing.T) {
	cases := map[string]otellog.Severity{
		"I1110 12:00:00.000000       1 x.go:1] hi": otellog.SeverityInfo,
		"W1110 12:00:00.000000       1 x.go:1] hi": otellog.SeverityWarn,
		"E1110 12:00:00.000000       1 x.go:1] hi": otellog.SeverityError,
		"F1110 12:00:00.000000       1 x.go:1] hi": otellog.SeverityFatal,
		"Info about something":                     otellog.SeverityUndefined,
		`{"msg":"json"}`:                           otellog.SeverityUndefined,
	}
	for line, want := range cases {
		if got := klogSeverity(line); got != want {
			t.Errorf("%q: got %v, want %v", line, got, want)
		}
	}
}

type recordingLogger struct {
	embedded.Logger
	mu    sync.Mutex
	lines []string
}

func (l *recordingLogger) Emit(_ context.Context, r otellog.Record) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.lines = append(l.lines, r.Body().AsString())
}

func (l *recordingLogger) Enabled(context.Context, otellog.EnabledParameters) bool { return true }

func TestTeeLines_PassesThroughAndEmits(t *testing.T) {
	out, err := os.CreateTemp(t.TempDir(), "out")
	if err != nil {
		t.Fatal(err)
	}
	f := out
	logger := &recordingLogger{}
	restore, err := teeLines(&f, logger)
	if err != nil {
		t.Fatal(err)
	}
	_, _ = f.WriteString("I1110 first\nsecond\n")
	restore()

	if f != out {
		t.Error("restore must put the original file back")
	}
	got, _ := os.ReadFile(out.Name())
	if string(got) != "I1110 first\nsecond\n" {
		t.Errorf("pass-through output = %q", got)
	}
	if len(logger.lines) != 2 || logger.lines[0] != "I1110 first" || logger.lines[1] != "second" {
		t.Errorf("emitted lines = %q", logger.lines)
	}
}
