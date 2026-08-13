package community

import (
	"regexp"
	"strings"
	"testing"
)

// A display name is matched anywhere in the reply, and a URL is part of the
// reply. See sirens-echo#465.

// mentionURLCorpus is real reply shapes this service emits, paired with a
// display name that collides with a component of the link.
var mentionURLCorpus = map[string]string{
	"eco":    "Open trades are listed at https://eco-app.coilysiren.me/trade",
	"wiki":   "See https://wiki.play.eco/en/index.php?stable=1&title=Housing",
	"issues": "Filed at https://forgejo.coilysiren.me/coilyco-gaming/sirens-echo/issues/7",
	"main":   "Source at https://forgejo.coilysiren.me/x/y/src/branch/main/file.go",
}

// The link must survive byte-identical, and nobody is reached for a name that
// only ever appeared inside it.
func TestANameInsideALinkLeavesTheLinkAlone(t *testing.T) {
	t.Parallel()
	for name, reply := range mentionURLCorpus {
		roster := mentionRoster{}
		roster.add(name, "999")
		out, resolved := roster.resolveMentions(reply)
		if out != reply {
			t.Errorf("the name %q rewrote the link it sits inside:\n  want %s\n  got  %s",
				name, reply, out)
		}
		if len(resolved) != 0 {
			t.Errorf("the name %q reached %v for a link that merely contains it",
				name, resolved)
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

// Both halves in one reply, which is the shape that decides whether the fix
// skipped links or merely stopped resolving names that appear in one.
func TestTheProseNameResolvesAndTheLinkDoesNot(t *testing.T) {
	t.Parallel()
	roster := mentionRoster{}
	roster.add("eco", "999")
	link := "https://eco-app.coilysiren.me/trade"
	out, resolved := roster.resolveMentions("eco posted this at " + link)
	if !strings.Contains(out, link) {
		t.Errorf("the link was rewritten: %q", out)
	}
	if !strings.HasPrefix(out, "<@999> posted") {
		t.Errorf("the prose name did not resolve: %q", out)
	}
	if len(resolved) != 1 {
		t.Errorf("resolved %v, want the one person named in prose", resolved)
	}
}

// Order must not matter, so the same reply with the link first still reaches
// the person named after it.
func TestALinkBeforeTheNameDoesNotConsumeTheResolution(t *testing.T) {
	t.Parallel()
	roster := mentionRoster{}
	roster.add("eco", "999")
	link := "https://eco-app.coilysiren.me/trade"
	out, resolved := roster.resolveMentions("Trades are at " + link + " and eco confirmed it.")
	if !strings.Contains(out, link) {
		t.Errorf("the link was rewritten: %q", out)
	}
	if !strings.HasSuffix(out, "and <@999> confirmed it.") {
		t.Errorf("the name after the link did not resolve: %q", out)
	}
	if len(resolved) != 1 {
		t.Errorf("resolved %v, want the one person named in prose", resolved)
	}
}

// Discord markup is not prose either, and its inner text is an id or an emoji
// name rather than a person. See sirens-echo#479.
var mentionMarkupCorpus = map[string]string{
	"trophy":              "nice work <:trophy:1234567890>",
	"wave":                "she sent <a:wave:1234567891> back",
	"1499488069269590262": "ask in <#1499488069269590262>",
	"1499488069269590263": "that is <@&1499488069269590263> territory",
}

func TestMarkupIsCarriedThroughByteIdentical(t *testing.T) {
	t.Parallel()
	for name, reply := range mentionMarkupCorpus {
		roster := mentionRoster{}
		roster.add(name, "999")
		out, resolved := roster.resolveMentions(reply)
		if out != reply || len(resolved) != 0 {
			t.Errorf("the name %q rewrote markup it sits inside: %s -> %s", name, reply, out)
		}
	}
}

// The bound is where it looks, not whether it looks. A name beside markup is
// still that person.
func TestANameBesideMarkupStillResolves(t *testing.T) {
	t.Parallel()
	roster := mentionRoster{}
	roster.add("trophy", "999")
	out, resolved := roster.resolveMentions("trophy earned <:trophy:1234567890>")
	if len(resolved) != 1 {
		t.Fatalf("the standalone name did not resolve: %q %v", out, resolved)
	}
	if !strings.HasSuffix(out, "<:trophy:1234567890>") {
		t.Errorf("the markup was altered: %q", out)
	}
}

// A code span holds a command, a tool name, or an identifier, so a person's
// name inside one is a collision rather than a reference. See sirens-echo#486.
var mentionCodeCorpus = map[string]string{
	"eco":     "run `eco status` to check",
	"steam":   "the ``steam`` reader is separate",
	"forgejo": "```\nforgejo --version\n```",
	"main":    "on branch `main` only",
}

func TestACodeSpanIsCarriedThroughByteIdentical(t *testing.T) {
	t.Parallel()
	for name, reply := range mentionCodeCorpus {
		roster := mentionRoster{}
		roster.add(name, "999")
		out, resolved := roster.resolveMentions(reply)
		if out != reply {
			t.Errorf("the name %q rewrote the code it sits inside:\n  want %s\n  got  %s",
				name, reply, out)
		}
		if len(resolved) != 0 {
			t.Errorf("the name %q reached %v for a code span that merely contains it",
				name, resolved)
		}
	}
}

// The receipt case lives in mentioncodespan_test.go, where the fixture is
// built by AppendToolDisclosure rather than copied out. See sirens-echo#510.

// The lead stops the class, not the exception list. Each was a separate patch
// and none reaches the span exceptions now. See sirens-echo#494.
func TestTheLeadAloneStopsEveryKnownShape(t *testing.T) {
	t.Parallel()
	lead := regexp.MustCompile(mentionLead + "coilysiren" + `\b`)
	for name, text := range map[string]string{
		"link":            "https://eco-app.coilysiren.me/jobs",
		"schemeless host": "see eco-app.coilysiren.me for jobs",
		"markup":          "<:coilysiren:123>",
		"dotted":          "a.coilysiren.b",
		"path":            "/repos/coilysiren/sirens-echo",
	} {
		if lead.MatchString(text) {
			t.Errorf("%s still matches without any span exception: %q", name, text)
		}
	}
}

// The half that must survive. A name in prose beside a code span is still that
// person, and the span is still left alone.
func TestANameBesideACodeSpanStillResolves(t *testing.T) {
	t.Parallel()
	roster := mentionRoster{}
	roster.add("eco", "999")
	out, resolved := roster.resolveMentions("eco asked about `eco status` today")
	if len(resolved) != 1 {
		t.Fatalf("the prose name did not resolve: %q %v", out, resolved)
	}
	if !strings.HasPrefix(out, "<@999> asked") || !strings.Contains(out, "`eco status`") {
		t.Errorf("the wrong occurrence was rewritten: %q", out)
	}
}

// And the cases a member would expect to reach them still do.
func TestTheLeadKeepsEveryWayAPersonIsNamed(t *testing.T) {
	t.Parallel()
	lead := regexp.MustCompile(mentionLead + "coilysiren" + `\b`)
	for name, text := range map[string]string{
		"start of reply":  "coilysiren asked about it",
		"mid sentence":    "ask coilysiren about it",
		"parenthesised":   "(coilysiren) filed it",
		"quoted":          `"coilysiren" filed it`,
		"after a newline": "the answer is\ncoilysiren knows",
	} {
		if !lead.MatchString(text) {
			t.Errorf("%s no longer names a person: %q", name, text)
		}
	}
}

// The first label of a schemeless hostname is not a person. The dot that
// proves it is a host comes after the hyphenated remainder. sirens-echo#515.
func TestTheFirstLabelOfASchemelessHostIsNotAPerson(t *testing.T) {
	t.Parallel()
	roster := mentionRoster{}
	roster.add("eco", "999")
	for name, reply := range map[string]string{
		"hyphenated first label": "See eco-app.coilysiren.me/jobs",
		"bare first label":       "See eco.coilysiren.me/jobs",
		"deeper host":            "try eco-app.staging.coilysiren.me now",
	} {
		out, resolved := roster.resolveMentions(reply)
		if out != reply || len(resolved) != 0 {
			t.Errorf("%s was rewritten: %q -> %q %v", name, reply, out, resolved)
		}
	}
}

// A hyphen in prose is not a hostname, so the person still resolves.
func TestAHyphenatedPhraseStillNamesAPerson(t *testing.T) {
	t.Parallel()
	roster := mentionRoster{}
	roster.add("Kai", "111")
	for name, reply := range map[string]string{
		"hyphenated adjective": "a Kai-approved change",
		"sentence end":         "ask Kai.",
		"plain":                "Kai runs the server",
	} {
		if _, resolved := roster.resolveMentions(reply); len(resolved) != 1 {
			t.Errorf("%s stopped naming a person: %q", name, reply)
		}
	}
}
