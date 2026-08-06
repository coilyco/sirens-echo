package community

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"unicode"
)

var (
	channelPattern      = regexp.MustCompile(`#[A-Za-z_][A-Za-z0-9_-]*`)
	discordMention      = regexp.MustCompile(`<[@#][!&]?[0-9]+>`)
	longNumericID       = regexp.MustCompile(`\b[0-9]{16,20}\b`)
	discordMessageLink  = regexp.MustCompile(`https?://(?:www\.)?discord(?:app)?\.com/channels/\S+`)
	claimedAction       = regexp.MustCompile(`(?i)\bI (?:have )?(sent|posted|opened|filed|created|escalated|contacted|checked|changed|updated|pinned|deleted|edited|messaged|closed|commented|labeled)\b`)
	markdownFencePrefix = regexp.MustCompile(`(?s)^\s*` + "```(?:json)?\\s*(.*?)\\s*```" + `\s*$`)
	firstPersonVoice    = regexp.MustCompile(`(?i)(?:^|[^A-Za-z0-9_])(?:i|i['’](?:m|ve|d|ll)|me|my|mine|myself|we|we['’](?:re|ve|d|ll)|us|our|ours|ourselves)(?:$|[^A-Za-z0-9_])`)
	socialOpening       = regexp.MustCompile(`(?i)^\s*(?:hi|hello|hey|greetings|thanks|thank you|sorry|sure|absolutely|of course)\b`)
	personalityPhrase   = regexp.MustCompile(`(?i)\b(?:happy to help|glad to help|let me know|what can I help|how can I help|would you like|hope that helps|here['’]s the thing|no worries|community host|my toolset|my tools)\b`)
)

// Decision is the model's constrained response. The runtime performs any issue
// write and reports it only after Forgejo confirms success.
type Decision struct {
	Reply string      `json:"reply"`
	Issue *IssueDraft `json:"issue"`
}

// IssueDraft is a sanitized request for an ordinary Forgejo issue.
type IssueDraft struct {
	Kind  string `json:"kind"`
	Title string `json:"title"`
	Body  string `json:"body"`
}

// ParseDecision parses the strict model response and applies local bounds.
func ParseDecision(raw string) (Decision, error) {
	candidate := strings.TrimSpace(raw)
	if match := markdownFencePrefix.FindStringSubmatch(candidate); len(match) == 2 {
		candidate = strings.TrimSpace(match[1])
	}
	var decision Decision
	parseErr := json.Unmarshal([]byte(candidate), &decision)
	if parseErr != nil {
		for _, embedded := range embeddedJSONObjects(candidate) {
			var decoded Decision
			if err := json.Unmarshal([]byte(embedded), &decoded); err == nil &&
				strings.TrimSpace(decoded.Reply) != "" {
				decision = decoded
				parseErr = nil
				break
			}
		}
	}
	if parseErr != nil {
		if strings.HasPrefix(candidate, "{") ||
			strings.HasPrefix(candidate, "[") ||
			strings.Contains(candidate, `"reply"`) ||
			strings.Contains(candidate, `"issue"`) {
			return Decision{}, fmt.Errorf("parse model decision: %w", parseErr)
		}
		decision = Decision{Reply: candidate}
	}
	decision.Reply = strings.TrimSpace(decision.Reply)
	if decision.Reply == "" {
		return Decision{}, fmt.Errorf("model decision has empty reply")
	}
	if len([]rune(decision.Reply)) > 1800 {
		return Decision{}, fmt.Errorf("model reply exceeds 1800 characters")
	}
	if decision.Issue != nil {
		draft, err := normalizeIssueDraft(*decision.Issue)
		if err != nil {
			return Decision{}, err
		}
		decision.Issue = &draft
	}
	return decision, nil
}

func embeddedJSONObjects(raw string) []string {
	var objects []string
	start := -1
	depth := 0
	inString := false
	escaped := false
	for index := 0; index < len(raw); index++ {
		current := raw[index]
		if start < 0 {
			if current == '{' {
				start = index
				depth = 1
			}
			continue
		}
		if inString {
			if escaped {
				escaped = false
				continue
			}
			if current == '\\' {
				escaped = true
				continue
			}
			if current == '"' {
				inString = false
			}
			continue
		}
		switch current {
		case '"':
			inString = true
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				objects = append(objects, raw[start:index+1])
				start = -1
			}
		}
	}
	return objects
}

// ValidateGrounding rejects invented channel references and first-person
// action claims that are not supported by a completed tool call.
func ValidateGrounding(decision Decision, suppliedContext string, executed ...ExecutedTool) error {
	allowedChannels := make(map[string]struct{})
	for _, channel := range channelPattern.FindAllString(suppliedContext, -1) {
		allowedChannels[strings.ToLower(channel)] = struct{}{}
	}
	check := decision.Reply
	if decision.Issue != nil {
		check += "\n" + decision.Issue.Title + "\n" + decision.Issue.Body
	}
	for _, channel := range channelPattern.FindAllString(check, -1) {
		if _, ok := allowedChannels[strings.ToLower(channel)]; !ok {
			return fmt.Errorf("model invented channel %s", channel)
		}
	}
	for _, match := range claimedAction.FindAllStringSubmatch(decision.Reply, -1) {
		if len(match) == 2 && !actionClaimSupported(match[1], executed) {
			return fmt.Errorf("model claimed an action the runtime has not performed")
		}
	}
	return nil
}

// ValidateNeutralStyle rejects the model-facing traits that make a service
// reply read as a person or character instead of a direct result.
func ValidateNeutralStyle(decision Decision) error {
	reply := strings.TrimSpace(decision.Reply)
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
		if unicode.Is(unicode.So, current) || unicode.Is(unicode.Sk, current) ||
			current == '\u200d' || current == '\ufe0f' {
			return fmt.Errorf("model reply used an emoji or decorative symbol")
		}
	}
	return nil
}

// ValidateResponseStyle applies the deterministic restrictions promised by the
// selected profile. Structural and grounding checks run separately for all styles.
func ValidateResponseStyle(style string, decision Decision) error {
	if style == "" || style == ResponseStyleNeutral {
		return ValidateNeutralStyle(decision)
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

func normalizeIssueDraft(draft IssueDraft) (IssueDraft, error) {
	draft.Kind = strings.TrimSpace(draft.Kind)
	if draft.Kind != "knowledge-gap" && draft.Kind != "correction" {
		return IssueDraft{}, fmt.Errorf("unsupported issue kind %q", draft.Kind)
	}
	draft.Title = sanitizeIssueText(draft.Title)
	draft.Body = sanitizeIssueText(draft.Body)
	if draft.Title == "" || draft.Body == "" {
		return IssueDraft{}, fmt.Errorf("issue draft requires title and body")
	}
	if strings.ContainsAny(draft.Title, "\r\n") {
		return IssueDraft{}, fmt.Errorf("issue title must be one line")
	}
	if len([]rune(draft.Title)) > 100 {
		return IssueDraft{}, fmt.Errorf("issue title exceeds 100 characters")
	}
	if len([]rune(draft.Body)) > 1200 {
		return IssueDraft{}, fmt.Errorf("issue body exceeds 1200 characters")
	}
	return draft, nil
}

func sanitizeIssueText(value string) string {
	clean := discordMessageLink.ReplaceAllString(value, "[redacted Discord link]")
	clean = discordMention.ReplaceAllString(clean, "[redacted mention]")
	clean = longNumericID.ReplaceAllString(clean, "[redacted identifier]")
	return strings.TrimSpace(clean)
}
