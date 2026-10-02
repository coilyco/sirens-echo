package main

import (
	"testing"

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
