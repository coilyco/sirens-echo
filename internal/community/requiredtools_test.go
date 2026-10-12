package community

import (
	"path/filepath"
	"testing"
)

// A case naming a required tool cannot pass when nothing serves it, so a bare
// gate run reports failures about the run. See sirens-echo#357.

func TestTheGateNamesWhichCasesNeedATool(t *testing.T) {
	t.Parallel()
	pack, err := LoadEvaluationPack(filepath.Join("..", "..", "agents", "echo", "packs", "evaluation.yaml"))
	if err != nil {
		t.Fatalf("load the gate: %v", err)
	}
	needing := make([]string, 0, len(pack.Cases))
	for _, evaluationCase := range pack.Cases {
		if evaluationCase.RequiredTool != "" {
			needing = append(needing, evaluationCase.ID)
		}
	}
	if len(needing) == 0 {
		t.Fatal("no gate case requires a tool, so the warning this pins is dead code")
	}
	// The count is not the point. A case gaining or losing a required tool
	// changes what a roster-less run means, which is a decision worth noticing.
	if len(needing) != 1 {
		t.Errorf("%d gate cases require a tool, was 1: %v. A roster-less run now "+
			"reports a different number of unscoreable cases, and the runner's "+
			"warning and docs/sirens-echo-eval.md should say so", len(needing), needing)
	}
}
