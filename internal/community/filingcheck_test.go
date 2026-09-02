package community

import (
	"context"
	"errors"
	"strings"
	"testing"
)

// A polite, on-topic, well-formed request that produced a tracker entry with no
// actionable content is the shape to catch. See sirens-echo#852 and #907.

// stagedClassifier answers each stage in order, so a test says what the two
// stages saw rather than what one of them did.
type stagedClassifier struct {
	answers []string
	asked   []string
	err     error
}

func (c *stagedClassifier) Complete(
	_ context.Context, prompt TurnPrompt, _ string,
) (CompletionResult, error) {
	if c.err != nil {
		return CompletionResult{}, c.err
	}
	c.asked = append(c.asked, prompt.System)
	answer := "work"
	if len(c.asked) <= len(c.answers) {
		answer = c.answers[len(c.asked)-1]
	}
	return CompletionResult{Content: answer}, nil
}

func filingAgent(client CompletionClient, principal Principal) *Agent {
	agent := &Agent{
		cfg:         Config{Principal: principal},
		completions: client,
		telemetry:   telemetryOrNoop(nil),
	}
	agent.ensureRuntimeDefaults()
	return agent
}

// The worked example: courteous, on topic, and nothing to act on.
func TestAPlaceholderRequestIsRefused(t *testing.T) {
	t.Parallel()
	client := &stagedClassifier{answers: []string{filingVerdictPlaceholder}}
	agent := filingAgent(client, Principal{})

	err := agent.checkMemberFiling(context.Background(), "A note for later", "Placeholder.")
	if err == nil {
		t.Fatal("a placeholder request was filed")
	}
	var refused *filingRefused
	if !errors.As(err, &refused) || refused.Stage != filingStageValidity {
		t.Fatalf("refusal = %#v", err)
	}
	// The model reads this and tells the member, so it says what would fix it.
	if !strings.Contains(err.Error(), "Say what should change") {
		t.Errorf("the refusal does not say what would fix it: %q", err.Error())
	}
	if len(client.asked) != 1 {
		t.Errorf("the second stage ran after the first refused: %d calls", len(client.asked))
	}
}

// Both stages run, and the second refuses on its own axis.
func TestAnOutOfScopeRequestIsRefusedBySecondStage(t *testing.T) {
	t.Parallel()
	client := &stagedClassifier{
		answers: []string{filingVerdictWork, filingVerdictOutOfScope},
	}
	agent := filingAgent(client, Principal{})

	err := agent.checkMemberFiling(context.Background(), "Nerf the plank market", "It is too cheap.")
	var refused *filingRefused
	if !errors.As(err, &refused) || refused.Stage != filingStageClassifier {
		t.Fatalf("refusal = %#v", err)
	}
	if len(client.asked) != 2 {
		t.Fatalf("stages run = %d, want both", len(client.asked))
	}
	// The two stages ask different questions, or the second one buys nothing.
	if client.asked[0] == client.asked[1] {
		t.Error("both stages asked the same question")
	}
}

// A request with work in it, about this service, files.
func TestARealRequestPassesBothStages(t *testing.T) {
	t.Parallel()
	client := &stagedClassifier{
		answers: []string{filingVerdictWork, filingVerdictInScope},
	}
	agent := filingAgent(client, Principal{})

	if err := agent.checkMemberFiling(
		context.Background(),
		"Thread titles clip at 30 characters",
		"They should use the full bound.",
	); err != nil {
		t.Fatalf("a real request was refused: %v", err)
	}
}

// The principal is exempt, because an admin filing is not the failure mode.
func TestThePrincipalSkipsTheChecks(t *testing.T) {
	t.Parallel()
	client := &stagedClassifier{answers: []string{filingVerdictPlaceholder}}
	principal := Principal{Handle: "kai", UserID: "1024000000000000001"}
	agent := filingAgent(client, principal)

	ctx := WithRequester(context.Background(), principal.UserID)
	if err := agent.checkMemberFiling(ctx, "A note for later", "Placeholder."); err != nil {
		t.Fatalf("the principal was refused: %v", err)
	}
	if len(client.asked) != 0 {
		t.Errorf("the checks ran for the principal: %d calls", len(client.asked))
	}
}

// A classifier that fails is not a denial, matching the content gate. Filing is
// the thing the member asked for, and a dead checker must not eat it.
func TestAFailedCheckerFilesRatherThanRefusing(t *testing.T) {
	t.Parallel()
	agent := filingAgent(&stagedClassifier{err: errors.New("backend down")}, Principal{})

	if err := agent.checkMemberFiling(context.Background(), "Title", "Body"); err != nil {
		t.Errorf("a failed checker refused the filing: %v", err)
	}
}

// An answer off the list is the checker failing rather than refusing, so it
// cannot invent a verdict that blocks a member.
func TestAnAnswerOffTheListDoesNotRefuse(t *testing.T) {
	t.Parallel()
	agent := filingAgent(&stagedClassifier{answers: []string{"maybe?"}}, Principal{})

	if err := agent.checkMemberFiling(context.Background(), "Title", "Body"); err != nil {
		t.Errorf("an invented verdict refused the filing: %v", err)
	}
}

// The refusal reaches the model as a readable tool result rather than failing
// the turn, so the model can tell the member what would fix it.
func TestARefusedFilingComesBackAsAToolResult(t *testing.T) {
	t.Parallel()
	provider := &TrackerProvider{
		Inner:  &fakeTrackerSession{answers: map[string][]ToolResult{}},
		Policy: sandboxPolicy(),
		FilingCheck: func(context.Context, string, string) error {
			return &filingRefused{Stage: filingStageValidity, Verdict: filingVerdictUnclear}
		},
	}
	session, err := provider.Open(context.Background())
	if err != nil {
		t.Fatalf("open: %v", err)
	}

	got, err := session.Call(context.Background(), "teable__create_issue", map[string]any{
		"title": "something is wrong", "body": "please fix",
	})
	if err != nil {
		t.Fatalf("file: %v", err)
	}
	if !got.IsError {
		t.Fatal("the check was not consulted")
	}
	if !strings.Contains(got.Text, "too vague") {
		t.Errorf("refusal = %q", got.Text)
	}
}

// No check configured files everything, which is what an unset control does
// everywhere else in this service.
func TestNoFilingCheckFilesEverything(t *testing.T) {
	t.Parallel()
	inner := &fakeTrackerSession{answers: map[string][]ToolResult{
		"teable__create_record": {
			{Text: recordPayload("rec1", "org/repo#7", "x", "open")},
			{Text: `{"records":[{"id":"cmt1"}]}`},
		},
		"teable__get_record": {{Text: recordPayload("rec1", "org/repo#7", "x", "open")}},
	}}
	provider := &TrackerProvider{Inner: inner, Policy: sandboxPolicy()}
	session, err := provider.Open(context.Background())
	if err != nil {
		t.Fatalf("open: %v", err)
	}

	got, err := session.Call(context.Background(), "teable__create_issue",
		map[string]any{"title": "x", "body": "y"})
	if err != nil {
		t.Fatalf("file: %v", err)
	}
	if got.IsError {
		t.Errorf("an unset check refused: %q", got.Text)
	}
}
