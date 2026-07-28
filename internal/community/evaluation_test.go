package community

import (
	"context"
	"io"
	"path/filepath"
	"testing"
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
	definition, aosContext, skillpack, pack := loadEvaluationFixture(t)
	client := fakeCompletionClient{responses: map[string]CompletionResult{
		"unknown-event-time": {
			Content: `{"reply":"I cannot confirm the time from the approved information. The member's guess is not verification.","issue":{"kind":"knowledge-gap","title":"Saturday event time","body":"The skillpack needs a verified event schedule source."}}`,
		},
		"explicit-correction": {
			Content: `{"reply":"Thanks for flagging that. I will treat my earlier answer as unverified while a steward reviews the source.","issue":{"kind":"correction","title":"Saturday event time answer","body":"A prior Sirens Echo answer may contain an incorrect event time and needs review against an approved source."}}`,
		},
		"eco-live-status": {
			Content: `{"reply":"The Eco tool reports that the server is online.","issue":null}`,
			ToolCalls: []ExecutedTool{
				{Name: "eco__get_eco_server_status"},
			},
		},
	}}
	if err := RunEvaluation(
		context.Background(),
		definition,
		aosContext,
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
	definition, aosContext, skillpack, pack := loadEvaluationFixture(t)
	client := fakeCompletionClient{responses: map[string]CompletionResult{
		"unknown-event-time": {
			Content: `{"reply":"Check #events.","issue":{"kind":"knowledge-gap","title":"Event time","body":"The time is unknown."}}`,
		},
		"explicit-correction": {
			Content: `{"reply":"Thanks for flagging that.","issue":{"kind":"correction","title":"Event time","body":"The answer may be wrong."}}`,
		},
		"eco-live-status": {
			Content: `{"reply":"The Eco tool reports that the server is online.","issue":null}`,
			ToolCalls: []ExecutedTool{
				{Name: "eco__get_eco_server_status"},
			},
		},
	}}
	if err := RunEvaluation(
		context.Background(),
		definition,
		aosContext,
		skillpack,
		pack,
		client,
		io.Discard,
	); err == nil {
		t.Fatal("expected evaluation failure")
	}
}

func loadEvaluationFixture(t *testing.T) (Definition, string, string, EvaluationPack) {
	t.Helper()
	definition, err := LoadDefinition(filepath.Join("..", "..", "agent", "sirens-echo.json"))
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
	return definition, "verified AOS Community context", skillpack, pack
}
