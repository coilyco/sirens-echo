package community

import (
	"context"
	"log/slog"
	"strings"

	"github.com/getsentry/sentry-go"
)

// sentryLogOmitKeys are scrubbed from a Sentry log on top of crashBreadcrumbKeys.
// The failed-turn event leaves error text out, so a log does too.
var sentryLogOmitKeys = map[string]bool{"error": true, "refused": true, "ceiling": true}

// sentryLogHandler sends error-level records to Sentry Logs, trace-linked by the
// resolver crash.go installs. One leg of the telemetry multiHandler.
type sentryLogHandler struct {
	attrs []slog.Attr
}

func (h sentryLogHandler) Enabled(_ context.Context, level slog.Level) bool {
	return level >= slog.LevelError && crashReportingActive()
}

func (h sentryLogHandler) Handle(ctx context.Context, record slog.Record) error {
	entry := sentry.NewLogger(ctx).Error()
	put := func(attr slog.Attr) bool {
		key := attr.Key
		value := attr.Value.Resolve()
		lower := strings.ToLower(key)
		_, isError := value.Any().(error)
		switch {
		case crashBreadcrumbKeys[lower] || sentryLogOmitKeys[lower] || isError:
			entry = entry.String(key, "[Filtered]")
		case value.Kind() == slog.KindInt64:
			entry = entry.Int64(key, value.Int64())
		case value.Kind() == slog.KindBool:
			entry = entry.Bool(key, value.Bool())
		case value.Kind() == slog.KindFloat64:
			entry = entry.Float64(key, value.Float64())
		default:
			entry = entry.String(key, value.String())
		}
		return true
	}
	for _, attr := range h.attrs {
		put(attr)
	}
	record.Attrs(put)
	entry.Emit(record.Message)
	return nil
}

func (h sentryLogHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return sentryLogHandler{attrs: append(append([]slog.Attr(nil), h.attrs...), attrs...)}
}

// WithGroup keeps the handler flat: a Sentry log attribute is one level deep.
func (h sentryLogHandler) WithGroup(string) slog.Handler { return h }
