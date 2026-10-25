package community

import (
	"context"
	"encoding/binary"
	"fmt"
	"log/slog"
	"net/url"
	"os"
	"strconv"
	"strings"

	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/trace"
)

// sentryTracesOption adds Sentry as a second span destination when SENTRY_DSN is
// set, unless SIRENS_ECHO_SENTRY_TRACES=false. A bad DSN never fails a start.
func sentryTracesOption(ctx context.Context) (sdktrace.TracerProviderOption, bool) {
	optedOut := false
	if raw := strings.TrimSpace(os.Getenv("SIRENS_ECHO_SENTRY_TRACES")); raw != "" {
		enabled, err := strconv.ParseBool(raw)
		optedOut = err == nil && !enabled
	}
	dsn := strings.TrimSpace(os.Getenv("SENTRY_DSN"))
	if optedOut || dsn == "" {
		return nil, false
	}
	endpoint, headers, err := sentryTracesTarget(dsn)
	if err == nil {
		exporter, newErr := otlptracehttp.New(ctx,
			otlptracehttp.WithEndpointURL(endpoint), otlptracehttp.WithHeaders(headers))
		if newErr == nil {
			return sdktrace.WithBatcher(traceRatioExporter{
				SpanExporter: exporter, rate: sentryTracesSampleRate(),
			}), true
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

// sentryTracesSampleRate is the knob as the fraction Sentry's options take.
func sentryTracesSampleRate() float64 {
	return float64(min(max(sentryTracesSamplePercent, 0), 100)) / 100
}

// traceRatioExporter samples by trace id for Sentry alone, so a kept trace keeps
// all its spans. Sampling at the tracer would thin SigNoz too.
type traceRatioExporter struct {
	sdktrace.SpanExporter
	rate float64
}

func (e traceRatioExporter) ExportSpans(ctx context.Context, spans []sdktrace.ReadOnlySpan) error {
	if e.rate >= 1 {
		return e.SpanExporter.ExportSpans(ctx, spans)
	}
	kept := make([]sdktrace.ReadOnlySpan, 0, len(spans))
	for _, span := range spans {
		if traceKept(span.SpanContext().TraceID(), e.rate) {
			kept = append(kept, span)
		}
	}
	if len(kept) == 0 {
		return nil
	}
	return e.SpanExporter.ExportSpans(ctx, kept)
}

// traceKept reads the low 8 bytes of the id as a fraction of the u64 range, the
// way the OTel TraceIDRatioBased sampler does.
func traceKept(id trace.TraceID, rate float64) bool {
	return float64(binary.BigEndian.Uint64(id[8:])>>1) < rate*float64(1<<63)
}
