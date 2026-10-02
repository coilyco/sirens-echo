package community

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// Three explanations produced identical telemetry: a quiet guild, stopped
// ingress, and a dead process. See issue 190.

// Counts drain on each beat, so a reader sees the interval rather than a total
// that needs a baseline to interpret.
func TestABeatDrainsItsCounters(t *testing.T) {
	t.Parallel()
	beats := &heartbeat{}
	beats.observe()
	beats.observe()
	beats.admit()
	beats.reply()
	beats.beat(context.Background(), telemetryOrNoop(nil))
	if got := beats.observed.Load(); got != 0 {
		t.Errorf("observed did not drain: %d", got)
	}
	if got := beats.admitted.Load(); got != 0 {
		t.Errorf("admitted did not drain: %d", got)
	}
	if got := beats.replied.Load(); got != 0 {
		t.Errorf("replied did not drain: %d", got)
	}
}

// Messages arriving and none producing a turn is the shape nothing detects
// today, and it is the one this separates from a quiet guild.
func TestObservedCountsBeforeEligibility(t *testing.T) {
	t.Parallel()
	beats := &heartbeat{}
	beats.observe()
	if got := beats.observed.Load(); got != 1 {
		t.Fatalf("observed = %d, want 1", got)
	}
	if got := beats.admitted.Load(); got != 0 {
		t.Errorf("an ineligible message was counted as admitted: %d", got)
	}
}

// The HTTP-only deployment opens no gateway and has no heartbeat, so every
// counter has to be safe on a nil receiver rather than guarded at each call.
func TestANilHeartbeatIsInert(t *testing.T) {
	t.Parallel()
	var beats *heartbeat
	beats.observe()
	beats.admit()
	beats.reply()
}

func TestNoGatewayHasNoBeatAge(t *testing.T) {
	t.Parallel()
	var none *heartbeat
	if _, ok := none.age(time.Now()); ok {
		t.Fatal("a nil heartbeat reported an age")
	}
	if _, ok := (&heartbeat{}).age(time.Now()); ok {
		t.Fatal("an unseeded heartbeat reported an age")
	}
}

// A gateway that never beats must still age, or a Gatus condition on the age
// could never fail for the process that needs it to.
func TestABeatAgeStartsAtStartAndResetsOnABeat(t *testing.T) {
	t.Parallel()
	started := time.Now().Add(-15 * time.Minute)
	beats := newHeartbeat(started)
	age, ok := beats.age(time.Now())
	if !ok || age < 15*time.Minute {
		t.Fatalf("age before any beat = %v, %v, want at least 15m", age, ok)
	}
	beats.beat(context.Background(), telemetryOrNoop(nil))
	age, ok = beats.age(time.Now())
	if !ok || age > time.Minute {
		t.Fatalf("age after a beat = %v, %v, want under a minute", age, ok)
	}
}

func healthzBody(t *testing.T, agent *Agent) (int, map[string]any) {
	t.Helper()
	recorder := httptest.NewRecorder()
	agent.HTTPHandler().ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/healthz", nil))
	var body map[string]any
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatalf("healthz body %q: %v", recorder.Body.String(), err)
	}
	return recorder.Code, body
}

func TestHealthzReportsTheBeatAgeAndStaysOK(t *testing.T) {
	t.Parallel()
	agent := &Agent{beats: newHeartbeat(time.Now().Add(-900 * time.Second))}
	code, body := healthzBody(t, agent)
	if code != http.StatusOK {
		t.Fatalf("status = %d, want 200 even when the beat is stale", code)
	}
	age, ok := body["gateway_beat_age_seconds"].(float64)
	if !ok || age < 900 || age > 960 {
		t.Fatalf("gateway_beat_age_seconds = %v, want about 900", body["gateway_beat_age_seconds"])
	}
	if body["ok"] != true {
		t.Fatalf("ok = %v", body["ok"])
	}
}

func TestHealthzOmitsTheAgeWithoutAGateway(t *testing.T) {
	t.Parallel()
	_, body := healthzBody(t, &Agent{})
	if _, present := body["gateway_beat_age_seconds"]; present {
		t.Fatalf("an HTTP-only agent reported a beat age: %v", body)
	}
}
