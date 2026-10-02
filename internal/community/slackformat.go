package community

import (
	"regexp"
	"strings"
)

// slackReactionNames maps the closed glyph set to reactions.add names. A test
// holds the line, so a new reaction cannot ship unmapped.
var slackReactionNames = map[string]string{
	reactionAccepted: "eyes",
	reactionTool:     "hammer",
	reactionFailed:   "x",
	reactionRefused:  "no_entry_sign",
	"\U0001F44D":     "thumbsup",
	"\U0001F44E":     "thumbsdown",
	"✅":              "white_check_mark",
	"\U0001F44B":     "wave",
	"❤️":             "heart",
	"\U0001F602":     "joy",
	"\U0001F389":     "tada",
	"\U0001F622":     "cry",
}

// slackReactionName reports the Slack name for a glyph, if the harness has one.
func slackReactionName(glyph string) (string, bool) {
	name, ok := slackReactionNames[glyph]
	return name, ok
}

// escapeSlack makes model text inert: with &, < and > converted, no reply can
// form a mention, a broadcast or a link.
func escapeSlack(text string) string {
	return strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;").Replace(text)
}

var (
	slackUserMention    = regexp.MustCompile(`<@([UW][A-Z0-9]+)(?:\|[^>]*)?>`)
	slackChannelMention = regexp.MustCompile(`<#[CG][A-Z0-9]+\|([^>]*)>`)
	slackSpecial        = regexp.MustCompile(`<!(here|channel|everyone)(?:\|[^>]*)?>`)
	slackLabelledLink   = regexp.MustCompile(`<(https?://[^|>]+)\|([^>]+)>`)
	slackBareLink       = regexp.MustCompile(`<(https?://[^>|]+)>`)
	slackLeadingMention = regexp.MustCompile(`^\s*<@([UW][A-Z0-9]+)(?:\|[^>]*)?>[\s:,]*`)
)

// stripLeadingMention drops the mention that addressed the bot, so the model
// reads the question and not its own name.
func stripLeadingMention(text, botUserID string) string {
	found := slackLeadingMention.FindStringSubmatch(text)
	if found == nil || found[1] != botUserID {
		return text
	}
	return strings.TrimSpace(text[len(found[0]):])
}

// plainSlackText turns Slack's wire markup into the text a member typed.
// resolve names a user id and may return "" for one it cannot name.
func plainSlackText(text string, resolve func(id string) string) string {
	text = slackUserMention.ReplaceAllStringFunc(text, func(match string) string {
		id := slackUserMention.FindStringSubmatch(match)[1]
		if name := resolve(id); name != "" {
			return "@" + name
		}
		return "@someone"
	})
	text = slackChannelMention.ReplaceAllString(text, "#$1")
	text = slackSpecial.ReplaceAllString(text, "@$1")
	text = slackLabelledLink.ReplaceAllString(text, "$2 ($1)")
	text = slackBareLink.ReplaceAllString(text, "$1")
	return strings.NewReplacer("&lt;", "<", "&gt;", ">", "&amp;", "&").Replace(text)
}
