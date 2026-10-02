package community

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

// These share the process-wide Sentry client with crash_test.go, so none of
// them runs in parallel.

func TestAFailedTurnSendsOneEventWithoutItsErrorText(t *testing.T) {
	transport := withCapturedCrashes(t)
	secret := "http://sirens-deep-forgejo-mcp:8080/mcp refused token abc123"
	agent := failingAgent(errors.New(secret))
	turn := &httpTurn{requestID: "req-fail"}

	if err := agent.runTurn(context.Background(), turn, nil); err == nil {
		t.Fatal("runTurn returned no error")
	}

	sent := transport.sent()
	if len(sent) != 1 {
		t.Fatalf("sent %d events, want 1", len(sent))
	}
	event := sent[0]
	if got := event.Tags[turnFailureTag]; got != causeStage {
		t.Errorf("class tag = %q, want %q", got, causeStage)
	}
	if got := event.Tags["sirens_echo.request_id"]; got != "req-fail" {
		t.Errorf("request id tag = %q", got)
	}
	if got := event.Tags["sirens_echo.transport"]; got != turn.Transport() {
		t.Errorf("transport tag = %q, want %q", got, turn.Transport())
	}
	if want := "turn failed: " + causeStage; event.Message != want {
		t.Errorf("message = %q, want %q", event.Message, want)
	}
	if len(event.Fingerprint) != 2 || event.Fingerprint[1] != causeStage {
		t.Errorf("fingerprint = %v, want it grouped by class", event.Fingerprint)
	}
	encoded, err := json.Marshal(event)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	for _, fragment := range []string{"abc123", "8080", "refused", "sirens-deep-forgejo-mcp"} {
		if strings.Contains(string(encoded), fragment) {
			t.Errorf("event leaked %q from the error text", fragment)
		}
	}
}

func TestAClassifiedFailureCarriesItsClass(t *testing.T) {
	transport := withCapturedCrashes(t)
	agent := failingAgent(context.DeadlineExceeded)

	if err := agent.runTurn(context.Background(), &httpTurn{requestID: "slow"}, nil); err == nil {
		t.Fatal("runTurn returned no error")
	}

	sent := transport.sent()
	if len(sent) != 1 || sent[0].Tags[turnFailureTag] != causeTimeout {
		t.Fatalf("events = %+v, want one tagged %q", sent, causeTimeout)
	}
}

func TestAShutdownDrainIsNotAFailedTurn(t *testing.T) {
	transport := withCapturedCrashes(t)
	agent := failingAgent(errShuttingDown)

	if err := agent.runTurn(context.Background(), &httpTurn{requestID: "drain"}, nil); err == nil {
		t.Fatal("runTurn returned no error")
	}

	if sent := transport.sent(); len(sent) != 0 {
		t.Fatalf("sent %d events for a restart, want none", len(sent))
	}
}

func TestFailedTurnEventsAreCappedPerHour(t *testing.T) {
	transport := withCapturedCrashes(t)
	for range turnFailureEventsPerHour + 3 {
		ReportTurnFailure(causeTimeout, "http", "general", "req")
	}
	if got := len(transport.sent()); got != turnFailureEventsPerHour {
		t.Fatalf("sent %d failed-turn events, want the cap %d", got, turnFailureEventsPerHour)
	}
}

func TestASpentTurnBudgetDoesNotMuteACrash(t *testing.T) {
	transport := withCapturedCrashes(t)
	for range turnFailureEventsPerHour + 1 {
		ReportTurnFailure(causeTimeout, "http", "general", "req")
	}
	before := len(transport.sent())
	ReportCrash(errors.New("run: gateway closed"))
	if got := len(transport.sent()); got != before+1 {
		t.Fatalf("a crash was muted by the turn budget: %d events, want %d", got, before+1)
	}
}

func TestASpentCrashBudgetDoesNotMuteAFailedTurn(t *testing.T) {
	transport := withCapturedCrashes(t)
	for range crashEventsPerMinute {
		ReportCrash(errors.New("run: gateway closed"))
	}
	before := len(transport.sent())
	ReportTurnFailure(causeTimeout, "http", "general", "req")
	if got := len(transport.sent()); got != before+1 {
		t.Fatalf("a failed turn was muted by the crash budget: %d events, want %d", got, before+1)
	}
}

func TestNoDSNMeansNoFailedTurnEvent(t *testing.T) {
	if active, err := initCrashReporting("", nil); active || err != nil {
		t.Fatalf("initCrashReporting(\"\") = %v, %v", active, err)
	}
	ReportTurnFailure(causeTimeout, "http", "general", "req")
}
