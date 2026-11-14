package main

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"time"

	prometheusbridge "go.opentelemetry.io/contrib/bridges/prometheus"
	"go.opentelemetry.io/contrib/exporters/autoexport"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	otellog "go.opentelemetry.io/otel/log"
	"go.opentelemetry.io/otel/propagation"
	sdklog "go.opentelemetry.io/otel/sdk/log"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	sdkresource "go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
)

// instrumentationName is the OTel tracer/meter/logger name used for all
// instrumentation emitted by this process.
const instrumentationName = "github.com/henningjanssen/cert-manager-webhook-inwx"

// signalEnabled reports whether OTEL_<SIGNAL>_EXPORTER asks for an exporter.
// Unlike autoexport (which defaults to otlp), an unset variable means off.
func signalEnabled(signal string) bool {
	v := os.Getenv("OTEL_" + signal + "_EXPORTER")
	return v != "" && v != "none"
}

// initTelemetry installs global OpenTelemetry providers for every signal
// (traces, metrics, logs) whose OTEL_<SIGNAL>_EXPORTER is set, e.g. "otlp".
// Signals left unset stay no-op. autoexport reads the standard per-signal
// variables, so each signal has its own endpoint and protocol:
// OTEL_EXPORTER_OTLP_<SIGNAL>_ENDPOINT and OTEL_EXPORTER_OTLP_<SIGNAL>_PROTOCOL
// ("grpc" or "http/protobuf"). OTEL_SERVICE_NAME and OTEL_RESOURCE_ATTRIBUTES
// are honoured.
//
// Metrics export the Prometheus default registry (the same data as /metrics).
// Logs export every line the process writes to stderr and stdout.
//
// A failing signal does not stop the others; its error is joined into the
// result. The returned shutdown function is always non-nil and must be called
// before exit to flush pending data.
func initTelemetry(ctx context.Context) (func(context.Context) error, error) {
	var shutdowns []func(context.Context) error
	shutdown := func(ctx context.Context) error {
		var errs []error
		// Reverse order: the log tee must be drained before its provider stops.
		for i := len(shutdowns) - 1; i >= 0; i-- {
			errs = append(errs, shutdowns[i](ctx))
		}
		return errors.Join(errs...)
	}

	// Detector errors are non-fatal; the resource keeps what was collected.
	res, _ := sdkresource.New(ctx,
		sdkresource.WithFromEnv(),
		sdkresource.WithProcess(),
	)

	var errs []error

	if signalEnabled("TRACES") {
		if exp, err := autoexport.NewSpanExporter(ctx); err != nil {
			errs = append(errs, fmt.Errorf("traces: %w", err))
		} else {
			tp := sdktrace.NewTracerProvider(
				sdktrace.WithBatcher(exp, sdktrace.WithBatchTimeout(5*time.Second)),
				sdktrace.WithResource(res),
			)
			otel.SetTracerProvider(tp)
			otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
				propagation.TraceContext{},
				propagation.Baggage{},
			))
			shutdowns = append(shutdowns, tp.Shutdown)
		}
	}

	if signalEnabled("METRICS") {
		autoexport.WithFallbackMetricProducer(func(context.Context) (sdkmetric.Producer, error) {
			return prometheusbridge.NewMetricProducer(), nil
		})
		if r, err := autoexport.NewMetricReader(ctx); err != nil {
			errs = append(errs, fmt.Errorf("metrics: %w", err))
		} else {
			mp := sdkmetric.NewMeterProvider(sdkmetric.WithReader(r), sdkmetric.WithResource(res))
			otel.SetMeterProvider(mp)
			shutdowns = append(shutdowns, mp.Shutdown)
		}
	}

	if signalEnabled("LOGS") {
		if exp, err := autoexport.NewLogExporter(ctx); err != nil {
			errs = append(errs, fmt.Errorf("logs: %w", err))
		} else {
			lp := sdklog.NewLoggerProvider(
				sdklog.WithProcessor(sdklog.NewBatchProcessor(exp)),
				sdklog.WithResource(res),
			)
			otel.SetLoggerProvider(lp)
			shutdowns = append(shutdowns, lp.Shutdown)
			logger := lp.Logger(instrumentationName)
			for _, f := range []**os.File{&os.Stderr, &os.Stdout} {
				restore, err := teeLines(f, logger)
				if err != nil {
					errs = append(errs, fmt.Errorf("logs: %w", err))
					continue
				}
				shutdowns = append(shutdowns, func(context.Context) error { restore(); return nil })
			}
		}
	}

	return shutdown, errors.Join(errs...)
}

// teeLines replaces *f with a pipe whose content is still written to the
// original file and is also emitted line by line as OTel log records.
// cert-manager configures klog itself, so intercepting the output stream is
// the only hook that catches its logs as well as ours.
// The returned restore function puts the original file back and waits for
// pending lines to be emitted.
func teeLines(f **os.File, logger otellog.Logger) (func(), error) {
	orig := *f
	r, w, err := os.Pipe()
	if err != nil {
		return nil, err
	}
	*f = w
	done := make(chan struct{})
	go func() {
		defer close(done)
		sc := bufio.NewScanner(io.TeeReader(r, orig))
		sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
		for sc.Scan() {
			var rec otellog.Record
			rec.SetTimestamp(time.Now())
			rec.SetSeverity(klogSeverity(sc.Text()))
			rec.SetBody(attribute.StringValue(sc.Text()))
			logger.Emit(context.Background(), rec)
		}
		// On an over-long line keep passing output through so writers never block.
		_, _ = io.Copy(orig, r)
	}()
	return func() {
		*f = orig
		_ = w.Close()
		<-done
	}, nil
}

// klogSeverity maps the klog text header (e.g. "I1110 12:00:00.000000 ...")
// to an OTel severity. Other formats are left unspecified.
func klogSeverity(line string) otellog.Severity {
	if len(line) < 5 || line[1] < '0' || line[1] > '9' {
		return otellog.SeverityUndefined
	}
	switch line[0] {
	case 'I':
		return otellog.SeverityInfo
	case 'W':
		return otellog.SeverityWarn
	case 'E':
		return otellog.SeverityError
	case 'F':
		return otellog.SeverityFatal
	}
	return otellog.SeverityUndefined
}
