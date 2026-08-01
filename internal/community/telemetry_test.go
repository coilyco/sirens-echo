package community

import (
	"context"
	"io"
	"log/slog"
	"testing"

	"go.opentelemetry.io/otel/codes"
	metricnoop "go.opentelemetry.io/otel/metric/noop"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
)

func TestOTLPSignalURLAppendsProtocolPath(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		name   string
		base   string
		signal string
		want   string
	}{
		{
			name:   "collector root",
			base:   "http://collector:4318",
			signal: "traces",
			want:   "http://collector:4318/v1/traces",
		},
		{
			name:   "collector base path",
			base:   "https://collector.example/otlp/",
			signal: "metrics",
			want:   "https://collector.example/otlp/v1/metrics",
		},
	} {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			got, err := otlpSignalURL(test.base, test.signal)
			if err != nil {
				t.Fatalf("otlpSignalURL: %v", err)
			}
			if got != test.want {
				t.Fatalf("URL = %q, want %q", got, test.want)
			}
		})
	}
}

func TestOTLPSignalURLRejectsInvalidBase(t *testing.T) {
	t.Parallel()
	if _, err := otlpSignalURL("collector:4318", "traces"); err == nil {
		t.Fatal("expected invalid endpoint error")
	}
}

func TestMarkSpanErrorRecordsSafeExceptionEvent(t *testing.T) {
	recorder := tracetest.NewSpanRecorder()
	provider := sdktrace.NewTracerProvider()
	provider.RegisterSpanProcessor(recorder)
	telemetry, err := newTelemetry(
		slog.New(slog.NewJSONHandler(io.Discard, nil)),
		provider,
		metricnoop.NewMeterProvider(),
	)
	if err != nil {
		t.Fatalf("newTelemetry: %v", err)
	}

	_, span := telemetry.StartSpan(context.Background(), "failed.operation")
	telemetry.MarkSpanError(span, "model_transport_failed")
	span.End()

	ended := recorder.Ended()
	if len(ended) != 1 {
		t.Fatalf("ended spans = %d, want 1", len(ended))
	}
	if got := ended[0].Status().Code; got != codes.Error {
		t.Fatalf("status = %v, want %v", got, codes.Error)
	}
	for _, event := range ended[0].Events() {
		if event.Name != "exception" {
			continue
		}
		attributes := make(map[string]string, len(event.Attributes))
		for _, item := range event.Attributes {
			attributes[string(item.Key)] = item.Value.AsString()
		}
		if got := attributes["exception.message"]; got != "model_transport_failed" {
			t.Fatalf("exception.message = %q, want closed-set error code", got)
		}
		if got := attributes["exception.type"]; got == "" {
			t.Fatal("exception.type is empty")
		}
		return
	}
	t.Fatal("exception event not recorded")
}
