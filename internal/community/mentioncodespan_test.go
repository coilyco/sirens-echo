package community

import (
	"strings"
	"testing"
)

// URLs and Discord markup are excluded from mention resolution. A backtick span
// is neither, and the disclosure footer is made of them. See sirens-echo#486.

// footerWithToolNames is the reply shape that reaches sendReply on any turn
// that called a tool, which is where mention resolution runs.
func footerWithToolNames() string {
	return AppendToolDisclosure("Trading is busy right now.",
		ExecutedTool{Name: "eco.get_market", Outcome: ToolOutcomeOK},
		ExecutedTool{Name: "eco.find_trade", Outcome: ToolOutcomeEmpty})
}

// Characterization. The fix is a third span exclusion in one function, which is
// worth one deliberate pass rather than a third patch.
func TestAToolNamedMemberRewritesTheFooter(t *testing.T) {
	t.Parallel()
	roster := mentionRoster{}
	roster.add("eco", "999")
	content := footerWithToolNames()
	out, resolved := roster.resolveMentions(content)
	if out == content && len(resolved) == 0 {
		t.Fatal("a display name no longer rewrites a tool name in the receipt, so " +
			"issue 486 is fixed and this test should go")
	}
	// Once per person, so the first line is rewritten and the second is not,
	// which leaves the receipt naming a person and a tool for the same call.
	if strings.Count(out, "eco.") != 1 {
		t.Errorf("expected one surviving tool name beside one rewritten, got:\n%s", out)
	}
}

// The same collision in an ordinary code span, which is the general form.
func TestANameInsideACodeSpanIsRewritten(t *testing.T) {
	t.Parallel()
	roster := mentionRoster{}
	roster.add("eco", "999")
	for _, reply := range []string{
		"run `eco status` to check",
		"```\neco --version\n```",
	} {
		if out, _ := roster.resolveMentions(reply); out == reply {
			t.Errorf("a code span is now left alone, so issue 486 is fixed for %q", reply)
		}
	}
}

// The half that must survive the fix. A name in prose is still a person, and
// emphasis or quotation around it does not change that.
func TestANameInProseStillResolvesBesideCode(t *testing.T) {
	t.Parallel()
	for _, reply := range []string{
		"eco confirmed it",
		"**eco** confirmed it",
		`she said "eco" earlier`,
	} {
		roster := mentionRoster{}
		roster.add("eco", "999")
		out, resolved := roster.resolveMentions(reply)
		if len(resolved) != 1 || !strings.Contains(out, "<@999>") {
			t.Errorf("a name in prose stopped resolving: %q -> %q %v", reply, out, resolved)
		}
	}
}
