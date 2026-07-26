package community

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
)

var (
	channelPattern      = regexp.MustCompile(`#[A-Za-z0-9_-]+`)
	discordMention      = regexp.MustCompile(`<[@#][!&]?[0-9]+>`)
	longNumericID       = regexp.MustCompile(`\b[0-9]{16,20}\b`)
	discordMessageLink  = regexp.MustCompile(`https?://(?:www\.)?discord(?:app)?\.com/channels/\S+`)
	claimedAction       = regexp.MustCompile(`(?i)\bI (?:have )?(?:sent|posted|opened|filed|created|escalated|contacted|checked|changed|updated|pinned|deleted|edited|messaged)\b`)
	markdownFencePrefix = regexp.MustCompile(`(?s)^\s*` + "```(?:json)?\\s*(.*?)\\s*```" + `\s*$`)
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
	if err := json.Unmarshal([]byte(candidate), &decision); err != nil {
		return Decision{}, fmt.Errorf("parse model decision: %w", err)
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

// ValidateGrounding rejects invented channel references and unsupported
// first-person action claims before anything reaches Discord or Forgejo.
func ValidateGrounding(decision Decision, suppliedContext string) error {
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
	if claimedAction.MatchString(decision.Reply) {
		return fmt.Errorf("model claimed an action the runtime has not performed")
	}
	return nil
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
