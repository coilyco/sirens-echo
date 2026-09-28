package community

import (
	"os"
	"strings"
	"sync"
	"time"

	"github.com/getsentry/sentry-go"
)

// Sentry receives crashes only, beside SigNoz. What counts as a crash, and what
// cannot be caught: docs/sirens-echo-observability.md.

var (
	crashMu     sync.Mutex
	crashActive bool
	crashWindow []time.Time
)

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
		Dsn:           dsn,
		Environment:   valueOrDefault(os.Getenv("OTEL_DEPLOYMENT_ENVIRONMENT"), "homelab"),
		EnableTracing: false,
		BeforeSend: func(event *sentry.Event, _ *sentry.EventHint) *sentry.Event {
			if !crashWithinBudget(time.Now()) {
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
	crashMu.Lock()
	defer crashMu.Unlock()
	cutoff := now.Add(-time.Minute)
	kept := crashWindow[:0]
	for _, at := range crashWindow {
		if at.After(cutoff) {
			kept = append(kept, at)
		}
	}
	crashWindow = kept
	if len(crashWindow) >= crashEventsPerMinute {
		return false
	}
	crashWindow = append(crashWindow, now)
	return true
}
