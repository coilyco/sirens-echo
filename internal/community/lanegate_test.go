package community

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// The lane guard refuses a gate run from main when AGENTS.md frontmatter
// declares pull-request-and-merge. Behaviour, not source text. See issue 329.

// laneFixture is a throwaway repository, because the guard reads a branch name
// and a declared workflow and nothing else about the tree.
func laneFixture(t *testing.T, workflow, branch string) string {
	t.Helper()
	root := t.TempDir()
	script, err := filepath.Abs(filepath.Join("..", "..", "scripts", "ward-command.sh"))
	if err != nil {
		t.Fatalf("resolve the script: %v", err)
	}
	body := "---\nward:\n  workflow: " + workflow + "\n---\n"
	if err := os.WriteFile(filepath.Join(root, "AGENTS.md"), []byte(body), 0o600); err != nil {
		t.Fatalf("write AGENTS.md: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(root, "scripts"), 0o750); err != nil {
		t.Fatalf("mkdir scripts: %v", err)
	}
	source, err := os.ReadFile(script)
	if err != nil {
		t.Fatalf("read the script: %v", err)
	}
	copied := filepath.Join(root, "scripts", "ward-command.sh")
	if err := os.WriteFile(copied, source, 0o700); err != nil {
		t.Fatalf("copy the script: %v", err)
	}
	for _, args := range [][]string{
		{"init", "-q", "-b", branch},
		{"-c", "user.email=qa@example.invalid", "-c", "user.name=qa",
			"commit", "-q", "--allow-empty", "-m", "fixture"},
	} {
		command := exec.Command("git", args...)
		command.Dir = root
		if out, err := command.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	// The wrapper installs the commit hook when none is present, which costs
	// seconds per fixture and is not what this test is about.
	hook := filepath.Join(root, ".git", "hooks", "pre-commit")
	if err := os.WriteFile(hook, []byte("#!/bin/sh\nexit 0\n"), 0o700); err != nil {
		t.Fatalf("write a placeholder hook: %v", err)
	}
	return root
}

// runLaneGate returns the guard's own refusal, or empty when it stayed quiet.
// The gate continues into build after the guard, which a fixture cannot pass.
func runLaneGate(t *testing.T, root string) string {
	t.Helper()
	// Executed directly rather than through a shell, so nothing here is parsed
	// as a shell word. The script carries its own interpreter.
	command := exec.Command(filepath.Join(root, "scripts", "ward-command.sh"), "gate")
	command.Dir = root
	// A pared PATH keeps git and bash and drops the Go toolchain, so a run the
	// guard allows fails at the next step instead of building the fixture.
	command.Env = []string{"PATH=/usr/bin:/bin", "HOME=" + root}
	output, _ := command.CombinedOutput()
	for _, line := range strings.Split(string(output), "\n") {
		if strings.HasPrefix(line, "gate: this repository is on the") {
			return line
		}
	}
	return ""
}

func TestTheLaneGateRefusesAGateRunFromMain(t *testing.T) {
	t.Parallel()
	refusal := runLaneGate(t, laneFixture(t, "pull-request-and-merge", "main"))
	if refusal == "" {
		t.Fatal("the gate ran from main on the pull-request lane, so an agent can " +
			"push straight to main with every check green. See issue 329")
	}
	if !strings.Contains(refusal, "pull-request-and-merge") {
		t.Errorf("the refusal does not name the declared lane: %q", refusal)
	}
}

// The two halves the guard must separate. A branch on the same lane, and main
// on a repository that never declared the lane, both have to run.
func TestTheLaneGateStaysQuietWhenItShould(t *testing.T) {
	t.Parallel()
	for name, fixture := range map[string]string{
		"a branch on the pull-request lane": laneFixture(t, "pull-request-and-merge", "qa/topic"),
		"main where no lane is declared":    laneFixture(t, "push-to-main", "main"),
	} {
		if refusal := runLaneGate(t, fixture); refusal != "" {
			t.Errorf("%s was refused, so the guard blocks work it should allow: %q",
				name, refusal)
		}
	}
}
