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
	gate, err := os.ReadFile("../../scripts/ward-command.sh")
	if err != nil {
		t.Fatalf("read the ward command script: %v", err)
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
		if !strings.Contains(string(workflow), "ward exec "+verb) {
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
	gate, err := os.ReadFile("../../scripts/ward-command.sh")
	if err != nil {
		t.Fatalf("read the ward command script: %v", err)
	}
	agents, err := os.ReadFile("../../AGENTS.md")
	if err != nil {
		t.Fatalf("read AGENTS.md: %v", err)
	}
	if !strings.Contains(string(agents), "workflow: pull-request-and-merge") {
		t.Skip("the repository is not on the pull-request lane")
	}
	body := string(gate)
	// The declaration moved out of ward.yaml in 7f37680 and this guard went
	// quiet instead of red, which is the failure the skip review caught.
	if !strings.Contains(body, "AGENTS.md") {
		t.Error("the gate does not read the file the lane is declared in")
	}
	if !strings.Contains(body, "pull-request-and-merge") {
		t.Error("the gate does not read the declared workflow, so main is pushable " +
			"with every check green")
	}
	if !strings.Contains(body, "symbolic-ref") {
		t.Error("the gate does not check which branch it is on")
	}
}
