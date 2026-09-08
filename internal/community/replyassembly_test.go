package community

import (
	"strings"
	"testing"
)

// One case per acceptance criterion on sirens-echo#413.

// observedIssueCall names an issue without filing one, so the reference
// resolves from the short form rather than from a creation.
func observedIssueCall() ExecutedTool {
	return ExecutedTool{
		Name:    "teable__search_issues",
		Result:  wantIssue233 + "  a harness gap\n\nread 1 open issues",
		Outcome: ToolOutcomeOK,
	}
}

// The reported defect, at the boundary Quail binary-searched on
// sirens-echo#413: a reply at the ceiling lost its reference.
func TestAReplyAtTheCeilingKeepsBothSuffixes(t *testing.T) {
	t.Parallel()
	for _, answerRunes := range []int{1900, 1920, 1921, 1950, 1990, 2500} {
		answer := strings.Repeat("a", answerRunes)
		sent := AssembleReply(answer, discordReplyLimit, createIssueCall())

		if got := runeLen(sent); got > discordReplyLimit {
			t.Errorf("answer=%d: %d runes, over the %d budget",
				answerRunes, got, discordReplyLimit)
		}
		if !strings.Contains(sent, wantIssue233) {
			t.Errorf("answer=%d: the reference was lost", answerRunes)
		}
		if !strings.Contains(sent, "`teable__create_issue`") {
			t.Errorf("answer=%d: the footer was lost", answerRunes)
		}
		if !strings.HasPrefix(sent, "aaa") {
			t.Errorf("answer=%d: the answer was discarded rather than shortened",
				answerRunes)
		}
	}
}

// The send boundary is where the guarantee has to hold, because truncateRunes
// is what reaches Discord. A budget function cannot catch this class.
func TestAnAssembledReplyIsUnchangedByTheTransportCut(t *testing.T) {
	t.Parallel()
	for _, answerRunes := range []int{10, 1000, 1920, 1990, 2500} {
		sent := AssembleReply(
			strings.Repeat("a", answerRunes), discordReplyLimit, createIssueCall(),
		)
		if truncateRunes(sent, discordReplyLimit) != sent {
			t.Errorf("answer=%d: the transport cut still changes the reply, "+
				"so assembly did not own the budget", answerRunes)
		}
	}
}

// A reply that already fits is what it was before assembly owned the budget.
func TestAReplyThatFitsIsAppendedExactlyAsBefore(t *testing.T) {
	t.Parallel()
	answer := "Copper is 2.4c."
	before := AppendToolDisclosure(
		AppendIssueReferences(answer, createIssueCall()), createIssueCall(),
	)
	if got := AssembleReply(answer, discordReplyLimit, createIssueCall()); got != before {
		t.Errorf("assembly changed a reply that already fits:\n%q\nvs\n%q", got, before)
	}
}

// Shortening can remove the short form a reference resolves, and a link to
// something the member cannot see is worse than no link.
func TestATruncatedShortFormIsNotResolvedIntoALink(t *testing.T) {
	t.Parallel()
	answer := strings.Repeat("a", 1970) + " #233 " + strings.Repeat("b", 100)
	sent := AssembleReply(answer, discordReplyLimit, observedIssueCall())

	if got := runeLen(sent); got > discordReplyLimit {
		t.Fatalf("reply is %d runes, over the %d budget", got, discordReplyLimit)
	}
	if strings.Contains(sent, "#233") {
		t.Skip("the short form survived truncation, so this case did not arise")
	}
	if strings.Contains(sent, wantIssue233) {
		t.Errorf("a link resolves a reference the member cannot see:\n%q", sent)
	}
}

// The harder direction, and why one reservation is not enough: cutting a tail
// that carried a suppressed reference makes the suffix grow.
func TestATruncatedTailWithASuppressedRefDoesNotOverflow(t *testing.T) {
	t.Parallel()
	answer := "See #233 for detail. " +
		strings.Repeat("a", 1990) + "\n" + wantIssue233
	if runeLen(answer) <= discordReplyLimit {
		t.Fatalf("the case needs an answer over the ceiling, got %d runes",
			runeLen(answer))
	}
	sent := AssembleReply(answer, discordReplyLimit, observedIssueCall())

	if got := runeLen(sent); got > discordReplyLimit {
		t.Errorf("suffix growth overran the budget: %d runes, limit %d",
			got, discordReplyLimit)
	}
	// The short form survives at the head, so the link it resolves must arrive.
	if !strings.Contains(sent, "#233") {
		t.Fatal("the short form was cut, so this case did not arise")
	}
	if !strings.Contains(sent, referenceHeading) {
		t.Errorf("the reference the truncation revealed was not appended:\n%q", sent)
	}
}

// Whatever converges, terminates. The bound is reached rather than assumed
// unreachable.
func TestAssemblyAtItsPassBoundStillFits(t *testing.T) {
	t.Parallel()
	answer := "See #233 for detail. " +
		strings.Repeat("a", 1990) + "\n" + wantIssue233
	for _, passes := range []int{0, 1, 2, maxAssemblyPasses} {
		sent := assembleReplyWithin(
			answer, discordReplyLimit, passes,
			serviceFacts{executed: []ExecutedTool{observedIssueCall()}},
		)
		if got := runeLen(sent); got > discordReplyLimit {
			t.Errorf("passes=%d: %d runes, over the %d budget",
				passes, got, discordReplyLimit)
		}
	}
}

// The degenerate case, in both regimes. Defined rather than emergent from
// whichever append ran last.
func TestSuffixesLargerThanTheBudgetDropWholeRatherThanCut(t *testing.T) {
	t.Parallel()
	// Room for the reference block but not for the receipt as well. The link
	// outranks the receipt, so the receipt is the one that goes.
	refOnly := AssembleReply("an answer", 70, createIssueCall())
	if got := runeLen(refOnly); got > 70 {
		t.Errorf("limit 70: %d runes, over budget", got)
	}
	if !strings.Contains(refOnly, wantIssue233) {
		t.Errorf("limit 70: the reference lost to the receipt:\n%q", refOnly)
	}
	if strings.Contains(refOnly, "> ") {
		t.Errorf("limit 70: a half-rendered receipt survived:\n%q", refOnly)
	}

	// Not even the block fits whole. A truncated key is worse than none, so the
	// block is dropped entire and the receipt, which does fit, remains.
	receiptOnly := AssembleReply("an answer", 50, createIssueCall())
	if got := runeLen(receiptOnly); got > 50 {
		t.Errorf("limit 50: %d runes, over budget", got)
	}
	if strings.Contains(receiptOnly, "/issues/233") {
		t.Errorf("limit 50: a URL was rendered into a budget it cannot fit:\n%q",
			receiptOnly)
	}
	if strings.TrimSpace(receiptOnly) == "" {
		t.Error("limit 50: a tiny budget produced an empty reply")
	}
}

// A transport with no ceiling keeps everything. The Discord constant used to
// reach the block on every transport, which is a ceiling HTTP does not have.
func TestAnUnboundedTransportKeepsEverySuffix(t *testing.T) {
	t.Parallel()
	answer := "See #233 for detail. " + strings.Repeat("a", 3000)
	sent := AssembleReply(answer, 0, observedIssueCall())

	if !strings.HasPrefix(sent, answer) {
		t.Error("an unbounded transport shortened the answer")
	}
	if !strings.Contains(sent, wantIssue233) {
		t.Errorf("an unbounded transport dropped the reference:\n%q", sent)
	}
	if !strings.Contains(sent, "`teable__search_issues`") {
		t.Error("an unbounded transport dropped the footer")
	}
}

// A third suffix is one entry in the order and does not re-decide the trade
// between the two that exist.
func TestTheSuffixOrderIsThePreferenceOrder(t *testing.T) {
	t.Parallel()
	at := map[string]int{}
	for index, suffix := range serviceSuffixOrder() {
		at[suffix.name] = index
	}
	for _, want := range []string{
		"issue references", "prefill truncation", "tool disclosure",
	} {
		if _, present := at[want]; !present {
			t.Fatalf("the order lost %q: %v", want, at)
		}
	}
	if at["issue references"] > at["tool disclosure"] {
		t.Error("the link must precede the receipt")
	}
	// The last entry is cut first, so a note about missing context outranks the
	// receipt. See sirens-echo#769.
	if at["prefill truncation"] > at["tool disclosure"] {
		t.Error("the truncation note must outrank the receipt")
	}
}
