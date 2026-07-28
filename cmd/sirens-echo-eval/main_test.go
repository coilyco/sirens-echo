package main

import "testing"

func TestEvaluationOTLPEndpointDefaultsToHostReachableReceiver(t *testing.T) {
	t.Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", "")

	if got := evaluationOTLPEndpoint(); got != defaultEvaluationOTLPEndpoint {
		t.Fatalf(
			"evaluationOTLPEndpoint() = %q, want %q",
			got,
			defaultEvaluationOTLPEndpoint,
		)
	}
}

func TestEvaluationOTLPEndpointPreservesOverride(t *testing.T) {
	const override = "https://collector.example/otlp"
	t.Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", override)

	if got := evaluationOTLPEndpoint(); got != override {
		t.Fatalf("evaluationOTLPEndpoint() = %q, want %q", got, override)
	}
}
