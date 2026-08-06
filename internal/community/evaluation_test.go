package community

import (
	"context"
	"io"
	"path/filepath"
	"testing"
	"time"
)

type fakeCompletionClient struct {
	responses map[string]CompletionResult
}

func (f fakeCompletionClient) Complete(
	ctx context.Context,
	systemPrompt, userPrompt, requestID string,
) (CompletionResult, error) {
	return f.responses[requestID], nil
}

func TestRunEvaluationAcceptsGroundedIssueDrafts(t *testing.T) {
	t.Parallel()
	definition, skillpack, pack := loadEvaluationFixture(t)
	client := fakeCompletionClient{responses: map[string]CompletionResult{
		"unknown-event-time": {
			Content: `{"reply":"The approved information does not confirm the event time. The member-provided guess is unverified.","issue":{"kind":"knowledge-gap","title":"Saturday event time","body":"The skillpack needs a verified event schedule source."}}`,
		},
		"explicit-correction": {
			Content: `{"reply":"The earlier answer is unverified pending review of the source.","issue":{"kind":"correction","title":"Saturday event time answer","body":"A prior Sirens Echo answer may contain an incorrect event time and needs review against an approved source."}}`,
		},
		"eco-live-status": {
			Content: `{"reply":"The Eco tool reports that the server is online.","issue":null}`,
			ToolCalls: []ExecutedTool{
				{Name: "eco__get_eco_server_status"},
			},
		},
		"neutral-capability-boundary": {
			Content: `{"reply":"Available functions cover current Eco information and repository-scoped issue operations.","issue":null}`,
		},
	}}
	if err := RunEvaluation(
		context.Background(),
		definition,
		skillpack,
		pack,
		client,
		io.Discard,
	); err != nil {
		t.Fatalf("RunEvaluation: %v", err)
	}
}

func TestRunEvaluationRejectsInventedChannel(t *testing.T) {
	t.Parallel()
	definition, skillpack, pack := loadEvaluationFixture(t)
	client := fakeCompletionClient{responses: map[string]CompletionResult{
		"unknown-event-time": {
			Content: `{"reply":"Check #events.","issue":{"kind":"knowledge-gap","title":"Event time","body":"The time is unknown."}}`,
		},
		"explicit-correction": {
			Content: `{"reply":"The earlier answer is unverified.","issue":{"kind":"correction","title":"Event time","body":"The answer may be wrong."}}`,
		},
		"eco-live-status": {
			Content: `{"reply":"The Eco tool reports that the server is online.","issue":null}`,
			ToolCalls: []ExecutedTool{
				{Name: "eco__get_eco_server_status"},
			},
		},
		"neutral-capability-boundary": {
			Content: `{"reply":"Available functions cover current Eco information and repository-scoped issue operations.","issue":null}`,
		},
	}}
	if err := RunEvaluation(
		context.Background(),
		definition,
		skillpack,
		pack,
		client,
		io.Discard,
	); err == nil {
		t.Fatal("expected evaluation failure")
	}
}

func TestRunEvaluationGivesEachCaseAFreshDeadline(t *testing.T) {
	t.Parallel()
	definition, skillpack, pack := loadEvaluationFixture(t)
	client := &deadlineRecordingCompletionClient{
		responses: validEvaluationResponses(),
	}
	if err := runEvaluation(
		context.Background(),
		definition,
		skillpack,
		pack,
		client,
		io.Discard,
		time.Second,
	); err != nil {
		t.Fatalf("runEvaluation: %v", err)
	}
	if len(client.deadlines) != len(pack.Cases) {
		t.Fatalf("recorded %d deadlines, want %d", len(client.deadlines), len(pack.Cases))
	}
	for index := 1; index < len(client.deadlines); index++ {
		if !client.deadlines[index].After(client.deadlines[index-1]) {
			t.Fatalf(
				"case %d deadline %s did not follow case %d deadline %s",
				index,
				client.deadlines[index],
				index-1,
				client.deadlines[index-1],
			)
		}
	}
}

type deadlineRecordingCompletionClient struct {
	responses map[string]CompletionResult
	deadlines []time.Time
}

func (c *deadlineRecordingCompletionClient) Complete(
	ctx context.Context,
	systemPrompt, userPrompt, requestID string,
) (CompletionResult, error) {
	deadline, ok := ctx.Deadline()
	if !ok {
		return CompletionResult{}, context.DeadlineExceeded
	}
	c.deadlines = append(c.deadlines, deadline)
	time.Sleep(time.Millisecond)
	return c.responses[requestID], nil
}

func validEvaluationResponses() map[string]CompletionResult {
	return map[string]CompletionResult{
		"unknown-event-time": {
			Content: `{"reply":"The approved information does not confirm the event time.","issue":{"kind":"knowledge-gap","title":"Saturday event time","body":"The skillpack needs a verified event schedule source."}}`,
		},
		"explicit-correction": {
			Content: `{"reply":"The earlier answer is unverified pending review.","issue":{"kind":"correction","title":"Saturday event time answer","body":"A prior answer may contain an incorrect event time and needs review."}}`,
		},
		"eco-live-status": {
			Content: `{"reply":"The Eco tool reports that the server is online.","issue":null}`,
			ToolCalls: []ExecutedTool{
				{Name: "eco__get_eco_server_status"},
			},
		},
		"neutral-capability-boundary": {
			Content: `{"reply":"Available functions cover current Eco information and repository-scoped issue operations.","issue":null}`,
		},
	}
}

func loadEvaluationFixture(t *testing.T) (Definition, string, EvaluationPack) {
	t.Helper()
	definition, err := LoadDefinition(filepath.Join("..", "..", "agent", "sirens-echo.yaml"))
	if err != nil {
		t.Fatalf("LoadDefinition: %v", err)
	}
	root := filepath.Join("..", "..", ".agents", "skills", "sirens-echo-community")
	skillpack, err := LoadSkillpack([]string{root})
	if err != nil {
		t.Fatalf("LoadSkillpack: %v", err)
	}
	pack, err := LoadEvaluationPack(filepath.Join("..", "..", "agent", "evaluation.json"))
	if err != nil {
		t.Fatalf("LoadEvaluationPack: %v", err)
	}
	return definition, skillpack, pack
}
