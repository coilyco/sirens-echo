package community

import (
	"context"
	"fmt"
	"log/slog"
	"net/url"
	"os"
	"strconv"
	"strings"

	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
)

// sentryTracesOption adds Sentry as a second span destination when
// SIRENS_ECHO_SENTRY_TRACES is true. A bad DSN never fails a start.
func sentryTracesOption(ctx context.Context) (sdktrace.TracerProviderOption, bool) {
	enabled, _ := strconv.ParseBool(strings.TrimSpace(os.Getenv("SIRENS_ECHO_SENTRY_TRACES")))
	dsn := strings.TrimSpace(os.Getenv("SENTRY_DSN"))
	if !enabled || dsn == "" {
		return nil, false
	}
	endpoint, headers, err := sentryTracesTarget(dsn)
	if err == nil {
		exporter, newErr := otlptracehttp.New(ctx,
			otlptracehttp.WithEndpointURL(endpoint), otlptracehttp.WithHeaders(headers))
		if newErr == nil {
			return sdktrace.WithBatcher(exporter), true
		}
		err = newErr
	}
	slog.Warn("sentry.traces.disabled", slog.String("error", err.Error()))
	return nil, false
}

// sentryTracesTarget derives Sentry's OTLP traces endpoint and auth header from
// a DSN, so one secret configures both Sentry paths.
func sentryTracesTarget(dsn string) (endpoint string, headers map[string]string, err error) {
	parsed, err := url.Parse(strings.TrimSpace(dsn))
	if err != nil || parsed.Scheme == "" || parsed.Host == "" || parsed.User == nil {
		return "", nil, fmt.Errorf("the Sentry DSN is not <scheme>://<key>@<host>/<project>")
	}
	key := parsed.User.Username()
	trimmed := strings.Trim(parsed.Path, "/")
	prefix, project := "", trimmed
	if i := strings.LastIndex(trimmed, "/"); i >= 0 {
		prefix, project = trimmed[:i], trimmed[i+1:]
	}
	if key == "" || project == "" {
		return "", nil, fmt.Errorf("the Sentry DSN names no public key or project")
	}
	base := parsed.Scheme + "://" + parsed.Host
	if prefix != "" {
		base += "/" + prefix
	}
	return base + "/api/" + project + "/integration/otlp/v1/traces",
		map[string]string{"x-sentry-auth": "sentry sentry_key=" + key}, nil
}
