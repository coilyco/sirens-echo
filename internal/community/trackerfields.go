package community

import (
	"strings"
)

// The field values the harness applies to a filed issue, never ones the model
// supplied. See docs/sirens-echo-issues.md.

const (
	// trackerStateOpen is the only state a filed issue may start in.
	trackerStateOpen = "open"
	// trackerStateClosed is what close_issue writes.
	trackerStateClosed = "closed"
	// sandboxAutonomy marks the contents unverified. Not configurable, and set
	// on every row this service files. See docs/sirens-echo-issues.md.
	sandboxAutonomy = "⚠️ SANDBOX ⚠️"
)

// trackerPolicy decides what a filed row carries and where it lands. An
// inactive policy applies nothing. See docs/sirens-echo-issues.md.
type trackerPolicy struct {
	// Tracker is the roster server name the definition selected.
	Tracker string
	// IssuesTable and CommentsTable are both needed: an issue row carries no
	// body, so the body is a linked comment row.
	IssuesTable   string
	CommentsTable string
	// OpenIssuesView bounds what a search reads. The tracker MCP takes no
	// filter argument, so the view is the filter.
	OpenIssuesView string
	// Org and Repo replace the move-to-repo label. The deployment sets them.
	Org  string
	Repo string
	// Priority and Roles fill the tracker's remaining notNull fields, and the
	// model supplies neither. See docs/sirens-echo-issues.md.
	Priority string
	Roles    []string
}

// active reports whether the policy can do anything. Both tables are required,
// because a header with no thread under it is worse than not starting.
func (p trackerPolicy) active() bool {
	return p.Tracker != "" &&
		strings.TrimSpace(p.IssuesTable) != "" &&
		strings.TrimSpace(p.CommentsTable) != ""
}

// issueFields returns the row the harness writes. The title is the only value
// that crosses from the model.
func (p trackerPolicy) issueFields(title string) map[string]any {
	fields := map[string]any{
		"title":    title,
		"state":    trackerStateOpen,
		"autonomy": sandboxAutonomy,
	}
	// Independently optional: an unset knob leaves the tracker's own default
	// rather than writing an empty select.
	for key, value := range map[string]string{
		"org":      strings.TrimSpace(p.Org),
		"repo":     strings.TrimSpace(p.Repo),
		"priority": strings.TrimSpace(p.Priority),
	} {
		if value != "" {
			fields[key] = value
		}
	}
	if roles := trimmedValues(p.Roles); len(roles) > 0 {
		fields["roles"] = roles
	}
	return fields
}

// commaValues reads a list-shaped environment variable.
func commaValues(raw string) []string {
	return trimmedValues(strings.Split(raw, ","))
}

// trimmedValues drops blanks, so a trailing comma configures nothing rather
// than an empty choice.
func trimmedValues(values []string) []string {
	kept := make([]string, 0, len(values))
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			kept = append(kept, trimmed)
		}
	}
	return kept
}
