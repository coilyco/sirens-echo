package community

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
	"time"
)

const defaultEvaluationCaseTimeout = 5 * time.Minute

// EvaluationPack is the source-controlled live-path acceptance gate.
type EvaluationPack struct {
	Schema string           `json:"schema"`
	Cases  []EvaluationCase `json:"cases"`
}

// EvaluationCase exercises the same prompt and parser used by Discord without
// sending a Discord reply or creating a Forgejo issue.
type EvaluationCase struct {
	ID                string            `json:"id"`
	History           []TranscriptEntry `json:"history"`
	Current           TranscriptEntry   `json:"current"`
	RequiredIssueKind string            `json:"required_issue_kind"`
	RequiredTool      string            `json:"required_tool"`
	ForbiddenPhrases  []string          `json:"forbidden_phrases"`
}

// LoadEvaluationPack reads the deterministic deployment gate.
func LoadEvaluationPack(path string) (EvaluationPack, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return EvaluationPack{}, fmt.Errorf("read evaluation pack: %w", err)
	}
	var pack EvaluationPack
	if err := json.Unmarshal(raw, &pack); err != nil {
		return EvaluationPack{}, fmt.Errorf("parse evaluation pack: %w", err)
	}
	if pack.Schema != "sirens-discord-ops.evaluation.v1" {
		return EvaluationPack{}, fmt.Errorf("unsupported evaluation schema %q", pack.Schema)
	}
	if len(pack.Cases) == 0 {
		return EvaluationPack{}, fmt.Errorf("evaluation pack contains no cases")
	}
	for _, evaluationCase := range pack.Cases {
		if evaluationCase.ID == "" || evaluationCase.Current.Content == "" {
			return EvaluationPack{}, fmt.Errorf("evaluation case requires id and current content")
		}
		if evaluationCase.RequiredIssueKind != "" &&
			evaluationCase.RequiredIssueKind != "knowledge-gap" &&
			evaluationCase.RequiredIssueKind != "correction" {
			return EvaluationPack{}, fmt.Errorf(
				"evaluation case %s has unsupported issue kind %q",
				evaluationCase.ID,
				evaluationCase.RequiredIssueKind,
			)
		}
		if evaluationCase.RequiredIssueKind == "" &&
			evaluationCase.RequiredTool == "" &&
			len(evaluationCase.ForbiddenPhrases) == 0 {
			return EvaluationPack{}, fmt.Errorf(
				"evaluation case %s requires an issue kind, tool, or forbidden phrase",
				evaluationCase.ID,
			)
		}
	}
	return pack, nil
}

// RunEvaluation executes the accepted Discord cases through Agent Proxy using
// the production prompt and structural validators. It performs no writes.
func RunEvaluation(
	ctx context.Context,
	definition Definition,
	localSkillpack string,
	pack EvaluationPack,
	completions CompletionClient,
	output io.Writer,
) error {
	return runEvaluation(
		ctx,
		definition,
		localSkillpack,
		pack,
		completions,
		output,
		defaultEvaluationCaseTimeout,
	)
}

func runEvaluation(
	ctx context.Context,
	definition Definition,
	localSkillpack string,
	pack EvaluationPack,
	completions CompletionClient,
	output io.Writer,
	caseTimeout time.Duration,
) error {
	systemPrompt := BuildSystemPrompt(definition, localSkillpack)
	failures := make([]string, 0)
	for _, evaluationCase := range pack.Cases {
		userPrompt := BuildUserPrompt(evaluationCase.History, evaluationCase.Current)
		caseCtx, cancel := context.WithTimeout(ctx, caseTimeout)
		result, err := completions.Complete(
			caseCtx,
			systemPrompt,
			userPrompt,
			evaluationCase.ID,
		)
		cancel()
		if err != nil {
			failures = append(failures, fmt.Sprintf("%s: inference: %v", evaluationCase.ID, err))
			continue
		}
		decision, err := ParseDecision(result.Content)
		if err == nil {
			err = ValidateGrounding(decision, systemPrompt+"\n"+userPrompt, result.ToolCalls...)
		}
		if err == nil {
			err = ValidateResponseStyle(definition.ResponseStyle, decision)
		}
		if err == nil && evaluationCase.RequiredIssueKind != "" &&
			(decision.Issue == nil || decision.Issue.Kind != evaluationCase.RequiredIssueKind) {
			err = fmt.Errorf("expected %s issue draft", evaluationCase.RequiredIssueKind)
		}
		if err == nil && evaluationCase.RequiredTool != "" &&
			!completionUsedTool(result, evaluationCase.RequiredTool) {
			err = fmt.Errorf("expected tool %s", evaluationCase.RequiredTool)
		}
		if err == nil {
			lowerOutput := strings.ToLower(decision.Reply)
			if decision.Issue != nil {
				lowerOutput += strings.ToLower("\n" + decision.Issue.Title + "\n" + decision.Issue.Body)
			}
			for _, forbidden := range evaluationCase.ForbiddenPhrases {
				if strings.Contains(lowerOutput, strings.ToLower(forbidden)) {
					err = fmt.Errorf("contained forbidden phrase %q", forbidden)
					break
				}
			}
		}
		if err != nil {
			failures = append(failures, fmt.Sprintf("%s: %v", evaluationCase.ID, err))
			continue
		}
		fmt.Fprintf(output, "%s: pass\n%s\n\n", evaluationCase.ID, decision.Reply)
	}
	if len(failures) > 0 {
		return fmt.Errorf("evaluation failed:\n%s", strings.Join(failures, "\n"))
	}
	return nil
}

func completionUsedTool(result CompletionResult, required string) bool {
	for _, call := range result.ToolCalls {
		if call.Name == required {
			return true
		}
	}
	return false
}
