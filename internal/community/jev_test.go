package community

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/coilyco/sirens-echo/internal/community/systemone"
)

func testJevAgent(t *testing.T) *Agent {
	t.Helper()
	agent := &Agent{
		telemetry: telemetryOrNoop(nil),
		taxonomy: ContentTaxonomy{
			Classes: []ContentClass{
				{ID: "nsfw", Summary: "explicit content", Deny: true, Sensitive: true},
				{ID: "other", Summary: "everything else"},
			},
		},
		phrases: PhraseRegistry{Phrases: []Phrase{
			{Key: "not-permitted", Text: "That's not something I can do."},
		}},
		tools: &MCPProvider{Servers: []MCPServerDefinition{{Name: "eco"}}},
		cfg: Config{
			// No LocalSkillRoots: drawer and root build zero questions here.
			Definition: Definition{},
		},
	}
	return agent
}

func decodeJSON(t *testing.T, r *http.Request, v any) {
	t.Helper()
	if err := json.NewDecoder(r.Body).Decode(v); err != nil {
		t.Fatalf("decode request body: %v", err)
	}
}

func encodeJSON(t *testing.T, w http.ResponseWriter, v any) {
	t.Helper()
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(v); err != nil {
		t.Fatalf("encode response body: %v", err)
	}
}

func TestRouteJevSkipsTheCallWhenNoModelIsConfigured(t *testing.T) {
	agent := testJevAgent(t)
	agent.cfg.JevModel = ""

	decision := agent.routeJev(context.Background(), TranscriptEntry{Content: "hi"}, "req-1", false, false)

	if decision.Ran {
		t.Fatal("Ran = true with no JevModel configured, want the stage skipped entirely")
	}
	for _, family := range routeFamilies {
		if reason := decision.Fallbacks[family]; reason != jevFallbackDisabled {
			t.Errorf("family %s fallback = %q, want %q", family, reason, jevFallbackDisabled)
		}
	}
}

func TestJevDisableRemovesOnlyTheDisabledFamiliesQuestions(t *testing.T) {
	agent := testJevAgent(t)
	disabled := jevDisabledSet([]string{"content", "Server"})

	questions, meta := agent.buildRouteQuestions(TranscriptEntry{Content: "hi"}, disabled, false, false)

	for _, q := range questions {
		if meta[q.Key].family == RouteFamilyContent {
			t.Errorf("content question %s survived kill-switch", q.Key)
		}
		if meta[q.Key].family == RouteFamilyServer {
			t.Errorf("server question %s survived kill-switch", q.Key)
		}
	}
	sawShape := false
	for _, q := range questions {
		if q.Key == "shape" {
			sawShape = true
		}
	}
	if !sawShape {
		t.Error("shape question missing, want it unaffected by disabling content and server")
	}
}

func TestBuildRouteQuestionsAsksCoalesceAndAddressedOnlyOnSignal(t *testing.T) {
	agent := testJevAgent(t)
	disabled := jevDisabledSet(nil)

	questions, _ := agent.buildRouteQuestions(TranscriptEntry{Content: "hi"}, disabled, false, false)
	for _, q := range questions {
		if q.Key == "coalesce" || q.Key == "addressed" {
			t.Errorf("question %s built with no trigger signal", q.Key)
		}
	}

	questions, _ = agent.buildRouteQuestions(TranscriptEntry{Content: "hi"}, disabled, true, true)
	keys := make(map[string]bool)
	for _, q := range questions {
		keys[q.Key] = true
	}
	if !keys["coalesce"] || !keys["addressed"] {
		t.Error("coalesce and addressed should both be asked once their trigger signals are true")
	}
}

func TestShapeOptionsAreReadFromTheLiveRegistriesNotAHardcodedList(t *testing.T) {
	agent := testJevAgent(t)
	options := agent.shapeOptions()

	names := make(map[string]bool)
	for _, option := range options {
		names[option.Name] = true
	}
	if !names["full"] {
		t.Error("shapeOptions missing full")
	}
	for _, key := range reactKeys() {
		// Only a key that needs no lookup may answer before the model runs.
		if names["react:"+key] != snapReactions[key] {
			t.Errorf("shapeOptions offers react:%s = %v, want %v from snapReactions",
				key, names["react:"+key], snapReactions[key])
		}
	}
	if !names["phrase:not-permitted"] {
		t.Error("shapeOptions missing phrase:not-permitted from the configured phrase registry")
	}
}

func TestApplyRouteAnswersContentReusesTheTaxonomyVerdict(t *testing.T) {
	agent := testJevAgent(t)
	disabled := jevDisabledSet(nil)
	meta := map[string]jevQuestionMeta{
		"content:nsfw":  {family: RouteFamilyContent, target: "nsfw"},
		"content:other": {family: RouteFamilyContent, target: "other"},
	}
	answers := map[string]systemone.Answer{
		"content:nsfw":  {Key: "content:nsfw", Probability: 0.91},
		"content:other": {Key: "content:other", Probability: 0.02},
	}

	decision := &RouteDecision{Fallbacks: map[RouteFamily]jevFallbackReason{}}
	agent.applyRouteAnswers(decision, meta, answers, disabled)

	if len(decision.ContentClasses) != 1 || decision.ContentClasses[0] != "nsfw" {
		t.Errorf("ContentClasses = %v, want [nsfw]", decision.ContentClasses)
	}
	if decision.hasFallback(RouteFamilyContent) {
		t.Error("content family should not have fallen back when every question was answered")
	}
}

func TestApplyRouteAnswersDrawerFallsBackWhenNothingPassesThreshold(t *testing.T) {
	agent := testJevAgent(t)
	disabled := jevDisabledSet(nil)
	meta := map[string]jevQuestionMeta{
		"drawer:a.md": {family: RouteFamilyDrawer, target: "a.md"},
	}
	answers := map[string]systemone.Answer{
		"drawer:a.md": {Key: "drawer:a.md", Probability: 0.10},
	}

	decision := &RouteDecision{Fallbacks: map[RouteFamily]jevFallbackReason{}}
	agent.applyRouteAnswers(decision, meta, answers, disabled)

	if len(decision.Drawers) != 0 {
		t.Errorf("Drawers = %v, want none below threshold", decision.Drawers)
	}
	if decision.Fallbacks[RouteFamilyDrawer] != jevFallbackNoDrawerPassed {
		t.Errorf("drawer fallback = %q, want %q", decision.Fallbacks[RouteFamilyDrawer], jevFallbackNoDrawerPassed)
	}
}

// TestRouteJevSplitsAtTheConfiguredThreshold exercises the two-parallel-
// requests contingency, against a local httptest server, not a live proxy.
func TestRouteJevSplitsAtTheConfiguredThreshold(t *testing.T) {
	agent := testJevAgent(t)
	agent.cfg.JevModel = "jev-latest"
	agent.cfg.AgentProxyURL = ""

	// The two halves arrive on separate server goroutines.
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		var req systemone.Request
		decodeJSON(t, r, &req)
		resp := systemone.Response{}
		for _, q := range req.Questions {
			resp.Answers = append(resp.Answers, systemone.Answer{Key: q.Key, Probability: 0.1})
		}
		encodeJSON(t, w, resp)
	}))
	defer server.Close()
	agent.cfg.AgentProxyURL = server.URL

	restore := setJevKnobsForTest(t, 0, 3) // splitAt = 3, well under the ~13 content questions alone
	defer restore()

	decision := agent.routeJev(context.Background(), TranscriptEntry{Content: "hi"}, "req-split", false, false)

	if !decision.Ran {
		t.Fatal("Ran = false, want the stage to have completed")
	}
	if !decision.Split {
		t.Error("Split = false, want true once question count exceeds SIRENS_ECHO_JEV_SPLIT_AT")
	}
	if got := calls.Load(); got != 2 {
		t.Errorf("upstream calls = %d, want 2 for a split request", got)
	}
}

func TestRouteJevFallsBackWhenTheCallFails(t *testing.T) {
	agent := testJevAgent(t)
	agent.cfg.JevModel = "jev-latest"
	agent.cfg.AgentProxyURL = "http://127.0.0.1:0" // nothing listens here

	decision := agent.routeJev(context.Background(), TranscriptEntry{Content: "hi"}, "req-fail", false, false)

	if decision.Ran {
		t.Fatal("Ran = true after a transport failure, want it to have fallen back")
	}
	for _, family := range routeFamilies {
		if reason := decision.Fallbacks[family]; reason != jevFallbackCallFailed {
			t.Errorf("family %s fallback = %q, want %q", family, reason, jevFallbackCallFailed)
		}
	}
}

func TestGatedSkillRootsAndGameFociReadFromLocalSkillRootsNotALiteralList(t *testing.T) {
	roots := []string{
		"path/to/sirens-echo-community",
		"path/to/sirens-game-eco",
		"path/to/sirens-game-enshrouded",
		"path/to/sirens-echo-science",
	}
	gated := gatedSkillRoots(roots)
	want := map[string]bool{"sirens-game-eco": true, "sirens-game-enshrouded": true, "sirens-echo-science": true}
	if len(gated) != len(want) {
		t.Fatalf("gatedSkillRoots = %v, want 3 entries", gated)
	}
	for _, g := range gated {
		if !want[g] {
			t.Errorf("unexpected gated root %s", g)
		}
	}

	foci := gameFociOnDisk(roots)
	if foci["sirens-game-eco"] != "Eco" || foci["sirens-game-enshrouded"] != "Enshrouded" {
		t.Errorf("gameFociOnDisk = %v, want Eco and Enshrouded", foci)
	}
}

// setJevKnobsForTest overrides jevSplitAt for one test, returning a restore func.
func setJevKnobsForTest(t *testing.T, timeoutSeconds, splitAt int) func() {
	t.Helper()
	previousSplit := jevSplitAt
	jevSplitAt = splitAt
	return func() { jevSplitAt = previousSplit }
}
