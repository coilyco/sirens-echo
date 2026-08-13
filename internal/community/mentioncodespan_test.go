package community

import (
	"strings"
	"testing"
)

// A code span joins URLs and Discord markup as a run no name is read inside.
// The disclosure footer is made of them. See sirens-echo#486.

// footerWithToolNames is the reply shape that reaches sendReply on any turn
// that called a tool, which is where mention resolution runs.
func footerWithToolNames() string {
	return AppendToolDisclosure("Trading is busy right now.",
		ExecutedTool{Name: "eco.get_market", Outcome: ToolOutcomeOK},
		ExecutedTool{Name: "eco.find_trade", Outcome: ToolOutcomeEmpty})
}

// The receipt is built the way a turn builds it, so the assertion cannot drift
// from the real footer. See sirens-echo#486.
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
	// Both calls keep their tool name, so the receipt does not name a person
	// on one line and the tool on the next.
	if strings.Count(out, "eco.") != 2 {
		t.Errorf("expected both tool names intact, got:\n%s", out)
	}
	if len(resolved) != 0 {
		t.Errorf("a tool name in the receipt reached %v", resolved)
	}
}

// The general form, in an ordinary code span rather than the receipt.
func TestANameInsideACodeSpanIsLeftAlone(t *testing.T) {
	t.Parallel()
	roster := mentionRoster{}
	roster.add("eco", "999")
	for _, reply := range []string{
		"run `eco status` to check",
		"```\neco --version\n```",
	} {
		if out, ids := roster.resolveMentions(reply); out != reply || len(ids) != 0 {
			t.Errorf("a code span was rewritten: %q -> %q %v", reply, out, ids)
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
