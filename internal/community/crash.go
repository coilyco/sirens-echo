package community

import (
	"context"
	"log/slog"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/getsentry/sentry-go"
)

// Sentry receives crashes and capped failed-turn events. What counts as each,
// and what cannot be caught: docs/sirens-echo-observability.md.

var (
	crashMu           sync.Mutex
	crashActive       bool
	crashWindow       []time.Time
	turnFailureWindow []time.Time
)

// turnFailureTag marks a failed-turn event, which spends its own budget.
const turnFailureTag = "turn.failure_class"

// InitCrashReporting turns crash reporting on when SENTRY_DSN is set. It never
// fails a start: a bad DSN leaves reporting off and says so in the return.
func InitCrashReporting() (bool, error) {
	return initCrashReporting(strings.TrimSpace(os.Getenv("SENTRY_DSN")), nil)
}

func initCrashReporting(dsn string, transport sentry.Transport) (bool, error) {
	crashMu.Lock()
	defer crashMu.Unlock()
	if dsn == "" {
		crashActive = false
		return false, nil
	}
	err := sentry.Init(sentry.ClientOptions{
		Dsn:         dsn,
		Environment: valueOrDefault(os.Getenv("OTEL_DEPLOYMENT_ENVIRONMENT"), "homelab"),
		// The -X stamped revision, so a crash names the image it came from.
		Release:       valueOrDefault(os.Getenv("SENTRY_RELEASE"), buildRevision),
		EnableTracing: false,
		BeforeSend: func(event *sentry.Event, _ *sentry.EventHint) *sentry.Event {
			budget := crashWithinBudget
			if _, isTurn := event.Tags[turnFailureTag]; isTurn {
				budget = turnFailureWithinBudget
			}
			if !budget(time.Now()) {
				return nil
			}
			return event
		},
		Transport: transport,
	})
	crashActive = err == nil
	return crashActive, err
}

// SetCrashService tags crash events with the same service.name OTel uses.
func SetCrashService(name string) {
	sentry.ConfigureScope(func(scope *sentry.Scope) {
		scope.SetTag("service.name", valueOrDefault(name, defaultInstanceName))
	})
}

// ReportCrash sends the error that is about to end the process, and waits for
// it, because os.Exit skips deferred work.
func ReportCrash(err error) {
	if err == nil || !crashReportingActive() {
		return
	}
	sentry.CaptureException(err)
	sentry.Flush(crashFlushTimeout)
}

// ReportTurnFailure sends one event per failed turn for a Sentry alert to count.
// Class and ids only: the error text can carry member or tool output.
func ReportTurnFailure(class, transport, role, requestID, traceID string) {
	if !crashReportingActive() {
		return
	}
	sentry.WithScope(func(scope *sentry.Scope) {
		scope.SetTag(turnFailureTag, class)
		scope.SetTag("sirens_echo.transport", transport)
		scope.SetTag("agent.role", role)
		scope.SetTag("sirens_echo.request_id", requestID)
		if traceID != "" {
			scope.SetTag("trace_id", traceID)
		}
		scope.SetLevel(sentry.LevelError)
		scope.SetFingerprint([]string{"turn-failed", class})
		sentry.CaptureMessage("turn failed: " + class)
	})
}

// RecoverCrash reports a main-goroutine panic and re-panics, so the process
// still dies the way it would have. Use as `defer community.RecoverCrash()`.
func RecoverCrash() {
	recovered := recover()
	if recovered == nil {
		return
	}
	if crashReportingActive() {
		sentry.CurrentHub().Recover(recovered)
		sentry.Flush(crashFlushTimeout)
	}
	panic(recovered)
}

func crashReportingActive() bool {
	crashMu.Lock()
	defer crashMu.Unlock()
	return crashActive
}

// crashWithinBudget caps events per process so a crash loop cannot spend the
// monthly quota.
func crashWithinBudget(now time.Time) bool {
	return eventsWithinBudget(&crashWindow, now, time.Minute, crashEventsPerMinute)
}

// turnFailureWithinBudget is a separate window, so a provider outage cannot
// spend the budget a real crash needs.
func turnFailureWithinBudget(now time.Time) bool {
	return eventsWithinBudget(&turnFailureWindow, now, time.Hour, turnFailureEventsPerHour)
}

func eventsWithinBudget(window *[]time.Time, now time.Time, span time.Duration, limit int) bool {
	crashMu.Lock()
	defer crashMu.Unlock()
	cutoff := now.Add(-span)
	kept := (*window)[:0]
	for _, at := range *window {
		if at.After(cutoff) {
			kept = append(kept, at)
		}
	}
	*window = kept
	if len(*window) >= limit {
		return false
	}
	*window = append(*window, now)
	return true
}

// crashBreadcrumbKeys hold member or model text and never ride on a crash.
var crashBreadcrumbKeys = map[string]bool{
	"content": true, "text": true, "prompt": true, "messages": true,
	"body": true, "reply": true, "arguments": true,
	"authorization": true, "token": true, "cookie": true,
}

// crashBreadcrumbHandler turns log records into breadcrumbs on the next crash,
// never into events of their own. It is one leg of the telemetry multiHandler.
type crashBreadcrumbHandler struct {
	attrs []slog.Attr
}

func (h crashBreadcrumbHandler) Enabled(_ context.Context, level slog.Level) bool {
	return level >= slog.LevelInfo && crashReportingActive()
}

func (h crashBreadcrumbHandler) Handle(_ context.Context, record slog.Record) error {
	data := make(map[string]any, len(h.attrs)+record.NumAttrs())
	put := func(attr slog.Attr) bool {
		if crashBreadcrumbKeys[strings.ToLower(attr.Key)] {
			data[attr.Key] = "[Filtered]"
		} else {
			data[attr.Key] = attr.Value.Resolve().Any()
		}
		return true
	}
	for _, attr := range h.attrs {
		put(attr)
	}
	record.Attrs(put)
	sentry.AddBreadcrumb(&sentry.Breadcrumb{
		Type:      "default",
		Category:  "log",
		Message:   record.Message,
		Level:     crashBreadcrumbLevel(record.Level),
		Data:      data,
		Timestamp: record.Time,
	})
	return nil
}

func (h crashBreadcrumbHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return crashBreadcrumbHandler{attrs: append(append([]slog.Attr(nil), h.attrs...), attrs...)}
}

// WithGroup keeps the handler flat: a breadcrumb's data is one level deep.
func (h crashBreadcrumbHandler) WithGroup(string) slog.Handler { return h }

func crashBreadcrumbLevel(level slog.Level) sentry.Level {
	switch {
	case level >= slog.LevelError:
		return sentry.LevelError
	case level >= slog.LevelWarn:
		return sentry.LevelWarning
	default:
		return sentry.LevelInfo
	}
}
