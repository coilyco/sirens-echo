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

func TestSentryTracesStayOffUnlessAskedFor(t *testing.T) {
	t.Setenv("SENTRY_DSN", "https://abc123@o9.ingest.us.sentry.io/4511")
	t.Setenv("SIRENS_ECHO_SENTRY_TRACES", "")
	if _, on := sentryTracesOption(context.Background()); on {
		t.Error("a DSN alone turned span export to Sentry on")
	}
	t.Setenv("SIRENS_ECHO_SENTRY_TRACES", "true")
	t.Setenv("SENTRY_DSN", "")
	if _, on := sentryTracesOption(context.Background()); on {
		t.Error("the flag without a DSN turned span export on")
	}
	t.Setenv("SENTRY_DSN", "not a dsn")
	if _, on := sentryTracesOption(context.Background()); on {
		t.Error("an unreadable DSN turned span export on")
	}
	t.Setenv("SENTRY_DSN", "https://abc123@o9.ingest.us.sentry.io/4511")
	if _, on := sentryTracesOption(context.Background()); !on {
		t.Error("the flag with a DSN left span export off")
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
