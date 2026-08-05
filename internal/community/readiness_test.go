package community

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"go.opentelemetry.io/otel/attribute"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
)

const testReadinessRoute = "sirens-echo/default"

func TestAgentProxyReadinessEndpoint(t *testing.T) {
	t.Parallel()
	endpoint, err := agentProxyReadinessEndpoint(
		"https://proxy.example/internal/?discard=true#fragment",
		testReadinessRoute,
	)
	if err != nil {
		t.Fatalf("agentProxyReadinessEndpoint: %v", err)
	}
	if endpoint != "https://proxy.example/internal/readyz/sirens-echo/default" {
		t.Fatalf("endpoint = %q", endpoint)
	}
	for _, test := range []struct {
		name  string
		base  string
		route string
	}{
		{name: "not logical", base: "https://proxy.example", route: "model"},
		{name: "extra segment", base: "https://proxy.example", route: "sirens-echo/a/b"},
		{name: "invalid base", base: "proxy.example", route: testReadinessRoute},
	} {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			if _, err := agentProxyReadinessEndpoint(test.base, test.route); err == nil {
				t.Fatal("expected endpoint validation error")
			}
		})
	}
}

func TestHealthzIsMetricsOnly(t *testing.T) {
	t.Parallel()
	agent, logs, spans, reader := newHealthTestAgent(t, nil, time.Second)
	request := httptest.NewRequest(http.MethodGet, healthzPath, nil)
	request.Header.Set("traceparent", "00-1234567890abcdef1234567890abcdef-1234567890abcdef-01")
	recorder := httptest.NewRecorder()

	agent.HTTPHandler().ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK || strings.TrimSpace(recorder.Body.String()) != `{"ok":true}` {
		t.Fatalf("health response = %d %q", recorder.Code, recorder.Body.String())
	}
	if logs.Len() != 0 {
		t.Fatalf("health log = %q", logs.String())
	}
	if ended := spans.Ended(); len(ended) != 0 {
		t.Fatalf("health spans = %#v", ended)
	}
	metrics := collectMetrics(t, reader)
	requests := metricByName(t, metrics, "sirens_echo.health.requests")
	sum, ok := requests.Data.(metricdata.Sum[int64])
	if !ok || len(sum.DataPoints) != 1 || sum.DataPoints[0].Value != 1 {
		t.Fatalf("health request metric = %#v", requests.Data)
	}
	assertAttributes(t, sum.DataPoints[0].Attributes, map[string]string{
		"endpoint": "healthz",
		"outcome":  "ready",
	})
	assertMetricAbsent(t, metrics, "sirens_echo.model.calls")
	assertMetricAbsent(t, metrics, "sirens_echo.turns")
}

func TestReadyzCallsOnlyAgentProxyReadiness(t *testing.T) {
	t.Parallel()
	var calls atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		calls.Add(1)
		if request.Method != http.MethodGet {
			t.Errorf("method = %s", request.Method)
		}
		if request.URL.Path != "/readyz/sirens-echo/default" {
			t.Errorf("path = %q", request.URL.Path)
		}
		if request.Body != nil && request.ContentLength > 0 {
			t.Errorf("readiness request carried a body")
		}
		if request.Header.Get("traceparent") != "" {
			t.Errorf("readiness request propagated trace context")
		}
		writer.Header().Set("Content-Type", "application/json")
		writer.WriteHeader(http.StatusOK)
		_, _ = writer.Write([]byte(`{"status":"ready","route":"sirens-echo/default"}`))
	}))
	t.Cleanup(server.Close)
	agent, logs, spans, reader := newHealthTestAgent(t, server.Client(), time.Second)
	agent.readinessEndpoint = server.URL + "/readyz/sirens-echo/default"
	request := httptest.NewRequest(http.MethodGet, readyzPath, nil)
	request.Header.Set("traceparent", "00-1234567890abcdef1234567890abcdef-1234567890abcdef-01")
	recorder := httptest.NewRecorder()

	agent.HTTPHandler().ServeHTTP(recorder, request)

	if calls.Load() != 1 {
		t.Fatalf("Agent Proxy calls = %d", calls.Load())
	}
	if recorder.Code != http.StatusOK || strings.TrimSpace(recorder.Body.String()) != `{"status":"ready"}` {
		t.Fatalf("readiness response = %d %q", recorder.Code, recorder.Body.String())
	}
	assertHealthSilence(t, logs, spans)
	metrics := collectMetrics(t, reader)
	assertMetricAbsent(t, metrics, "sirens_echo.model.calls")
	assertMetricAbsent(t, metrics, "sirens_echo.turns")
	state := metricByName(t, metrics, "sirens_echo.readiness.state")
	gauge, ok := state.Data.(metricdata.Gauge[int64])
	if !ok || len(gauge.DataPoints) != 1 || gauge.DataPoints[0].Value != 1 {
		t.Fatalf("readiness state metric = %#v", state.Data)
	}
	lastSuccess := metricByName(t, metrics, "sirens_echo.readiness.last_success")
	lastGauge, ok := lastSuccess.Data.(metricdata.Gauge[int64])
	if !ok || len(lastGauge.DataPoints) != 1 || lastGauge.DataPoints[0].Value <= 0 {
		t.Fatalf("readiness last success metric = %#v", lastSuccess.Data)
	}
	duration := metricByName(t, metrics, "sirens_echo.readiness.duration")
	histogram, ok := duration.Data.(metricdata.Histogram[float64])
	if !ok || len(histogram.DataPoints) != 1 || histogram.DataPoints[0].Count != 1 {
		t.Fatalf("readiness duration metric = %#v", duration.Data)
	}
}

func TestReadyzFailsClosedWithoutLeakingDependencyDetails(t *testing.T) {
	t.Parallel()
	const privateDetail = "private backend body and physical model"
	for _, test := range []struct {
		name        string
		status      int
		body        string
		wantOutcome readinessOutcome
	}{
		{
			name:        "not ready",
			status:      http.StatusServiceUnavailable,
			body:        `{"status":"not_ready","route":"sirens-echo/default","failed_checks":["ollama_catalog"]}`,
			wantOutcome: readinessNotReady,
		},
		{
			name:        "unknown route",
			status:      http.StatusNotFound,
			body:        `{"status":"unknown_route"}`,
			wantOutcome: readinessUnknownRoute,
		},
		{
			name:        "malformed response",
			status:      http.StatusOK,
			body:        `{"status":"ready","detail":"` + privateDetail,
			wantOutcome: readinessInvalidResponse,
		},
	} {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
				writer.WriteHeader(test.status)
				_, _ = writer.Write([]byte(test.body))
			}))
			t.Cleanup(server.Close)
			agent, logs, spans, reader := newHealthTestAgent(t, server.Client(), time.Second)
			agent.readinessEndpoint = server.URL + "/readyz/sirens-echo/default"
			recorder := httptest.NewRecorder()

			agent.HTTPHandler().ServeHTTP(
				recorder,
				httptest.NewRequest(http.MethodGet, readyzPath, nil),
			)

			if recorder.Code != http.StatusServiceUnavailable ||
				strings.TrimSpace(recorder.Body.String()) != `{"status":"not_ready"}` {
				t.Fatalf("readiness response = %d %q", recorder.Code, recorder.Body.String())
			}
			if strings.Contains(recorder.Body.String(), privateDetail) || strings.Contains(logs.String(), privateDetail) {
				t.Fatal("dependency detail escaped the readiness boundary")
			}
			assertHealthSilence(t, logs, spans)
			metrics := collectMetrics(t, reader)
			requests := metricByName(t, metrics, "sirens_echo.health.requests")
			sum, ok := requests.Data.(metricdata.Sum[int64])
			if !ok || len(sum.DataPoints) != 1 {
				t.Fatalf("health request metric = %#v", requests.Data)
			}
			assertAttributes(t, sum.DataPoints[0].Attributes, map[string]string{
				"endpoint": "readyz",
				"outcome":  string(test.wantOutcome),
			})
		})
	}
}

func TestReadyzTimeoutIsMetricsOnly(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, request *http.Request) {
		<-request.Context().Done()
	}))
	t.Cleanup(server.Close)
	agent, logs, spans, reader := newHealthTestAgent(t, server.Client(), 20*time.Millisecond)
	agent.readinessEndpoint = server.URL + "/readyz/sirens-echo/default"
	recorder := httptest.NewRecorder()

	agent.HTTPHandler().ServeHTTP(
		recorder,
		httptest.NewRequest(http.MethodGet, readyzPath, nil),
	)

	if recorder.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d", recorder.Code)
	}
	assertHealthSilence(t, logs, spans)
	metrics := collectMetrics(t, reader)
	requests := metricByName(t, metrics, "sirens_echo.health.requests")
	sum, ok := requests.Data.(metricdata.Sum[int64])
	if !ok || len(sum.DataPoints) != 1 {
		t.Fatalf("health request metric = %#v", requests.Data)
	}
	assertAttributes(t, sum.DataPoints[0].Attributes, map[string]string{
		"endpoint": "readyz",
		"outcome":  "timeout",
	})
	assertMetricAbsent(t, metrics, "sirens_echo.readiness.last_success")
}

func TestReadyzDependencyErrorDoesNotLeak(t *testing.T) {
	t.Parallel()
	const privateDetail = "private dependency address and credential"
	client := &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		return nil, errors.New(privateDetail)
	})}
	agent, logs, spans, reader := newHealthTestAgent(t, client, time.Second)
	recorder := httptest.NewRecorder()

	agent.HTTPHandler().ServeHTTP(
		recorder,
		httptest.NewRequest(http.MethodGet, readyzPath, nil),
	)

	if recorder.Code != http.StatusServiceUnavailable ||
		strings.TrimSpace(recorder.Body.String()) != `{"status":"not_ready"}` {
		t.Fatalf("readiness response = %d %q", recorder.Code, recorder.Body.String())
	}
	if strings.Contains(recorder.Body.String(), privateDetail) || strings.Contains(logs.String(), privateDetail) {
		t.Fatal("dependency error escaped the readiness boundary")
	}
	assertHealthSilence(t, logs, spans)
	metrics := collectMetrics(t, reader)
	requests := metricByName(t, metrics, "sirens_echo.health.requests")
	sum, ok := requests.Data.(metricdata.Sum[int64])
	if !ok || len(sum.DataPoints) != 1 {
		t.Fatalf("health request metric = %#v", requests.Data)
	}
	assertAttributes(t, sum.DataPoints[0].Attributes, map[string]string{
		"endpoint": "readyz",
		"outcome":  "dependency_error",
	})
}

func newHealthTestAgent(
	t *testing.T,
	client *http.Client,
	timeout time.Duration,
) (*Agent, *bytes.Buffer, *tracetest.SpanRecorder, *sdkmetric.ManualReader) {
	t.Helper()
	logs := &bytes.Buffer{}
	spanRecorder := tracetest.NewSpanRecorder()
	traceProvider := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(spanRecorder))
	metricReader := sdkmetric.NewManualReader()
	meterProvider := sdkmetric.NewMeterProvider(sdkmetric.WithReader(metricReader))
	telemetry, err := newTelemetry(
		slog.New(slog.NewJSONHandler(logs, nil)),
		traceProvider,
		meterProvider,
	)
	if err != nil {
		t.Fatalf("newTelemetry: %v", err)
	}
	t.Cleanup(func() {
		_ = meterProvider.Shutdown(context.Background())
		_ = traceProvider.Shutdown(context.Background())
	})
	if client == nil {
		client = newReadinessHTTPClient(timeout)
	}
	client.Timeout = timeout
	return &Agent{
		telemetry:         telemetry,
		readinessClient:   client,
		readinessEndpoint: "http://127.0.0.1:1/readyz/sirens-echo/default",
		readinessRoute:    testReadinessRoute,
		readinessTimeout:  timeout,
	}, logs, spanRecorder, metricReader
}

func assertHealthSilence(t *testing.T, logs *bytes.Buffer, spans *tracetest.SpanRecorder) {
	t.Helper()
	if logs.Len() != 0 {
		t.Fatalf("health log = %q", logs.String())
	}
	if ended := spans.Ended(); len(ended) != 0 {
		t.Fatalf("health spans = %#v", ended)
	}
}

func collectMetrics(t *testing.T, reader *sdkmetric.ManualReader) metricdata.ResourceMetrics {
	t.Helper()
	var metrics metricdata.ResourceMetrics
	if err := reader.Collect(context.Background(), &metrics); err != nil {
		t.Fatalf("collect metrics: %v", err)
	}
	return metrics
}

func metricByName(
	t *testing.T,
	metrics metricdata.ResourceMetrics,
	name string,
) metricdata.Metrics {
	t.Helper()
	for _, scope := range metrics.ScopeMetrics {
		for _, current := range scope.Metrics {
			if current.Name == name {
				return current
			}
		}
	}
	t.Fatalf("metric %q was not collected", name)
	return metricdata.Metrics{}
}

func assertMetricAbsent(t *testing.T, metrics metricdata.ResourceMetrics, name string) {
	t.Helper()
	for _, scope := range metrics.ScopeMetrics {
		for _, current := range scope.Metrics {
			if current.Name == name {
				t.Fatalf("metric %q was unexpectedly collected", name)
			}
		}
	}
}

func assertAttributes(t *testing.T, attributes attribute.Set, want map[string]string) {
	t.Helper()
	got := make(map[string]string)
	for _, keyValue := range attributes.ToSlice() {
		got[string(keyValue.Key)] = keyValue.Value.AsString()
	}
	if len(got) != len(want) {
		t.Fatalf("attributes = %#v, want %#v", got, want)
	}
	for key, value := range want {
		if got[key] != value {
			t.Fatalf("attribute %s = %q, want %q", key, got[key], value)
		}
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (function roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return function(request)
}
