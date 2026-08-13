package community

import (
	"fmt"
	"regexp"
	"strings"
	"unicode"
)

var (
	channelPattern = regexp.MustCompile(`#[A-Za-z_][A-Za-z0-9_-]*`)
	// An adverb between the pronoun and the verb used to defeat this. See
	// sirens-echo#575.
	claimedAction     = regexp.MustCompile(`(?i)\bI (?:have )?(?:already |just |now |since )?(sent|posted|opened|filed|created|escalated|contacted|checked|changed|updated|pinned|deleted|edited|messaged|closed|commented|labeled)\b`)
	firstPersonVoice  = regexp.MustCompile(`(?i)(?:^|[^A-Za-z0-9_])(?:i|i['’](?:m|ve|d|ll)|me|my|mine|myself|we|we['’](?:re|ve|d|ll)|us|our|ours|ourselves)(?:$|[^A-Za-z0-9_])`)
	socialOpening     = regexp.MustCompile(`(?i)^\s*(?:hi|hello|hey|greetings|thanks|thank you|sorry|sure|absolutely|of course)\b`)
	personalityPhrase = regexp.MustCompile(`(?i)\b(?:happy to help|glad to help|let me know|what can I help|how can I help|would you like|hope that helps|here['’]s the thing|no worries|community host|my toolset|my tools)\b`)
)

// urlSpan matches a bare link. Every check here reads prose, and a link is not
// prose. See docs/sirens-echo-prompt.md.
var urlSpan = regexp.MustCompile(`https?://[^\s<>()\[\]]+`)

// maskURLs hides links from a prose check. The replacement is a plain word, so
// masking cannot join the words on either side into a phrase neither had.
func maskURLs(text string) string {
	return urlSpan.ReplaceAllString(text, " link ")
}

// trackerArtifact names what this runtime creates in the issue tracker. The
// passive form anchors on it so it cannot fire on prose about the game world.
const trackerArtifact = `(?:issues?|corrections?|tickets?|bug report|feature request)`

// passiveActionClaim is the neutral profile's voice for the claim the
// first-person matcher catches. See docs/sirens-echo-grounding.md.
var passiveActionClaim = regexp.MustCompile(
	// The adverb slot is bounded rather than \w+: an open gap would carry "not"
	// past the auxiliary and leave the denial to notAClaim. See sirens-echo#602.
	`(?i)\b` + trackerArtifact + `\b[^.!?\n]{0,60}?\b` +
		`(?:(?:has|have)\s+(?:already\s+|just\s+|now\s+|recently\s+|since\s+)?been|was|were)\s+` +
		`(?:sent|posted|opened|filed|created|escalated|closed|` +
		`commented|updated|labeled|logged|raised|submitted|tracked)\b`,
)

// pastReference places an event before this turn. The check asks whether this
// turn wrote to the tracker, so a dated event is out of scope by construction.
var pastReference = regexp.MustCompile(
	// Only words that cannot mean inside this turn. See sirens-echo#575.
	`(?i)\b(?:yesterday|previously|originally|formerly|ago|before|prior\s+to|` +
		`last\s+(?:week|month|year|night|time|season|wipe|patch|cycle)|` +
		`january|february|march|april|may|june|july|august|september|` +
		`october|november|december|\d{4})\b`,
)

// turnReference anchors a sentence to this exchange, which outranks any past
// word beside it: before your message is not before this turn.
var turnReference = regexp.MustCompile(
	`(?i)\b(?:your\s+message|you\s+asked|this\s+message|this\s+turn|` +
		`this\s+conversation|just\s+now|` +
		// An interval shorter than a turn dates an event inside it. Longer
		// ones stay reportage, so a while ago is untouched. See #601.
		`(?:a\s+few\s+|a\s+|several\s+)?(?:moments?|seconds?)\s+ago)\b`,
)

// subjectlessClaim is the clipped form, a sentence opening on the participle
// with no subject at all. See docs/sirens-echo-grounding.md.
var subjectlessClaim = regexp.MustCompile(
	`(?i)^\s*(?:filed|created|opened|closed|posted|sent|commented on|updated|labeled)\s+` +
		`(?:a|an|the|this|one)\s+(?:new\s+|tracking\s+|correction\s+|follow-up\s+)?` +
		trackerArtifact + `\b`,
)

// continuingWorkClaimPattern is shared verbatim with the
// no-continuing-work-claim case. See docs/sirens-echo-continuing-work.md.
const continuingWorkClaimPattern = `(?i)\b(?:the system|the service|this service|sirens echo|sirens deep)\s+(?:is\s+now|will)\s+(?:continue\s+to\s+|keep\s+)?(?:process|processing|monitor|monitoring|watch|watching|track|tracking|check|checking|notify|notifying|update|updating|alert|search|searching|look\s+up|looking\s+up|query|querying|retrieve|retrieving|fetch|fetching)\b`

// continuingWorkClaim is a promise of work between turns. This runtime holds
// no scheduler, so the reply describes something no code will ever do.
var continuingWorkClaim = regexp.MustCompile(continuingWorkClaimPattern)

// notAClaim disqualifies a sentence that denies, hedges, supposes, asks, or
// credits someone else. None of those assert that this turn did the thing.
var notAClaim = regexp.MustCompile(
	`(?i)(?:\bno\b|\bnot\b|\bnever\b|\bcannot\b|n['’]t\b|\bif\b|\bwhether\b|\bunless\b|` +
		`\bonce\b|\bwhen\b|\bwould\b|\bcould\b|\bshould\b|\bmight\b|\bmay\b|\bby\b)`,
)

// sentenceBreak splits a reply into sentences. Polarity belongs to the clause
// that carries it, so a denial two sentences later must not excuse a claim.
var sentenceBreak = regexp.MustCompile(`[.!?]+`)

// claimsCompletedTrackerAction reports an unattributed assertion that a tracker
// write finished. See docs/sirens-echo-grounding.md for what each gate excludes.
func claimsCompletedTrackerAction(reply string) bool {
	for _, sentence := range sentenceBreak.Split(reply, -1) {
		// A dated event is reportage, unless the date is this exchange.
		dated := pastReference.MatchString(sentence) && !turnReference.MatchString(sentence)
		if notAClaim.MatchString(sentence) || dated {
			continue
		}
		if passiveActionClaim.MatchString(sentence) || subjectlessClaim.MatchString(sentence) {
			return true
		}
	}
	return false
}

// ParseReply bounds the model's plain-text reply. Nothing unwraps a fence or
// JSON: a fence is reply content now, and stripping it would corrupt an answer.
func ParseReply(raw string) (string, error) {
	reply := strings.TrimSpace(raw)
	if reply == "" {
		return "", fmt.Errorf("model reply is empty")
	}
	if len([]rune(reply)) > 1800 {
		return "", fmt.Errorf("model reply exceeds 1800 characters")
	}
	return reply, nil
}

// ValidateNoToolCallMarkup refuses a reply carrying the model's own tool-call
// syntax. See docs/sirens-echo-capability-limits.md for refuse over strip.
func ValidateNoToolCallMarkup(reply string) error {
	// Deliberately not in ParseReply: the evaluation scorer and the repair loop
	// both call that, and this must not reshape what those two measure.
	if containsToolCallMarkup(reply) {
		return fmt.Errorf("model reply carries unparsed tool-call markup")
	}
	return nil
}

// ValidateGrounding rejects invented channel references and first-person
// action claims that are not supported by a completed tool call.
func ValidateGrounding(reply string, suppliedContext string, executed ...ExecutedTool) error {
	masked := maskURLs(reply)
	allowedChannels := make(map[string]struct{})
	for _, channel := range channelPattern.FindAllString(maskURLs(suppliedContext), -1) {
		allowedChannels[strings.ToLower(channel)] = struct{}{}
	}
	for _, channel := range channelPattern.FindAllString(masked, -1) {
		if _, ok := allowedChannels[strings.ToLower(channel)]; !ok {
			return fmt.Errorf("model invented channel %s", channel)
		}
	}
	for _, match := range claimedAction.FindAllStringSubmatch(masked, -1) {
		if len(match) == 2 && !actionClaimSupported(match[1], executed) {
			return fmt.Errorf("model claimed an action the runtime has not performed")
		}
	}
	// The neutral profile forbids first person, so the matcher above can never
	// fire on an Echo reply. This is the same claim in the voice Echo can use.
	if claimsCompletedTrackerAction(masked) && !trackerWasTouched(executed) {
		return fmt.Errorf("model claimed a tracker action the runtime has not performed")
	}
	// No tool call can support this one. A turn ends when the reply is sent, so
	// work continuing past it is never something the runtime went on to do.
	if continuingWorkClaim.MatchString(masked) {
		return fmt.Errorf("model claimed work continuing past the end of this turn")
	}
	return nil
}

// selfClaimVerbs are the writes this runtime can perform, in the active voice a
// reply uses when it names itself as the actor.
const selfClaimVerbs = `(?:filed|created|opened|closed|posted|sent|commented on|updated|labeled|escalated)`

// genericSelfNouns are ways this runtime names itself without its identity. A
// reply this service wrote calling itself "the service" is naming itself.
var genericSelfNouns = []string{"the service", "the harness", "the bot", "the agent"}

// selfReferencePattern alternates the configured identity with the generic
// nouns. A short form of the identity is not derived. See sirens-echo#557.
func selfReferencePattern(identity string) string {
	references := make([]string, 0, len(genericSelfNouns)+1)
	references = append(references, regexp.QuoteMeta(identity))
	for _, noun := range genericSelfNouns {
		references = append(references, regexp.QuoteMeta(noun))
	}
	return strings.Join(references, "|")
}

// ValidateSelfAttributedClaim rejects a reply naming this service as having
// completed a tracker write. See docs/sirens-echo-grounding.md.
func ValidateSelfAttributedClaim(reply string, identity string, executed ...ExecutedTool) error {
	identity = strings.TrimSpace(identity)
	// Without a configured identity there is no name to distinguish the service
	// from a member, and a member filing something is a correct reply.
	if identity == "" || trackerWasTouched(executed) {
		return nil
	}
	// The auxiliary is optional, because the simple past is at least as natural
	// a thing for a model to write as the perfect. See sirens-echo#241.
	claim := regexp.MustCompile(
		`(?i)\b(?:` + selfReferencePattern(identity) + `)` +
			`\s+(?:(?:has|have)\s+)?` + selfClaimVerbs + `\b`,
	)
	for _, sentence := range sentenceBreak.Split(maskURLs(reply), -1) {
		if notAClaim.MatchString(sentence) {
			continue
		}
		if claim.MatchString(sentence) {
			return fmt.Errorf("model claimed a tracker action the runtime has not performed")
		}
	}
	return nil
}

// trackerWasTouched asks only whether the turn reached the issue tracker at
// all. See docs/sirens-echo-grounding.md for why the exact write tool is wrong.
func trackerWasTouched(executed []ExecutedTool) bool {
	for _, tool := range executed {
		if strings.Contains(strings.ToLower(tool.Name), "issue") {
			return true
		}
	}
	return false
}

// Impersonation patterns. Deliberately narrow: naming its own identity and
// saying it is an agent are the honest answers and must stay allowed.
var (
	// humanClaim covers "I am a person" and its plural and contracted forms.
	humanClaim = regexp.MustCompile(`(?i)\b(?:i|we)\s*(?:'m|’m|'re|’re| am| are)\s+(?:a|an|the)?\s*(?:real\s+|actual\s+|genuine\s+)?(?:human(?:\s+being)?s?|person|people|woman|man|guy|girl|lady|dude|folk)\b`)
	// agentDenial covers "I am not a bot", which misleads exactly as much as
	// claiming to be human does.
	agentDenial = regexp.MustCompile(`(?i)\b(?:i|we)\s*(?:'m|’m|'re|’re| am| are)\s+not\s+(?:a|an)?\s*(?:bots?|ai|a\.i\.|robots?|machines?|programs?|scripts?|agents?|llms?|language\s+models?|chat\s?bots?|assistants?)\b`)
)

// ValidateIdentityClaim rejects a reply that claims to be a person, denies
// being an agent, or answers as the configured principal.
func ValidateIdentityClaim(reply string, principal Principal) error {
	if humanClaim.MatchString(reply) {
		return fmt.Errorf("model claimed to be a person")
	}
	if agentDenial.MatchString(reply) {
		return fmt.Errorf("model denied being an agent")
	}
	if !principal.Configured() {
		return nil
	}
	// The handle is deployment-owned, so this matches the one account the
	// prompt already trusts rather than guessing at names.
	claim := regexp.MustCompile(
		`(?i)\b(?:i|this)\s*(?:'m|’m| am| is)\s+@?` + regexp.QuoteMeta(principal.Handle) + `\b`,
	)
	if claim.MatchString(reply) {
		return fmt.Errorf("model answered as the configured principal")
	}
	return nil
}

// ValidateNeutralStyle rejects the model-facing traits that make a service
// reply read as a person or character instead of a direct result.
func ValidateNeutralStyle(reply string) error {
	reply = maskURLs(strings.TrimSpace(reply))
	if socialOpening.MatchString(reply) {
		return fmt.Errorf("model reply used a social opening")
	}
	if firstPersonVoice.MatchString(reply) {
		return fmt.Errorf("model reply used first-person or collective voice")
	}
	if personalityPhrase.MatchString(reply) {
		return fmt.Errorf("model reply used conversational personality framing")
	}
	if strings.ContainsRune(reply, '!') {
		return fmt.Errorf("model reply used an exclamation mark")
	}
	for _, current := range reply {
		// ASCII is never decorative. Sk covers the grave accent and the
		// circumflex, so scanning it would ban code spans and exponents.
		if current < 0x80 {
			continue
		}
		if unicode.Is(unicode.So, current) || unicode.Is(unicode.Sk, current) ||
			current == '\u200d' || current == '\ufe0f' {
			return fmt.Errorf("model reply used an emoji or decorative symbol")
		}
	}
	return nil
}

// ValidateResponseStyle applies the deterministic restrictions promised by the
// selected profile. Structural and grounding checks run separately for all styles.
func ValidateResponseStyle(style string, reply string) error {
	if style == "" || style == ResponseStyleNeutral {
		return ValidateNeutralStyle(reply)
	}
	if style == ResponseStyleSocial {
		return nil
	}
	return fmt.Errorf("unsupported response style %q", style)
}

func actionClaimSupported(verb string, executed []ExecutedTool) bool {
	allowedSuffixes := map[string][]string{
		"checked":   {"__get_issue", "__list_issue", "__list_issue_comment", "__list_issue_label", "__list_repository_label", "__get_eco_server_status"},
		"opened":    {"__create_issue"},
		"filed":     {"__create_issue"},
		"created":   {"__create_issue"},
		"closed":    {"__close_issue"},
		"commented": {"__comment_issue"},
		"changed":   {"__add_issue_label", "__set_issue_label", "__remove_issue_label"},
		"updated":   {"__add_issue_label", "__set_issue_label", "__remove_issue_label"},
		"labeled":   {"__add_issue_label", "__set_issue_label", "__remove_issue_label"},
	}
	for _, suffix := range allowedSuffixes[strings.ToLower(verb)] {
		for _, tool := range executed {
			if strings.HasSuffix(tool.Name, suffix) {
				return true
			}
		}
	}
	return false
}
