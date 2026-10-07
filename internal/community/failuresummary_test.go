package community

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"testing"
)

const summarySecret = "https://sirens-deep-forgejo-mcp:8080/mcp?token=abc123 member said hunter2"

func TestErrorSummaryNamesTheReasonAndTheTypeNeverTheText(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		err  error
		want []string
	}{
		{"unclassified", errors.New(summarySecret), []string{"reason=unlabelled", "chain=*errors.errorString"}},
		{"repair tool call", fmt.Errorf("%s: %w", summarySecret, ErrRepairToolCall), []string{"reason=repair_tool_call"}},
		{"no roster", fmt.Errorf("%s: %w", summarySecret, ErrNoToolRoster), []string{"reason=no_tool_roster"}},
		{"no choices", fmt.Errorf("%s: %w", summarySecret, ErrNoChoices), []string{"reason=no_choices"}},
		{
			"transport",
			fmt.Errorf("Agent Proxy request: %w: %w", ErrModelTransport, &url.Error{Op: "Post", URL: summarySecret, Err: errors.New(summarySecret)}),
			[]string{"reason=model_transport", "*url.Error"},
		},
		{"tool surface", ToolFailure{Server: "forgejo", Tool: "list", Err: errors.New(summarySecret)}, []string{"reason=tool_failed", "community.ToolFailure"}},
		{"backend status", fmt.Errorf("%s: %w", summarySecret, modelHTTPError{Status: 502}), []string{"reason=unlabelled", "http_status=502"}},
		{"backend rejection", modelHTTPError{Status: 400}, []string{"reason=model_rejected", "http_status=400"}},
		{"deadline", fmt.Errorf("%s: %w", summarySecret, context.DeadlineExceeded), []string{"reason=deadline_exceeded"}},
		{"joined with a notice failure", errors.Join(ErrNoChoices, errors.New(summarySecret)), []string{"reason=no_choices", "*errors.errorString"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := errorSummary(tc.err)
			for _, want := range tc.want {
				if !strings.Contains(got, want) {
					t.Errorf("summary %q lacks %q", got, want)
				}
			}
			for _, leaked := range []string{"abc123", "8080", "hunter2", "sirens-deep", "forgejo", "token", "member said"} {
				if strings.Contains(got, leaked) {
					t.Errorf("summary %q leaked %q", got, leaked)
				}
			}
			if strings.ContainsAny(got, "\n\r") {
				t.Errorf("summary %q spans lines", got)
			}
		})
	}
}

func TestErrorSummaryIsBounded(t *testing.T) {
	t.Parallel()
	deep := errors.New("root")
	for range 40 {
		deep = fmt.Errorf("layer %s: %w", summarySecret, deep)
	}
	if got := errorSummary(deep); len(got) > 200 {
		t.Errorf("summary is %d bytes, want at most 200", len(got))
	}
	if got := errorSummary(nil); got != "" {
		t.Errorf("summary of nil = %q, want empty", got)
	}
}

func TestEveryProxyReturnLeftInStageFailedHasItsOwnReason(t *testing.T) {
	t.Parallel()
	for _, sentinel := range []error{ErrRepairToolCall, ErrNoToolRoster, ErrNoChoices, ErrModelTransport} {
		if got := failureCause(sentinel); got != causeStage {
			t.Errorf("failureCause(%v) = %q, want it left in %q so no notice or alert moves", sentinel, got, causeStage)
		}
		if got := summaryReason(sentinel); got == "unlabelled" {
			t.Errorf("%v has no reason of its own", sentinel)
		}
	}
}

func TestTurnStageFailedLogCarriesTheSummaryAndNotTheText(t *testing.T) {
	telemetry, _, logs := jobTelemetry(t)
	agent := failingAgent(fmt.Errorf("%s: %w", summarySecret, ErrNoChoices))
	agent.telemetry = telemetry

	if err := agent.runTurn(context.Background(), &httpTurn{requestID: "summary"}, nil); err == nil {
		t.Fatal("runTurn returned no error")
	}

	logged := logs.String()
	if !strings.Contains(logged, "turn.stage.failed") || !strings.Contains(logged, `"error_summary":"reason=no_choices chain=`) {
		t.Fatalf("turn.stage.failed carries no summary:\n%s", logged)
	}
	for _, leaked := range []string{"abc123", "hunter2", "sirens-deep-forgejo-mcp"} {
		if strings.Contains(logged, leaked) {
			t.Errorf("log leaked %q", leaked)
		}
	}
}

// A tag keeps the fingerprint the class an alert counts. Shares the Sentry client
// with turnfailure_test.go, so it does not run in parallel.
func TestAFailedTurnEventCarriesTheSummaryAsATagNotInTheFingerprint(t *testing.T) {
	transport := withCapturedCrashes(t)
	agent := failingAgent(fmt.Errorf("%s: %w", summarySecret, ErrRepairToolCall))

	if err := agent.runTurn(context.Background(), &httpTurn{requestID: "tagged"}, nil); err == nil {
		t.Fatal("runTurn returned no error")
	}

	sent := transport.sent()
	if len(sent) != 1 {
		t.Fatalf("sent %d events, want 1", len(sent))
	}
	got := sent[0].Tags["sirens_echo.error_summary"]
	if !strings.HasPrefix(got, "reason=repair_tool_call chain=") {
		t.Errorf("summary tag = %q", got)
	}
	if len(sent[0].Fingerprint) != 2 || sent[0].Fingerprint[1] != causeStage {
		t.Errorf("fingerprint = %v, want it still grouped by class alone", sent[0].Fingerprint)
	}
	for _, leaked := range []string{"abc123", "hunter2"} {
		if strings.Contains(got, leaked) {
			t.Errorf("tag leaked %q", leaked)
		}
	}
}
