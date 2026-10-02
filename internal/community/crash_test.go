package community

import (
	"context"
	"errors"
	"log/slog"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/getsentry/sentry-go"
)

type captureTransport struct {
	mu     sync.Mutex
	events []*sentry.Event
}

func (c *captureTransport) Configure(sentry.ClientOptions)        {}
func (c *captureTransport) Flush(time.Duration) bool              { return true }
func (c *captureTransport) FlushWithContext(context.Context) bool { return true }
func (c *captureTransport) Close()                                {}
func (c *captureTransport) SendEvent(event *sentry.Event) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.events = append(c.events, event)
}

func (c *captureTransport) sent() []*sentry.Event {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]*sentry.Event(nil), c.events...)
}

func withCapturedCrashes(t *testing.T) *captureTransport {
	t.Helper()
	transport := &captureTransport{}
	active, err := initCrashReporting("https://public@example.invalid/1", transport)
	if err != nil || !active {
		t.Fatalf("initCrashReporting = %v, %v", active, err)
	}
	t.Cleanup(func() {
		_, _ = initCrashReporting("", nil)
		crashMu.Lock()
		crashWindow = nil
		turnFailureWindow = nil
		crashMu.Unlock()
	})
	return transport
}

func TestNoDSNLeavesCrashReportingOff(t *testing.T) {
	active, err := initCrashReporting("", nil)
	if active || err != nil {
		t.Fatalf("initCrashReporting(\"\") = %v, %v, want off and no error", active, err)
	}
	ReportCrash(errors.New("ignored"))
}

func TestReportCrashSendsTheErrorEndingTheProcess(t *testing.T) {
	transport := withCapturedCrashes(t)
	SetCrashService("sirens-deep")
	ReportCrash(errors.New("run: gateway closed"))
	sent := transport.sent()
	if len(sent) != 1 {
		t.Fatalf("sent %d events, want 1", len(sent))
	}
	if got := sent[0].Exception[len(sent[0].Exception)-1].Value; got != "run: gateway closed" {
		t.Fatalf("exception value = %q", got)
	}
	if got := sent[0].Tags["service.name"]; got != "sirens-deep" {
		t.Fatalf("service.name tag = %q", got)
	}
}

func TestRecoverCrashReportsAndStillPanics(t *testing.T) {
	transport := withCapturedCrashes(t)
	defer func() {
		if recovered := recover(); recovered != "lane wedged" {
			t.Fatalf("re-panic = %v, want the original panic", recovered)
		}
		if len(transport.sent()) != 1 {
			t.Fatalf("sent %d events, want 1", len(transport.sent()))
		}
	}()
	func() {
		defer RecoverCrash()
		panic("lane wedged")
	}()
}

func TestCrashBudgetCapsEventsPerProcessMinute(t *testing.T) {
	withCapturedCrashes(t)
	start := time.Unix(1000, 0)
	allowed := 0
	for range crashEventsPerMinute + 1 {
		if crashWithinBudget(start) {
			allowed++
		}
	}
	if allowed != crashEventsPerMinute {
		t.Fatalf("allowed %d in one minute, want %d", allowed, crashEventsPerMinute)
	}
	if !crashWithinBudget(start.Add(61 * time.Second)) {
		t.Fatalf("budget did not recover after the window")
	}
}

func TestACrashCarriesReleaseAndLogBreadcrumbs(t *testing.T) {
	transport := withCapturedCrashes(t)
	logger := slog.New(crashBreadcrumbHandler{})
	secret := strings.Join([]string{"MEMBER", "TEXT"}, "-")
	logger.Info("turn.admitted", slog.String("stage", "admission"), slog.String("content", secret))
	logger.Debug("too quiet to keep")
	ReportCrash(errors.New("run: gateway closed"))

	sent := transport.sent()
	if len(sent) != 1 {
		t.Fatalf("sent %d events, want 1 (a log line must never raise one)", len(sent))
	}
	crumbs := sent[0].Breadcrumbs
	if len(crumbs) != 1 || crumbs[0].Message != "turn.admitted" {
		t.Fatalf("breadcrumbs = %+v, want the one info record", crumbs)
	}
	if crumbs[0].Data["stage"] != "admission" {
		t.Fatalf("breadcrumb data lost a harmless attr: %v", crumbs[0].Data)
	}
	if got := crumbs[0].Data["content"]; got != "[Filtered]" {
		t.Fatalf("content attr = %v, want it scrubbed", got)
	}
}

func TestCrashReleaseFallsBackToTheStampedRevision(t *testing.T) {
	t.Setenv("SENTRY_RELEASE", "")
	previous := buildRevision
	buildRevision = "abc1234"
	t.Cleanup(func() { buildRevision = previous })
	transport := withCapturedCrashes(t)
	ReportCrash(errors.New("boom"))
	if got := transport.sent()[0].Release; got != "abc1234" {
		t.Fatalf("release = %q, want the stamped revision", got)
	}
}
