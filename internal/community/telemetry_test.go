package community

import (
	"context"
	"testing"

	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
)

func TestMetricExporterUsesDeltaTemporality(t *testing.T) {
	t.Parallel()
	exporter, err := newMetricExporter(
		context.Background(),
		"http://collector:4318/v1/metrics",
	)
	if err != nil {
		t.Fatalf("newMetricExporter: %v", err)
	}
	t.Cleanup(func() {
		if err := exporter.Shutdown(context.Background()); err != nil {
			t.Errorf("shutdown metric exporter: %v", err)
		}
	})

	for _, test := range []struct {
		name string
		kind sdkmetric.InstrumentKind
		want metricdata.Temporality
	}{
		{
			name: "counter",
			kind: sdkmetric.InstrumentKindCounter,
			want: metricdata.DeltaTemporality,
		},
		{
			name: "histogram",
			kind: sdkmetric.InstrumentKindHistogram,
			want: metricdata.DeltaTemporality,
		},
		{
			name: "observable counter",
			kind: sdkmetric.InstrumentKindObservableCounter,
			want: metricdata.DeltaTemporality,
		},
		{
			name: "gauge",
			kind: sdkmetric.InstrumentKindGauge,
			want: metricdata.CumulativeTemporality,
		},
		{
			name: "up-down counter",
			kind: sdkmetric.InstrumentKindUpDownCounter,
			want: metricdata.CumulativeTemporality,
		},
	} {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			if got := exporter.Temporality(test.kind); got != test.want {
				t.Fatalf("temporality = %s, want %s", got, test.want)
			}
		})
	}
}

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
