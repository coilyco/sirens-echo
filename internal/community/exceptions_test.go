package community

import (
	"bytes"
	"context"
	tracenoop "go.opentelemetry.io/otel/trace/noop"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
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
	if exceptionCodeCount != 40 {
		t.Fatalf("catalog cardinality = %d, want documented bound 40", exceptionCodeCount)
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
	if len(stages) != 12 {
		t.Fatalf("exception stage cardinality = %d, want documented bound 12", len(stages))
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

// A new exception must not be silently unclassified. The fault is a string
// rather than a bool so a forgotten field is empty, not "service". Issue 159.
func TestEveryExceptionDeclaresAFault(t *testing.T) {
	t.Parallel()
	for code := exceptionCode(0); code < exceptionCodeCount; code++ {
		spec := exceptionFor(code)
		switch spec.fault {
		case faultCaller, faultService:
		default:
			t.Errorf("%s declares fault %q; it must be %q or %q",
				spec.typeName, spec.fault, faultCaller, faultService)
		}
	}
}

// The stage cannot stand in for the fault, which is the reason this field
// exists rather than a query grouping by stage.
func TestTheHTTPStageIsNotTheCallerBucket(t *testing.T) {
	t.Parallel()
	promptFailed := exceptionFor(exceptionHTTPTurnPromptFailed)
	if promptFailed.stage != "http" {
		t.Fatalf("prompt_failed is no longer on the http stage: %q", promptFailed.stage)
	}
	if promptFailed.fault != faultService {
		t.Error("an MCP failure surfaced on the HTTP path is being blamed on the caller")
	}
	invalidJSON := exceptionFor(exceptionHTTPTurnInvalidJSON)
	if invalidJSON.stage != promptFailed.stage {
		t.Fatal("the two codes no longer share a stage, so this test proves nothing")
	}
	if invalidJSON.fault != faultCaller {
		t.Error("a malformed request body is not the caller's fault here")
	}
}

// The refusal reaches logs, not only the span. Vera's severity pipeline made
// log rows alertable, and a span attribute is not on that path. Issue 158.
func TestAnHTTPRefusalLogsItsFault(t *testing.T) {
	t.Parallel()
	var captured bytes.Buffer
	telemetry, err := newTelemetry(
		slog.New(slog.NewJSONHandler(&captured, nil)),
		tracenoop.NewTracerProvider(),
		metricnoop.NewMeterProvider(),
	)
	if err != nil {
		t.Fatalf("newTelemetry: %v", err)
	}
	agent := &Agent{telemetry: telemetry}
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/v1/turn", nil)
	agent.writeHTTPError(
		recorder, request, http.StatusMethodNotAllowed,
		exceptionHTTPTurnMethodNotAllowed, "method not allowed",
	)
	logged := captured.String()
	for _, expected := range []string{`"fault":"caller"`, `"outcome":"method_not_allowed"`} {
		if !strings.Contains(logged, expected) {
			t.Errorf("the refusal log omits %s:\n%s", expected, logged)
		}
	}
	// The response body is service text here, and logging it would be the
	// first step toward logging one that is not.
	if strings.Contains(logged, "method not allowed") {
		t.Errorf("the refusal log carries the response body:\n%s", logged)
	}
}
