package community

import "testing"

// Action-claim corpus for ValidateGrounding. See docs/sirens-echo-grounding-corpus.md
// for what each column means and how to retire a row.

// corpusIdentity is the service name a self-attributed claim would use. A
// member's name must not match it, which is the point of the check.
const corpusIdentity = "Sirens Echo"

// groundingRow pairs a reply with what the validator does today and what it
// ought to do. A row where the two disagree is an open defect, not a target.
type groundingRow struct {
	reply string
	// rejectedNow is the behavior on origin/main. The test asserts this, so CI
	// stays honest about what ships rather than about what is wanted.
	rejectedNow bool
	// shouldReject is the intended behavior. Where it differs from rejectedNow
	// the row names the issue that closes the gap.
	shouldReject bool
	issue        string
}

// falsePositives are correct replies the detector must not reject. This half is
// where a widened pattern regresses, so it is the half worth having.
var falsePositives = []groundingRow{
	{
		reply:       "No issue has been filed for this.",
		rejectedNow: false, shouldReject: false,
	},
	{
		reply:       "No correction has been filed.",
		rejectedNow: false, shouldReject: false,
	},
	{
		reply:       "I cannot confirm whether an issue has been filed.",
		rejectedNow: false, shouldReject: false,
	},
	{
		reply:       "You asked whether a correction has been filed. It has not.",
		rejectedNow: false, shouldReject: false,
	},
	{
		reply:       "The issue was created in June by another member.",
		rejectedNow: false, shouldReject: false,
	},
	{
		reply:       "That issue was closed last week, before this thread started.",
		rejectedNow: false, shouldReject: false,
	},
	{
		reply:       "If an issue is filed, it will appear in the tracker.",
		rejectedNow: false, shouldReject: false,
	},
	{reply: "Has an issue been filed for this yet?", rejectedNow: false, shouldReject: false},
	{reply: "An issue would be created if the threshold were breached.", rejectedNow: false, shouldReject: false},
	{reply: "An issue can be filed if you want one.", rejectedNow: false, shouldReject: false},
	{reply: "An issue will be filed once you confirm the details.", rejectedNow: false, shouldReject: false},
	{reply: "Your message was posted to the wrong channel.", rejectedNow: false, shouldReject: false},
	{reply: "Filed issues are listed in the tracker.", rejectedNow: false, shouldReject: false},
	{reply: "Octavian has filed a correction.", rejectedNow: false, shouldReject: false},
	{reply: "Sirens Echo has not filed anything for this.", rejectedNow: false, shouldReject: false},
	{reply: "An issue has not been filed for this.", rejectedNow: false, shouldReject: false},
	{reply: "A correction has been filed by another member.", rejectedNow: false, shouldReject: false},
	{reply: "No issue has been filed. The gap is recorded here instead.", rejectedNow: false, shouldReject: false},
	{reply: "The service cannot search the tracker, since no tool is available.", rejectedNow: false, shouldReject: false},
	{reply: "The Eco app is now tracking prices for that item.", rejectedNow: false, shouldReject: false},
	{reply: "Searching the tracker is something an operator can do.", rejectedNow: false, shouldReject: false},
}

// ungroundedClaims assert a completed tracker action with no tool behind it.
// Every row should be rejected; the ones that are not are the escaping shapes.
var ungroundedClaims = []groundingRow{
	{reply: "I filed a correction for review.", rejectedNow: true, shouldReject: true},
	{reply: "A correction has been filed for review.", rejectedNow: true, shouldReject: true},
	{reply: "An issue has been opened for this.", rejectedNow: true, shouldReject: true},
	{
		// Undated simple past is a claim. A dated one is reportage, which is
		// what the exemption reads. See docs/sirens-echo-grounding.md.
		reply:       "A tracking issue was created.",
		rejectedNow: true, shouldReject: true,
	},
	{reply: "The correction was filed for review.", rejectedNow: true, shouldReject: true},
	{reply: "Sirens Echo has filed a correction.", rejectedNow: true, shouldReject: true},
	{reply: "Filed a correction for review.", rejectedNow: true, shouldReject: true},
	{reply: "Created a tracking issue.", rejectedNow: true, shouldReject: true},
	{
		// A lookup announced with the subject present, which the verb list
		// missed until sirens-echo#341.
		reply:       "Sirens Echo is now searching the tracker.",
		rejectedNow: true, shouldReject: true,
	},
	{
		// Observed live with no issue tool served. The subject is absent, and
		// requiring it is what keeps the correct replies above clean.
		reply:       "Searching the issue tracker for an open ticket.",
		rejectedNow: false, shouldReject: true, issue: "341",
	},
}

func runGroundingRows(t *testing.T, rows []groundingRow) {
	t.Helper()
	for _, row := range rows {
		err := ValidateGrounding(row.reply, "")
		if err == nil {
			// The gate applies both, so the corpus has to as well.
			err = ValidateSelfAttributedClaim(row.reply, corpusIdentity)
		}
		rejected := err != nil
		if rejected == row.rejectedNow {
			continue
		}
		if row.rejectedNow == row.shouldReject {
			t.Errorf("regression on %q: rejected = %v, want %v", row.reply, rejected, row.rejectedNow)
			continue
		}
		t.Errorf("behavior changed on %q: rejected = %v, was %v. If issue %s was fixed, "+
			"set rejectedNow to %v and clear the issue field",
			row.reply, rejected, row.rejectedNow, row.issue, rejected)
	}
}

// A correct reply that denies, hedges, questions, or describes someone else's
// action carries no claim and must reach the member.
func TestGroundingDoesNotRejectCorrectReplies(t *testing.T) {
	t.Parallel()
	runGroundingRows(t, falsePositives)
}

// A reply asserting a completed tracker action with no tool behind it must not
// reach the member, in any grammatical voice.
func TestGroundingRejectsUngroundedActionClaims(t *testing.T) {
	t.Parallel()
	runGroundingRows(t, ungroundedClaims)
}

// A tracker tool in the turn grounds the claim, so the same sentence that fails
// above has to pass here. Without this the detector could just reject the verb.
func TestGroundingAcceptsAClaimATrackerToolSupports(t *testing.T) {
	t.Parallel()
	executed := []ExecutedTool{{Name: "forgejo__create_issue"}}
	for _, reply := range []string{
		"A correction has been filed for review.",
		"An issue has been opened for this.",
		"I filed a correction for review.",
	} {
		if err := ValidateGrounding(reply, "", executed...); err != nil {
			t.Errorf("grounded claim %q rejected: %v", reply, err)
		}
	}
}

// The open rows are reported as one summary so the count is visible without
// reading the table. Use `go test -v` to see it.
func TestGroundingCorpusReportsOpenRows(t *testing.T) {
	t.Parallel()
	open := 0
	for _, row := range append(append([]groundingRow{}, falsePositives...), ungroundedClaims...) {
		if row.rejectedNow == row.shouldReject {
			continue
		}
		open++
		kind := "false positive"
		if row.shouldReject {
			kind = "escapes"
		}
		t.Logf("issue %s, %s: %q", row.issue, kind, row.reply)
	}
	t.Logf("%d rows still disagree with intended behavior", open)
}
