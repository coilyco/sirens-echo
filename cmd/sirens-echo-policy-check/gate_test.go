package main_test

import (
	"os"
	"strings"
	"testing"
)

// gateVerbs reads the gate's own verb list rather than the whole script. Every
// verb also appears as its own case label, which any looser match would find.
func gateVerbs(t *testing.T, script string) string {
	t.Helper()
	const marker = "for verb in "
	start := strings.Index(script, marker)
	if start < 0 {
		t.Fatal("the gate has no verb list")
	}
	line := script[start+len(marker):]
	if end := strings.Index(line, ";"); end >= 0 {
		line = line[:end]
	}
	return " " + strings.TrimSpace(line) + " "
}

// The gate exists so one habit replaces six. A check CI runs and the gate omits
// is a check a push can miss while the gate reports ready. See issue 305.
func TestTheGateCoversEveryCheckCIRuns(t *testing.T) {
	t.Parallel()
	gate, err := os.ReadFile("../../scripts/task.sh")
	if err != nil {
		t.Fatalf("read the gate script: %v", err)
	}
	workflow, err := os.ReadFile("../../.forgejo/workflows/ci.yml")
	if err != nil {
		t.Fatalf("read the CI workflow: %v", err)
	}
	verbs := gateVerbs(t, string(gate))
	for _, verb := range []string{"build", "policy-check", "vet", "test", "test-skips"} {
		if !strings.Contains(verbs, " "+verb+" ") {
			t.Errorf("the gate omits %q, which CI runs", verb)
		}
		if !strings.Contains(string(workflow), "just "+verb) {
			t.Errorf("CI no longer runs %q; drop it from the gate or restore it", verb)
		}
	}
	if !strings.Contains(string(gate), "pre-commit run --all-files") {
		t.Error("the gate does not run pre-commit, which is the step that keeps failing")
	}
}

// The repository declares its lane in AGENTS.md frontmatter and nothing read
// it, so an agent could push to main with every check green. See issue 329.
func TestTheGateReadsTheDeclaredWorkflow(t *testing.T) {
	t.Parallel()
	gate, err := os.ReadFile("../../scripts/task.sh")
	if err != nil {
		t.Fatalf("read the gate script: %v", err)
	}
	body := string(gate)
	// This guard went quiet instead of red twice: once when the declaration moved
	// out of ward.yaml, once when the lane was renamed. Neither keys on one name.
	if !strings.Contains(body, "AGENTS.md") {
		t.Error("the gate does not read the file the lane is declared in")
	}
	for _, lane := range []string{"pull-request-and-merge", "remote-branch-only"} {
		if !strings.Contains(body, lane) {
			t.Errorf("the gate does not name the %s lane, so main is pushable from "+
				"it with every check green", lane)
		}
	}
	if !strings.Contains(body, "symbolic-ref") {
		t.Error("the gate does not check which branch it is on")
	}
}
