package community

import (
	"context"
	"fmt"
	"strings"
)

// The verbs a grooming definition gets in place of filing. Reading and editing
// an existing issue, each write read back. See docs/sirens-echo-issues.md.

const (
	trackerReadTool   = "read_issue"
	trackerUpdateTool = "update_issue"
)

// groomFields are the columns update_issue may change, by the tracker's names.
// State is close_issue's and org and repo are routing, so they are excluded.
var groomFields = []string{"title", "priority", "autonomy", "status", "verdict", "gate", "roles"}

// groomShown is what read_issue prints of a row, in this order.
var groomShown = []string{
	"key", "title", "state", "priority", "autonomy", "status", "verdict", "gate", "roles",
}

func (s *trackerSession) groomTools() []ToolDefinition {
	keyProperty := scratchStringProperty("The issue key, such as coilyco-gaming/sirens-echo#123.")
	update := map[string]any{"key": keyProperty}
	for _, field := range groomFields {
		update[field] = scratchStringProperty("New " + field + ". Leave it out to keep the current value.")
	}
	update["roles"] = map[string]any{
		"type":        "array",
		"items":       map[string]any{"type": "string"},
		"description": "The full list of roles, replacing the current one. Leave it out to keep the current list.",
	}
	return []ToolDefinition{
		s.tool(trackerSearchTool,
			"Search open issues on this service's tracker by words in the title or key. "+
				"Reads a bounded window of the open issues rather than the whole "+
				"tracker, so an empty result means none matched in what it read.",
			scratchObjectSchema(map[string]any{
				"query": scratchStringProperty("Words to match against issue keys and titles."),
			}, []string{"query"}),
		),
		s.tool(trackerReadTool,
			"Read one issue: its fields and the newest comments on its thread. Read an "+
				"issue before changing it.",
			scratchObjectSchema(map[string]any{"key": keyProperty}, []string{"key"}),
		),
		s.tool(trackerUpdateTool,
			"Change fields on one existing issue. Name only the fields to change. The "+
				"result says which changes the tracker stored, and only those are done.",
			scratchObjectSchema(update, []string{"key"}),
		),
		s.tool(trackerCommentTool,
			"Add a comment to an issue already on this service's tracker.",
			scratchObjectSchema(map[string]any{
				"key":  keyProperty,
				"body": scratchStringProperty("The comment text."),
			}, []string{"key", "body"}),
		),
		s.tool(trackerCloseTool,
			"Close an issue on this service's tracker.",
			scratchObjectSchema(map[string]any{"key": keyProperty}, []string{"key"}),
		),
	}
}

// getIssue reads one issue row fresh, so a decision never rests on the window a
// search read earlier.
func (s *trackerSession) getIssue(ctx context.Context, recordID string) (trackerRecord, bool, error) {
	read, err := s.record(ctx, recordGetTool, map[string]any{
		"tableId":      s.policy.IssuesTable,
		"recordId":     recordID,
		"fieldKeyType": "name",
	})
	if err != nil {
		return trackerRecord{}, false, err
	}
	if read.IsError {
		return trackerRecord{}, false, nil
	}
	stored := decodeRecords(read.Text)
	if len(stored) == 0 {
		return trackerRecord{}, false, nil
	}
	return stored[0], true, nil
}

func (s *trackerSession) readIssue(ctx context.Context, arguments map[string]any) (ToolResult, error) {
	key := strings.TrimSpace(scratchStringArg(arguments, "key"))
	if key == "" {
		return scratchRefusal("key is required to read an issue")
	}
	recordID, err := s.resolveKey(ctx, key)
	if err != nil {
		return ToolResult{}, err
	}
	if recordID == "" {
		return scratchRefusal("no open issue in the window read has the key %s", key)
	}
	row, found, err := s.getIssue(ctx, recordID)
	if err != nil {
		return ToolResult{}, err
	}
	if !found {
		return scratchRefusal("the tracker returned no row for %s, so it is unread", key)
	}
	var out strings.Builder
	for _, name := range groomShown {
		if text := displayValue(row.Fields[name]); text != "" {
			fmt.Fprintf(&out, "%s: %s\n", name, text)
		}
	}
	ids := linkedIDs(row.Fields["comments"])
	shown := ids
	if len(shown) > maxGroomComments {
		shown = shown[len(shown)-maxGroomComments:]
	}
	fmt.Fprintf(&out, "\ncomments: showing the newest %d of %d, oldest first\n", len(shown), len(ids))
	for _, id := range shown {
		fmt.Fprintf(&out, "\n%s", s.commentText(ctx, id))
	}
	return ToolResult{Text: out.String(), Detail: key}, nil
}

// commentText renders one comment row, or says it could not be read rather
// than leaving a silent gap in the thread.
func (s *trackerSession) commentText(ctx context.Context, recordID string) string {
	read, err := s.record(ctx, recordGetTool, map[string]any{
		"tableId":      s.policy.CommentsTable,
		"recordId":     recordID,
		"fieldKeyType": "name",
	})
	if err != nil || read.IsError {
		return "[a comment could not be read]\n"
	}
	stored := decodeRecords(read.Text)
	if len(stored) == 0 {
		return "[a comment could not be read]\n"
	}
	kind := stored[0].field("kind")
	if kind == "" {
		kind = "comment"
	}
	return fmt.Sprintf("[%s]\n%s\n", kind, strings.TrimSpace(stored[0].field("body")))
}

// updateIssue edits one row, guarding the unverified marker before the write and
// reading the result back after. See docs/sirens-echo-issues.md.
func (s *trackerSession) updateIssue(ctx context.Context, arguments map[string]any) (ToolResult, error) {
	key := strings.TrimSpace(scratchStringArg(arguments, "key"))
	if key == "" {
		return scratchRefusal("key is required to update an issue")
	}
	changes := requestedChanges(arguments)
	if len(changes) == 0 {
		return scratchRefusal("name at least one field to change")
	}
	if changes["autonomy"] == sandboxAutonomy {
		return scratchRefusal("the unverified marker is set by the runtime and is not a value to write")
	}
	recordID, err := s.resolveKey(ctx, key)
	if err != nil {
		return ToolResult{}, err
	}
	if recordID == "" {
		return scratchRefusal("no open issue in the window read has the key %s", key)
	}
	current, found, err := s.getIssue(ctx, recordID)
	if err != nil {
		return ToolResult{}, err
	}
	if !found {
		return scratchRefusal("the tracker returned no row for %s, so nothing was changed", key)
	}
	// Fresh, not the window's copy: a human may have cleared or set it since.
	if current.field("autonomy") == sandboxAutonomy {
		return scratchRefusal(
			"%s carries the unverified marker, so its fields are left for a human to change. "+
				"Reading it and commenting on it stay open.", key)
	}
	edited, err := s.record(ctx, recordEditTool, map[string]any{
		"tableId":      s.policy.IssuesTable,
		"recordId":     recordID,
		"fieldKeyType": "name",
		"record":       map[string]any{"fields": changes},
	})
	if err != nil {
		return ToolResult{}, err
	}
	if edited.IsError {
		return edited, nil
	}
	stored, found, err := s.getIssue(ctx, recordID)
	if err != nil {
		return ToolResult{}, err
	}
	if !found {
		return scratchRefusal("the tracker accepted the edit but reading %s back failed, so it is unconfirmed", key)
	}
	var applied, missed []string
	for _, name := range groomFields {
		want, asked := changes[name]
		if !asked {
			continue
		}
		if storedMatches(stored.Fields[name], want) {
			applied = append(applied, name+" = "+displayValue(want))
		} else {
			missed = append(missed, name)
		}
	}
	if len(missed) > 0 {
		return scratchRefusal(
			"the tracker accepted the edit but %s still read back unchanged on %s, so "+
				"that change is unconfirmed: do not report it as done. Applied: %s",
			strings.Join(missed, ", "), key, orNone(applied))
	}
	return ToolResult{Text: "updated " + key + "\n" + strings.Join(applied, "\n"), Detail: key}, nil
}

// requestedChanges keeps the groomable fields the model named and drops the
// rest, so an argument that is not one of them cannot reach the write.
func requestedChanges(arguments map[string]any) map[string]any {
	changes := make(map[string]any)
	for _, name := range groomFields {
		if name == "roles" {
			if roles := uniqueValues(valueStrings(arguments["roles"])); len(roles) > 0 {
				changes["roles"] = roles
			}
			continue
		}
		if value := strings.TrimSpace(scratchStringArg(arguments, name)); value != "" {
			changes[name] = value
		}
	}
	return changes
}

// storedMatches compares what was asked with what the tracker now holds. A
// list compares as a set, since a select's stored order is the tracker's.
func storedMatches(actual, want any) bool {
	switch wanted := want.(type) {
	case []string:
		held := uniqueValues(valueStrings(actual))
		if len(held) != len(wanted) {
			return false
		}
		set := make(map[string]bool, len(held))
		for _, value := range held {
			set[value] = true
		}
		for _, value := range wanted {
			if !set[value] {
				return false
			}
		}
		return true
	case string:
		text, _ := actual.(string)
		return strings.TrimSpace(text) == wanted
	}
	return false
}

// valueStrings reads a cell that is a string, or a list of them.
func valueStrings(value any) []string {
	switch typed := value.(type) {
	case string:
		return trimmedValues([]string{typed})
	case []string:
		return trimmedValues(typed)
	case []any:
		out := make([]string, 0, len(typed))
		for _, item := range typed {
			if text, ok := item.(string); ok {
				out = append(out, text)
			}
		}
		return trimmedValues(out)
	}
	return nil
}

func uniqueValues(values []string) []string {
	seen := make(map[string]bool, len(values))
	out := make([]string, 0, len(values))
	for _, value := range values {
		if !seen[value] {
			seen[value] = true
			out = append(out, value)
		}
	}
	return out
}

func displayValue(value any) string {
	if list := valueStrings(value); list != nil {
		return strings.Join(list, ", ")
	}
	return ""
}

// linkedIDs reads the record ids a link cell carries.
func linkedIDs(value any) []string {
	items, _ := value.([]any)
	ids := make([]string, 0, len(items))
	for _, item := range items {
		if link, ok := item.(map[string]any); ok {
			if id, _ := link["id"].(string); id != "" {
				ids = append(ids, id)
			}
		}
	}
	return ids
}

func orNone(values []string) string {
	if len(values) == 0 {
		return "none"
	}
	return strings.Join(values, "; ")
}
