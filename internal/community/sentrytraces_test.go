package community

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	sdktrace "go.opentelemetry.io/otel/sdk/trace"
)

func TestSentryTracesTargetIsDerivedFromTheDSN(t *testing.T) {
	t.Parallel()
	cases := []struct{ name, dsn, endpoint string }{
		{"saas", "https://abc123@o9.ingest.us.sentry.io/4511", "https://o9.ingest.us.sentry.io/api/4511/integration/otlp/v1/traces"},
		{"path prefix", "https://abc123@sentry.example.net/relay/7", "https://sentry.example.net/relay/api/7/integration/otlp/v1/traces"},
		{"port", "http://abc123@host:9000/3", "http://host:9000/api/3/integration/otlp/v1/traces"},
	}
	for _, tc := range cases {
		endpoint, headers, err := sentryTracesTarget(tc.dsn)
		if err != nil {
			t.Fatalf("%s: %v", tc.name, err)
		}
		if endpoint != tc.endpoint {
			t.Errorf("%s: endpoint = %q, want %q", tc.name, endpoint, tc.endpoint)
		}
		if got := headers["x-sentry-auth"]; got != "sentry sentry_key=abc123" {
			t.Errorf("%s: auth header = %q", tc.name, got)
		}
	}
}

func TestSentryTracesTargetRefusesADSNItCannotRead(t *testing.T) {
	t.Parallel()
	for _, dsn := range []string{"", "not a dsn", "https://o9.ingest.sentry.io/4511", "https://abc@o9.ingest.sentry.io/", "https://abc@o9.ingest.sentry.io"} {
		if endpoint, _, err := sentryTracesTarget(dsn); err == nil {
			t.Errorf("%q gave endpoint %q, want an error", dsn, endpoint)
		}
	}
}

func TestSentryTracesFollowTheDSNUnlessOptedOut(t *testing.T) {
	t.Setenv("SENTRY_DSN", "https://abc123@o9.ingest.us.sentry.io/4511")
	for _, unset := range []string{"", "true", "not a bool"} {
		t.Setenv("SIRENS_ECHO_SENTRY_TRACES", unset)
		if _, on := sentryTracesOption(context.Background()); !on {
			t.Errorf("a DSN with the flag %q left span export off", unset)
		}
	}
	t.Setenv("SIRENS_ECHO_SENTRY_TRACES", "false")
	if _, on := sentryTracesOption(context.Background()); on {
		t.Error("the opt-out left span export on")
	}
	t.Setenv("SIRENS_ECHO_SENTRY_TRACES", "")
	t.Setenv("SENTRY_DSN", "")
	if _, on := sentryTracesOption(context.Background()); on {
		t.Error("no DSN turned span export on")
	}
	t.Setenv("SENTRY_DSN", "not a dsn")
	if _, on := sentryTracesOption(context.Background()); on {
		t.Error("an unreadable DSN turned span export on")
	}
}

type recordingExporter struct{ names []string }

func (r *recordingExporter) ExportSpans(_ context.Context, spans []sdktrace.ReadOnlySpan) error {
	for _, span := range spans {
		r.names = append(r.names, span.Name())
	}
	return nil
}

func (r *recordingExporter) Shutdown(context.Context) error { return nil }

// A sampled trace keeps all its spans, since the keep decision reads only the
// trace id, and the share kept tracks the rate.
func TestTraceRatioExporterKeepsWholeTraces(t *testing.T) {
	t.Parallel()
	recorder := &recordingExporter{}
	exporter := traceRatioExporter{SpanExporter: recorder, rate: 0.25}
	provider := sdktrace.NewTracerProvider(sdktrace.WithSyncer(exporter))
	const traces = 2000
	for range traces {
		ctx, root := provider.Tracer("test").Start(context.Background(), "root")
		_, child := provider.Tracer("test").Start(ctx, "child")
		child.End()
		root.End()
	}
	roots, children := 0, 0
	for _, name := range recorder.names {
		if name == "root" {
			roots++
		} else {
			children++
		}
	}
	if roots != children {
		t.Fatalf("kept %d roots and %d children, want whole traces", roots, children)
	}
	if kept := float64(roots) / traces; kept < 0.2 || kept > 0.3 {
		t.Fatalf("kept %.3f of traces at rate 0.25", kept)
	}
}

func TestSentryTracesSampleRateReadsThePercentKnob(t *testing.T) {
	saved := sentryTracesSamplePercent
	t.Cleanup(func() { sentryTracesSamplePercent = saved })
	for percent, want := range map[int]float64{100: 1, 25: 0.25, 250: 1, 0: 0} {
		sentryTracesSamplePercent = percent
		if got := sentryTracesSampleRate(); got != want {
			t.Errorf("percent %d gave rate %v, want %v", percent, got, want)
		}
	}
}

// The wire check: a span ended while the option is on reaches the Sentry path
// with the DSN's key, which is what the derivation tests only imply.
func TestSentryTracesPostSpansToTheDerivedPathWithTheKey(t *testing.T) {
	type seen struct{ path, auth string }
	got := make(chan seen, 4)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got <- seen{r.URL.Path, r.Header.Get("x-sentry-auth")}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()
	host := strings.TrimPrefix(server.URL, "http://")
	t.Setenv("SENTRY_DSN", "http://abc123@"+host+"/4511")
	t.Setenv("SIRENS_ECHO_SENTRY_TRACES", "true")

	option, on := sentryTracesOption(context.Background())
	if !on {
		t.Fatal("span export to Sentry stayed off")
	}
	provider := sdktrace.NewTracerProvider(option)
	_, span := provider.Tracer("test").Start(context.Background(), "community.turn")
	span.End()
	if err := provider.Shutdown(context.Background()); err != nil {
		t.Fatalf("shutdown: %v", err)
	}
	select {
	case request := <-got:
		if request.path != "/api/4511/integration/otlp/v1/traces" {
			t.Errorf("path = %q", request.path)
		}
		if request.auth != "sentry sentry_key=abc123" {
			t.Errorf("auth = %q", request.auth)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("no span reached the Sentry endpoint")
	}
}
