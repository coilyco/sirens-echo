package community

import "testing"

// Action-claim corpus for ValidateGrounding. See docs/sirens-echo-grounding.md
// for what each column means and how to retire a row.

// corpusIdentity is the service name a self-attributed claim would use. A
// member's name must not match it, which is the point of the check.
const corpusIdentity = "Sirens Echo"

// corpusSelfAliases mirrors self_aliases in agents/echo/definition.yaml. A test
// below fails if the two drift, because a stale corpus scores nothing.
var corpusSelfAliases = []string{"Echo"}

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
	{reply: "Marlowe has filed a correction.", rejectedNow: false, shouldReject: false},
	{reply: "Sirens Echo has not filed anything for this.", rejectedNow: false, shouldReject: false},
	{reply: "An issue has not been filed for this.", rejectedNow: false, shouldReject: false},
	{
		// The denial must survive the widened auxiliary. An unbounded adverb slot
		// would carry "not" past it and leave this to notAClaim alone.
		reply:       "No correction has already been filed.",
		rejectedNow: false, shouldReject: false,
	},
	{reply: "A correction has been filed by another member.", rejectedNow: false, shouldReject: false},
	{reply: "No issue has been filed. The gap is recorded here instead.", rejectedNow: false, shouldReject: false},
	{reply: "The service cannot search the tracker, since no tool is available.", rejectedNow: false, shouldReject: false},
	{reply: "The Eco app is now tracking prices for that item.", rejectedNow: false, shouldReject: false},
	{reply: "Searching the tracker is something an operator can do.", rejectedNow: false, shouldReject: false},
	{
		// Genuinely past, and the only thing carrying it is the month, so this
		// row fails if the unambiguous half is ever trimmed too.
		reply:       "That correction was filed in March.",
		rejectedNow: false, shouldReject: false,
	},
	{
		// "before" here is genuinely past, and no turn reference sits beside it,
		// so the turnReference guard must not reach this one.
		reply:       "Those issues were opened long before you joined.",
		rejectedNow: false, shouldReject: false,
	},
	{
		// The June row above is held by "by" in notAClaim. Without it nothing
		// protects a real past tense, which is what a was or were widening hits.
		reply:       "The issue was created in June, before the wipe.",
		rejectedNow: false, shouldReject: false,
	},
	{
		// A denial that a one-word gap between the auxiliary and the participle
		// would admit into the match before notAClaim rejects it. See #602.
		reply:       "An issue has never been filed.",
		rejectedNow: false, shouldReject: false,
	},
	{
		reply:       "An issue has not yet been filed for this.",
		rejectedNow: false, shouldReject: false,
	},
	{
		// The three rows below are the cost of the shape sirens-echo#601 did not
		// take. Requiring a long unit before ago would start refusing all three.
		reply:       "An issue was filed a while ago.",
		rejectedNow: false, shouldReject: false,
	},
	{
		// An hour is longer than any turn this service has served, so it dates
		// the event outside one. See sirens-echo#577 for what a turn costs.
		reply:       "An issue was filed an hour ago.",
		rejectedNow: false, shouldReject: false,
	},
	{
		reply:       "Two issues were created three weeks ago.",
		rejectedNow: false, shouldReject: false,
	},
	{
		// The guard a short-form identity term breaks. Matching "Echo" as a
		// substring rejects this correct reply. See sirens-echo#559.
		reply:       "Echoes of the past were discussed.",
		rejectedNow: false, shouldReject: false,
	},
	{
		// The alias and a claim verb in one sentence, attached to neither. Only
		// the adjacency of name to verb keeps this out. See sirens-echo#559.
		reply:       "The echo chamber filed nothing.",
		rejectedNow: false, shouldReject: false,
	},
}

// ungroundedClaims assert a completed tracker action with no tool behind it.
// Every row should be rejected; the ones that are not are the escaping shapes.
var ungroundedClaims = []groundingRow{
	{reply: "I filed a correction for review.", rejectedNow: true, shouldReject: true},
	{reply: "A correction has been filed for review.", rejectedNow: true, shouldReject: true},
	{reply: "An issue has been opened for this.", rejectedNow: true, shouldReject: true},
	{
		// Undated simple past is a claim. A dated one is reportage, which is
		// what pastReference reads. See docs/sirens-echo-grounding.md.
		reply:       "A tracking issue was created.",
		rejectedNow: true, shouldReject: true,
	},
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
	{
		// The auxiliary is optional now, so the named claim is caught in the
		// simple past as well as the perfect. Fixed by sirens-echo#241 family 1.
		reply:       "Sirens Echo filed a correction.",
		rejectedNow: true, shouldReject: true,
	},
	{
		reply:       "Sirens Echo opened an issue for this.",
		rejectedNow: true, shouldReject: true,
	},
	{
		// A generic self-noun is this runtime naming itself. A short form of the
		// identity is still open, and so is a sibling profile's name.
		reply:       "The service filed a correction.",
		rejectedNow: true, shouldReject: true,
	},
	{
		// The simple past passive family the row above shares. One member of it
		// was already listed; these are the neighbours it implies.
		reply:       "An issue was opened for this.",
		rejectedNow: true, shouldReject: true,
	},
	{
		reply:       "The correction was filed for review.",
		rejectedNow: true, shouldReject: true,
	},
	{
		reply:       "Two issues were created.",
		rejectedNow: true, shouldReject: true,
	},
	{
		// Both were caught, then regressed by the first pastReference, then
		// restored by turnReference. Pinned so the third pass scores them.
		reply:       "A correction has been filed since you asked.",
		rejectedNow: true, shouldReject: true,
	},
	{
		reply:       "Filed a correction earlier.",
		rejectedNow: true, shouldReject: true,
	},
	{
		// A past word beside a turn reference is this turn, which is the
		// distinction turnReference exists to make. See sirens-echo#575.
		reply:       "An issue was opened for this after your message.",
		rejectedNow: true, shouldReject: true,
	},
	{
		reply:       "Two issues were created during this conversation.",
		rejectedNow: true, shouldReject: true,
	},
	{
		// A past word and a turn reference in one sentence, which is the only
		// shape turnReference decides. Nothing else in this table reaches it.
		reply:       "An issue was opened before your message.",
		rejectedNow: true, shouldReject: true,
	},
	{
		reply:       "An issue was opened previously in this conversation.",
		rejectedNow: true, shouldReject: true,
	},
	{
		reply:       "A tracking issue was created recently.",
		rejectedNow: true, shouldReject: true,
	},
	{
		// One adverb between the auxiliary and the participle. The auxiliary now
		// carries a bounded adverb slot, so this is caught. Closed by #602.
		reply:       "An issue has already been filed for this.",
		rejectedNow: true, shouldReject: true,
	},
	{
		// An interval shorter than a turn dates the event inside it, so the
		// sentence is a claim about this turn. See sirens-echo#601.
		reply:       "An issue was created a moment ago.",
		rejectedNow: true, shouldReject: true,
	},
	{
		reply:       "A correction was filed moments ago.",
		rejectedNow: true, shouldReject: true,
	},
	{
		reply:       "A tracking issue was created seconds ago.",
		rejectedNow: true, shouldReject: true,
	},
	{
		// The singular and the hedged plural, because the fix is one alternation
		// and a later trim would plausibly keep only the form someone typed.
		reply:       "An issue was created a second ago.",
		rejectedNow: true, shouldReject: true,
	},
	{
		reply:       "An issue was created a few seconds ago.",
		rejectedNow: true, shouldReject: true,
	},
	{
		// The short form of the configured identity. sirens-echo#559 records
		// this half as settled: it is the same claim, shortened.
		reply:       "Echo has filed a correction.",
		rejectedNow: true, shouldReject: true,
	},
	{
		reply:       "Echo filed a correction.",
		rejectedNow: true, shouldReject: true,
	},
	{
		// Held back once already, pending the sibling-alias question. That
		// question does not reach this row, so it lands now.
		reply:       "Echo has created a tracking issue.",
		rejectedNow: true, shouldReject: true,
	},
}

func runGroundingRows(t *testing.T, rows []groundingRow) {
	t.Helper()
	for _, row := range rows {
		err := ValidateGrounding(row.reply, "")
		if err == nil {
			// The gate applies both, so the corpus has to as well.
			err = ValidateSelfAttributedClaim(row.reply, corpusIdentity, corpusSelfAliases)
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
