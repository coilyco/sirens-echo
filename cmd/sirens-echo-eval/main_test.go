package main

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/coilyco/sirens-echo/internal/community"
)

func TestEvaluationOTLPEndpointDefaultsToHostReachableReceiver(t *testing.T) {
	t.Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", "")

	if got := evaluationOTLPEndpoint(); got != defaultEvaluationOTLPEndpoint {
		t.Fatalf(
			"evaluationOTLPEndpoint() = %q, want %q",
			got,
			defaultEvaluationOTLPEndpoint,
		)
	}
}

func TestEvaluationOTLPEndpointPreservesOverride(t *testing.T) {
	const override = "https://collector.example/otlp"
	t.Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", override)

	if got := evaluationOTLPEndpoint(); got != override {
		t.Fatalf("evaluationOTLPEndpoint() = %q, want %q", got, override)
	}
}

func TestEvaluationTargetsEchoByDefault(t *testing.T) {
	t.Setenv("SIRENS_ECHO_DEFINITION", "")
	t.Setenv("SIRENS_ECHO_EVALUATION_PACK", "")

	if got := evaluationDefinitionPath(); got != defaultEvaluationDefinition {
		t.Fatalf("evaluationDefinitionPath() = %q, want %q", got, defaultEvaluationDefinition)
	}
	if got := evaluationPackPath(); got != defaultEvaluationPack {
		t.Fatalf("evaluationPackPath() = %q, want %q", got, defaultEvaluationPack)
	}
}

func TestEvaluationTargetsTheSelectedProfile(t *testing.T) {
	t.Setenv("SIRENS_ECHO_DEFINITION", "agents/deep/definition.yaml")
	t.Setenv("SIRENS_ECHO_EVALUATION_PACK", "agents/deep/packs/evaluation.yaml")

	if got := evaluationDefinitionPath(); got != "agents/deep/definition.yaml" {
		t.Fatalf("evaluationDefinitionPath() = %q, want the Deep definition", got)
	}
	if got := evaluationPackPath(); got != "agents/deep/packs/evaluation.yaml" {
		t.Fatalf("evaluationPackPath() = %q, want the Deep pack", got)
	}
}

// A fixture run must be self-describing. Without this, roster: empty is the only
// signal and it says no tools were available when three were served.
func TestRateProvenanceRecordsTheFixtureAndRunner(t *testing.T) {
	t.Setenv("SIRENS_ECHO_TOOL_FIXTURE", "agent/tool-fixture-injection.yaml")
	t.Setenv("SIRENS_ECHO_RUNNER", "abc1234")
	t.Setenv("SIRENS_ECHO_SUBSTRATE", "")
	t.Setenv("SIRENS_ECHO_IMAGE", "")
	got := rateProvenance("agents/deep/packs/rate-fixture.yaml", "http://proxy", "route", "")
	if got.Fixture != "agent/tool-fixture-injection.yaml" {
		t.Errorf("Fixture = %q, want the fixture path", got.Fixture)
	}
	if got.Runner == community.RunnerUnrecorded || got.Runner == "" {
		t.Errorf("Runner = %q, want the recorded revision", got.Runner)
	}
	if got.Image != community.ImageUnrecorded {
		t.Errorf("Image = %q, want %q since no pod participates",
			got.Image, community.ImageUnrecorded)
	}
}

// A run with no fixture says so, rather than leaving the field empty and letting
// a reader guess whether it was absent or unrecorded.
func TestRateProvenanceNamesTheAbsenceOfAFixture(t *testing.T) {
	t.Setenv("SIRENS_ECHO_TOOL_FIXTURE", "")
	t.Setenv("SIRENS_ECHO_RUNNER", "")
	got := rateProvenance("agents/deep/packs/rate.yaml", "http://proxy", "route", "")
	if got.Fixture != community.FixtureNone {
		t.Errorf("Fixture = %q, want %q", got.Fixture, community.FixtureNone)
	}
	if got.Roster != "empty" {
		t.Errorf("Roster = %q, want empty", got.Roster)
	}
}

// An evaluation run must not report as the deployment. Left unset the instance
// name fell back to sirens-echo. See sirens-echo#533.
func TestEvaluationDoesNotReportAsTheDeployment(t *testing.T) {
	t.Parallel()
	for _, deployment := range []string{"sirens-echo", "sirens-deep"} {
		if evaluationInstanceName == deployment {
			t.Fatalf("evaluation reports as the %s deployment", deployment)
		}
	}
	// Lowercase and hyphenated, because the config validator rejects anything
	// else and a rejected name would fall back to the default it replaced.
	for _, r := range evaluationInstanceName {
		if (r < 'a' || r > 'z') && (r < '0' || r > '9') && r != '-' {
			t.Fatalf("instance name %q is not a service name", evaluationInstanceName)
		}
	}
	if evaluationInstanceName == "" {
		t.Fatal("an empty instance name falls back to the deployment's")
	}
}

func gitRepo(t *testing.T, ignore string) string {
	t.Helper()
	dir := t.TempDir()
	if out, err := exec.Command("git", "-C", dir, "init", "-q").CombinedOutput(); err != nil {
		t.Skipf("git init: %v %s", err, out)
	}
	if ignore != "" {
		if err := os.WriteFile(filepath.Join(dir, ".gitignore"), []byte(ignore), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func TestALiveRunWritesAnUntrackedMarkedFileAndNothingToStdout(t *testing.T) {
	repo := gitRepo(t, "/.eval-live/\n")
	t.Chdir(repo)
	t.Setenv(liveDirEnv, "")
	var stdout bytes.Buffer
	var logged string
	logf := func(format string, args ...any) { logged = fmt.Sprintf(format, args...) }
	now := time.Date(2026, 11, 4, 19, 30, 0, 0, time.UTC)

	out, err := evaluationOutput("roster.yaml", "agents/echo/packs/rate.yaml", &stdout, now, logf)
	if err != nil {
		t.Fatalf("evaluationOutput: %v", err)
	}
	if _, err := fmt.Fprintln(out, "reply naming a real player"); err != nil {
		t.Fatal(err)
	}

	path := filepath.Join(repo, ".eval-live", "20261104T193000Z-rate.out")
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("the live dataset is not at %s: %v", path, err)
	}
	if !community.HasLiveRunMarker(body) || !strings.HasPrefix(string(body), community.LiveRunMarker) {
		t.Errorf("the file does not open with the live marker:\n%s", body)
	}
	if stdout.Len() != 0 {
		t.Errorf("a live run wrote %q to stdout, where a redirect can land it in the tracked tree", stdout.String())
	}
	if info, _ := os.Stat(path); info.Mode().Perm() != 0o600 {
		t.Errorf("mode = %v, want 0600", info.Mode().Perm())
	}
	if !strings.Contains(logged, "untracked") {
		t.Errorf("the run did not say where the output went: %q", logged)
	}
	status, err := exec.Command("git", "-C", repo, "status", "--porcelain", "--untracked-files=all").Output()
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(status), ".eval-live") {
		t.Errorf("git status shows the live output as a candidate:\n%s", status)
	}
}

func TestAnOfflineOrFixtureRunKeepsStdoutAndWritesNoFile(t *testing.T) {
	repo := gitRepo(t, "")
	t.Chdir(repo)
	var stdout bytes.Buffer

	out, err := evaluationOutput("", "agents/echo/packs/rate.yaml", &stdout, time.Now(), t.Logf)
	if err != nil {
		t.Fatalf("evaluationOutput: %v", err)
	}
	if out != io.Writer(&stdout) {
		t.Fatal("a run naming no roster did not keep stdout, so a fixture record can no longer be committed")
	}
	if _, err := os.Stat(filepath.Join(repo, defaultLiveDir)); err == nil {
		t.Error("an offline run created the live directory")
	}
}

func TestALiveRunRefusesADirectoryGitWouldTrack(t *testing.T) {
	repo := gitRepo(t, "")
	t.Chdir(repo)
	t.Setenv(liveDirEnv, "")

	_, err := evaluationOutput("roster.yaml", "pack.yaml", io.Discard, time.Now(), t.Logf)

	if err == nil || !strings.Contains(err.Error(), "not ignored") {
		t.Fatalf("err = %v, want a refusal naming the missing ignore rule", err)
	}
	entries, _ := os.ReadDir(filepath.Join(repo, defaultLiveDir))
	if len(entries) != 0 {
		t.Errorf("a refused run still wrote %d file(s)", len(entries))
	}
}

func TestALiveRunOutsideARepositoryHasNothingToTrack(t *testing.T) {
	t.Setenv(liveDirEnv, filepath.Join(t.TempDir(), "live"))

	if _, err := evaluationOutput("roster.yaml", "pack.yaml", io.Discard, time.Now(), t.Logf); err != nil {
		t.Fatalf("a directory outside any repository was refused: %v", err)
	}
}
