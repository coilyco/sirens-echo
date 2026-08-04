package community

import (
	"context"
	"io"
	"log/slog"
	"strings"
	"testing"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	metricnoop "go.opentelemetry.io/otel/metric/noop"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
)

func TestMarkSpanErrorRecordsCatalogedException(t *testing.T) {
	span := recordExceptionForTest(t, exceptionModelTransportFailed)
	spec := exceptionFor(exceptionModelTransportFailed)

	if got := span.Status().Code; got != codes.Error {
		t.Fatalf("status code = %v, want %v", got, codes.Error)
	}
	if got := span.Status().Description; got != spec.message {
		t.Fatalf("status description = %q, want %q", got, spec.message)
	}
	eventAttributes := exceptionEventAttributes(t, span)
	for key, want := range map[string]string{
		"exception.type":    spec.typeName,
		"exception.message": spec.message,
		"error.stage":       spec.stage,
		"error.outcome":     spec.outcome,
	} {
		if got := eventAttributes[key]; got != want {
			t.Fatalf("event attribute %s = %q, want %q", key, got, want)
		}
	}
	if _, ok := eventAttributes["exception.stacktrace"]; ok {
		t.Fatal("exception event contains a stack trace")
	}

	spanAttributes := stringAttributes(span.Attributes())
	for key, want := range map[string]string{
		"error.type":    spec.typeName,
		"error.stage":   spec.stage,
		"error.outcome": spec.outcome,
	} {
		if got := spanAttributes[key]; got != want {
			t.Fatalf("span attribute %s = %q, want %q", key, got, want)
		}
	}
}

func TestExceptionCatalogIsCompleteAndBounded(t *testing.T) {
	if exceptionCodeCount != 23 {
		t.Fatalf("catalog cardinality = %d, want documented bound 23", exceptionCodeCount)
	}
	types := make(map[string]struct{}, exceptionCodeCount)
	stages := make(map[string]struct{})
	for code := exceptionCode(0); code < exceptionCodeCount; code++ {
		spec := exceptionFor(code)
		if !strings.HasPrefix(spec.typeName, "sirens_echo.") {
			t.Fatalf("exception %d type = %q, want sirens_echo namespace", code, spec.typeName)
		}
		if _, exists := types[spec.typeName]; exists {
			t.Fatalf("exception type %q is not unique", spec.typeName)
		}
		types[spec.typeName] = struct{}{}
		stages[spec.stage] = struct{}{}
		if spec.message == "" || spec.outcome == "" {
			t.Fatalf("exception %d has an incomplete catalog entry", code)
		}
		_ = exceptionEventAttributes(t, recordExceptionForTest(t, code))
	}
	if len(types) != int(exceptionCodeCount) {
		t.Fatalf("exception type cardinality = %d, want %d", len(types), exceptionCodeCount)
	}
	if len(stages) != 9 {
		t.Fatalf("exception stage cardinality = %d, want documented bound 9", len(stages))
	}
}

func TestMarkSpanErrorRedactsUnclassifiedRuntimeData(t *testing.T) {
	dynamicValues := []string{
		"dynamic-upstream-61.invalid",
		"dynamic-request-61",
		"dynamic-user-61",
		"/dynamic/path/61",
		"credential-61-secret",
		"payload-61-private",
	}
	untrusted := strings.Join(dynamicValues, "|")
	invalidCode := exceptionCode(int(exceptionCodeCount) + len(untrusted))
	span := recordExceptionForTest(t, invalidCode)
	eventAttributes := exceptionEventAttributes(t, span)
	spanAttributes := stringAttributes(span.Attributes())

	values := []string{span.Status().Description}
	for _, value := range eventAttributes {
		values = append(values, value)
	}
	for _, value := range spanAttributes {
		values = append(values, value)
	}
	serialized := strings.Join(values, "\n")
	for _, dynamic := range dynamicValues {
		if strings.Contains(serialized, dynamic) {
			t.Fatalf("dynamic value %q entered exception fields", dynamic)
		}
	}
	fallback := exceptionFor(exceptionUnclassified)
	if got := eventAttributes["exception.type"]; got != fallback.typeName {
		t.Fatalf("exception.type = %q, want safe fallback %q", got, fallback.typeName)
	}
	if _, ok := eventAttributes["exception.stacktrace"]; ok {
		t.Fatal("unclassified exception contains a stack trace")
	}
}

func recordExceptionForTest(t *testing.T, code exceptionCode) sdktrace.ReadOnlySpan {
	t.Helper()
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
	telemetry.MarkSpanError(span, code)
	span.End()
	ended := recorder.Ended()
	if len(ended) != 1 {
		t.Fatalf("ended spans = %d, want 1", len(ended))
	}
	return ended[0]
}

func exceptionEventAttributes(t *testing.T, span sdktrace.ReadOnlySpan) map[string]string {
	t.Helper()
	var found map[string]string
	for _, event := range span.Events() {
		if event.Name != "exception" {
			continue
		}
		if found != nil {
			t.Fatal("span contains more than one exception event")
		}
		found = stringAttributes(event.Attributes)
	}
	if found == nil {
		t.Fatal("exception event not recorded")
	}
	return found
}

func stringAttributes(attributes []attribute.KeyValue) map[string]string {
	values := make(map[string]string, len(attributes))
	for _, item := range attributes {
		values[string(item.Key)] = item.Value.AsString()
	}
	return values
}
