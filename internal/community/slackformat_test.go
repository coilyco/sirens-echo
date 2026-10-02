package community

import (
	"strings"
	"testing"
)

func TestEveryHarnessGlyphHasASlackName(t *testing.T) {
	t.Parallel()
	glyphs := []string{reactionAccepted, reactionTool, reactionFailed, reactionRefused}
	for _, glyph := range replyReactions {
		glyphs = append(glyphs, glyph)
	}
	for _, glyph := range glyphs {
		name, ok := slackReactionName(glyph)
		if !ok || name == "" || strings.ContainsAny(name, ": ") {
			t.Errorf("glyph %q has no usable Slack name (%q)", glyph, name)
		}
	}
}

func TestReplyTextCannotFormAMentionOrALink(t *testing.T) {
	t.Parallel()
	hostile := "<!channel> <!here> <!everyone> <!subteam^S123> <@U0KAI0001> <https://x.test|go> a&b"
	escaped := escapeSlack(hostile)
	if strings.ContainsAny(escaped, "<>") {
		t.Fatalf("escaped text still carries a control character: %q", escaped)
	}
	if !strings.Contains(escaped, "a&amp;b") {
		t.Fatalf("an ampersand must be escaped, got %q", escaped)
	}
}

func TestOnlyTheMentionThatAddressedTheBotIsStripped(t *testing.T) {
	t.Parallel()
	if got := stripLeadingMention("<@U0BOT0001> what is up?", "U0BOT0001"); got != "what is up?" {
		t.Fatalf("got %q", got)
	}
	if got := stripLeadingMention("<@U0BOT0001>: what is up?", "U0BOT0001"); got != "what is up?" {
		t.Fatalf("a colon after the mention must go too, got %q", got)
	}
	other := "<@U0KAI0001> what is up?"
	if got := stripLeadingMention(other, "U0BOT0001"); got != other {
		t.Fatalf("a mention of someone else is the member's own words, got %q", got)
	}
}

func TestSlackMarkupReadsAsPlainText(t *testing.T) {
	t.Parallel()
	resolve := func(id string) string {
		if id == "U0KAI0001" {
			return "Kai"
		}
		return ""
	}
	got := plainSlackText(
		"ask <@U0KAI0001> and <@U0UNKNOWN> in <#C0GENERAL|general>, <!here>: <https://a.test|docs> or <https://b.test> &amp; &lt;done&gt;",
		resolve,
	)
	want := "ask @Kai and @someone in #general, @here: docs (https://a.test) or https://b.test & <done>"
	if got != want {
		t.Fatalf("got  %q\nwant %q", got, want)
	}
}
