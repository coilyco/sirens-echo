package community

import (
	"strings"
	"testing"
)

// A display name is matched anywhere in the reply, and a URL is part of the
// reply. See sirens-echo#465.

// Real reply shapes, each paired with a display name colliding with the link.
// Kept as the corpus the fix is measured against. See sirens-echo#465.
var mentionURLCorpus = map[string]string{
	"eco":    "Open trades are listed at https://eco-app.coilysiren.me/trade",
	"wiki":   "See https://wiki.play.eco/en/index.php?stable=1&title=Housing",
	"issues": "Filed at https://forgejo.coilysiren.me/coilyco-gaming/sirens-echo/issues/7",
	"main":   "Source at https://forgejo.coilysiren.me/x/y/src/branch/main/file.go",
}

// Quail's corpus, now asserting the fixed behaviour rather than the defect.
func TestQuailsCorpusLeavesEveryLinkAlone(t *testing.T) {
	t.Parallel()
	for name, reply := range mentionURLCorpus {
		roster := mentionRoster{}
		roster.add(name, "999")
		out, resolved := roster.resolveMentions(reply)
		if out != reply || len(resolved) != 0 {
			t.Errorf("the name %q rewrote the link it sits inside: %s -> %s", name, reply, out)
		}
	}
}

// The half that must survive any fix. A name outside a link still reaches the
// person, and a name inside a longer word still does not.
func TestANameOutsideALinkStillResolves(t *testing.T) {
	t.Parallel()
	roster := mentionRoster{}
	roster.add("Kai", "111")
	out, resolved := roster.resolveMentions("Kai runs the server.")
	if len(resolved) != 1 || !strings.Contains(out, "<@111>") {
		t.Errorf("a plain name stopped resolving: %q -> %q %v", "Kai runs the server.", out, resolved)
	}
	unchanged := "Kaitlyn runs the server."
	if got, ids := roster.resolveMentions(unchanged); got != unchanged || len(ids) > 0 {
		t.Errorf("a name inside a longer word resolved: %q -> %q %v", unchanged, got, ids)
	}
}
