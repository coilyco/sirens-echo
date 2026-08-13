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

// bb1c1e0 closed this by treating a dotted identifier as a label, which covers
// the receipt without covering code spans. Guarding the outcome, not the route.
func TestAToolNamedMemberLeavesTheFooterAlone(t *testing.T) {
	t.Parallel()
	roster := mentionRoster{}
	roster.add("eco", "999")
	content := footerWithToolNames()
	out, resolved := roster.resolveMentions(content)
	if out != content {
		t.Errorf("a display name rewrote a tool name in the receipt:\n  want %s\n  got  %s",
			content, out)
	}
	if len(resolved) != 0 {
		t.Errorf("a member was reached %v for a tool name that merely contains "+
			"their name", resolved)
	}
	// Both lines, because once-per-person would otherwise leave the receipt
	// naming a person for one call and the tool for the next.
	if strings.Count(out, "eco.") != 2 {
		t.Errorf("expected both tool names intact, got:\n%s", out)
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
