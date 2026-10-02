package community

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/coilyco/sirens-echo/internal/community/systemone"
)

// TestRouteJevQuestionCountAndLatency is the spec's "measure first" step. The
// live half needs AGENT_PROXY_URL and SIRENS_ECHO_JEV_MODEL; see the allowlist entry.
func TestRouteJevQuestionCountAndLatency(t *testing.T) {
	agent := testJevAgent(t)
	// A representative turn: every family with a trigger present, and a
	// handful of drawer references so the drawer family is not empty either.
	agent.cfg.Definition.LocalSkillRoots = []string{
		"../../.agents/skills/coilyco-org",
		"../../.agents/skills/sirens-echo-knowledge",
		"../../.agents/skills/sirens-game-enshrouded",
	}
	agent.taxonomy = ContentTaxonomy{Classes: realContentClassesForTest(t)}

	disabled := jevDisabledSet(nil)
	questions, _ := agent.buildRouteQuestions(
		TranscriptEntry{Content: "how do I get the fire staff blueprint"},
		disabled,
		true, // hasEarlierInWindow, so coalesce is asked
		true, // threadNoMention, so addressed is asked
	)
	t.Logf("route.jev built %d questions for a representative turn", len(questions))
	if len(questions) == 0 {
		t.Fatal("built zero questions, want every family that can fire to contribute at least one")
	}
	// 13 content + 1 focus + 1 route + 1 depth + 1 shape + 1 coalesce + 1
	// addressed is the floor with zero drawers, roots or servers configured.
	const floor = 19
	if len(questions) < floor {
		t.Errorf("built %d questions, want at least %d for this turn's configured roots", len(questions), floor)
	}

	baseURL := strings.TrimSpace(os.Getenv("AGENT_PROXY_URL"))
	model := strings.TrimSpace(os.Getenv("SIRENS_ECHO_JEV_MODEL"))
	if baseURL == "" || model == "" {
		t.Skip("AGENT_PROXY_URL and SIRENS_ECHO_JEV_MODEL are not both set: skipping the live latency and " +
			"single-request-acceptance measurement. Set both against a reachable Agent Proxy deployment " +
			"to run it for real; see teable:coilyco-gaming/sirens-echo#8050 for the filed follow-up to run " +
			"this once against a live deployment.")
	}

	client := systemone.Client{BaseURL: baseURL, Model: model, Timeout: 15 * time.Second}
	started := time.Now()
	resp, err := client.Ask(context.Background(), systemone.Request{
		State:     jevState(TranscriptEntry{Content: "how do I get the fire staff blueprint"}),
		Questions: questions,
	})
	latency := time.Since(started)
	if err != nil {
		t.Fatalf("live route.jev request with %d questions failed after %s: %v", len(questions), latency, err)
	}
	t.Logf("live route.jev request: %d questions, %d answers, latency %s", len(questions), len(resp.Answers), latency)
	if len(resp.Answers) != len(questions) {
		t.Errorf("got %d answers for %d questions in one request, want the full batch accepted in one call "+
			"(if this fails, route.jev's split-at-%d contingency is what to widen, not this test)",
			len(resp.Answers), len(questions), jevSplitAt)
	}
	if latency > 4*time.Second {
		t.Errorf("latency %s exceeds the spec's proposed p95 (4s) for a single request", latency)
	}
}

// realContentClassesForTest loads the actual tracked taxonomy, so the probe's
// question count reflects the real 13 classes rather than a fixture's two.
func realContentClassesForTest(t *testing.T) []ContentClass {
	t.Helper()
	taxonomy, err := LoadContentTaxonomy("../../agent/content-classes.yaml")
	if err != nil {
		t.Fatalf("load content taxonomy: %v", err)
	}
	return taxonomy.Classes
}
