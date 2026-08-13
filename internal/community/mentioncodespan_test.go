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

// Both characterization tests here said to delete them once 486 was fixed. It
// is, so these assert the fixed behaviour over the same corpora.
func TestACodeSpanAndAToolFooterAreLeftAlone(t *testing.T) {
	t.Parallel()
	roster := mentionRoster{}
	roster.add("eco", "999")
	for name, reply := range map[string]string{
		"inline code":  "run `eco status` to check",
		"fenced block": "```\neco --version\n```",
		"branch name":  "on branch `main` only",
		"tool footer":  footerWithToolNames(),
	} {
		out, resolved := roster.resolveMentions(reply)
		if out != reply || len(resolved) != 0 {
			t.Errorf("%s was rewritten: %q -> %q %v", name, reply, out, resolved)
		}
	}
}
