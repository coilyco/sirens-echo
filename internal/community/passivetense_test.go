package community

import "testing"

// The simple past passive is a claim. A dated one is reportage, and refusing a
// correct reply is worse than missing a wrong one. See sirens-echo#555.

const groundedContext = "The current channel is #bots."

// QA's family 3, measured through ValidateGrounding in sirens-echo#241.
func TestTheSimplePastPassiveIsCaught(t *testing.T) {
	t.Parallel()
	for name, reply := range map[string]string{
		"indefinite article": "A tracking issue was created.",
		"with a complement":  "An issue was opened for this.",
		"definite article":   "The correction was filed for review.",
		"plural":             "Two issues were created.",
		"other artifact":     "A ticket was raised for this.",
	} {
		if ValidateGrounding(reply, groundedContext) == nil {
			t.Errorf("%s escaped grounding: %q", name, reply)
		}
	}
}

// The false fire the naive fix produced, and its neighbours. Every one of these
// is something a member could truthfully be told.
func TestDatedReportageIsNotAClaimAboutThisTurn(t *testing.T) {
	t.Parallel()
	for name, reply := range map[string]string{
		"the measured false fire": "The issue was created in June, before the wipe.",
		"a named month":           "That issue was opened in August.",
		"a year":                  "The correction was filed in 2024.",
		"yesterday":               "An issue was created yesterday.",
		"last week":               "The ticket was raised last week.",
		"an interval":             "Two issues were created three weeks ago.",
		"before an event":         "The issue was opened before the last wipe.",
		"previously":              "A correction was previously filed for this.",
		"since":                   "No issue was filed since the migration.",
		"earlier":                 "The bug report was submitted earlier.",
	} {
		if err := ValidateGrounding(reply, groundedContext); err != nil {
			t.Errorf("%s was refused as a claim: %q: %v", name, reply, err)
		}
	}
}

// The perfect must keep behaving exactly as it did. Widening the tense must not
// quietly move the shapes that already worked.
func TestThePerfectPassiveIsUnchanged(t *testing.T) {
	t.Parallel()
	for _, reply := range []string{
		"A correction has been filed for review.",
		"An issue has been opened for this.",
	} {
		if ValidateGrounding(reply, groundedContext) == nil {
			t.Errorf("the perfect stopped being caught: %q", reply)
		}
	}
}

// A turn that actually wrote to the tracker may say so in either tense.
func TestAPerformedWriteStillPermitsThePassive(t *testing.T) {
	t.Parallel()
	executed := ExecutedTool{Name: "forgejo__create_issue", Outcome: ToolOutcomeOK}
	for _, reply := range []string{
		"A tracking issue was created.",
		"A tracking issue has been created.",
	} {
		if err := ValidateGrounding(reply, groundedContext, executed); err != nil {
			t.Errorf("a performed write was refused: %q: %v", reply, err)
		}
	}
}

// The exemption is scoped to the sentence carrying it, the same way polarity is.
// A dated sentence must not excuse an undated claim beside it.
func TestADatedSentenceDoesNotExcuseTheNextOne(t *testing.T) {
	t.Parallel()
	reply := "The wipe happened in June. A tracking issue was created."
	if ValidateGrounding(reply, groundedContext) == nil {
		t.Error("a dated sentence excused an undated claim in the same reply")
	}
}
