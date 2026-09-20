package community

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
)

// The grooming verbs edit existing rows and refuse to report a write the
// tracker did not store. See docs/sirens-echo-issues.md.

const groomKey = "coilyco-gaming/sirens-echo#7"

func groomPolicyFixture() trackerPolicy {
	policy := sandboxPolicy()
	policy.Groom = true
	return policy
}

func groomFixture(t *testing.T, answers map[string][]ToolResult) (ToolSession, *fakeTrackerSession) {
	t.Helper()
	inner := &fakeTrackerSession{answers: answers, tools: []ToolDefinition{
		{Name: "teable__create_record", Server: "teable", Original: "create_record"},
		{Name: "teable__edit_record", Server: "teable", Original: "edit_record"},
		{Name: "teable__list_record", Server: "teable", Original: "list_record"},
		{Name: "teable__get_record", Server: "teable", Original: "get_record"},
	}}
	provider := &TrackerProvider{Inner: inner, Policy: groomPolicyFixture()}
	session, err := provider.Open(context.Background())
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	return session, inner
}

// issueRow renders what the tracker returns for one issue row.
func issueRow(fields map[string]any) ToolResult {
	row := map[string]any{"key": groomKey, "title": "a record"}
	for name, value := range fields {
		row[name] = value
	}
	payload, _ := json.Marshal(map[string]any{
		"records": []any{map[string]any{"id": "rec1", "name": groomKey, "fields": row}},
	})
	return ToolResult{Text: string(payload)}
}

func commentRow(kind, body string) ToolResult {
	payload, _ := json.Marshal(map[string]any{
		"records": []any{map[string]any{
			"id": "c", "name": "c", "fields": map[string]any{"kind": kind, "body": body},
		}},
	})
	return ToolResult{Text: string(payload)}
}

func groomCall(t *testing.T, session ToolSession, verb string, arguments map[string]any) ToolResult {
	t.Helper()
	got, err := session.Call(context.Background(), "teable__"+verb, arguments)
	if err != nil {
		t.Fatalf("%s: %v", verb, err)
	}
	return got
}

// A grooming definition is offered no filing verb, and the record verbs stay
// hidden exactly as they do for a filing one.
func TestGroomingOffersEditingAndNotFiling(t *testing.T) {
	t.Parallel()
	session, _ := groomFixture(t, map[string][]ToolResult{})

	offered := make(map[string]bool)
	for _, tool := range session.Tools() {
		offered[tool.Name] = true
	}
	for _, wanted := range []string{
		"teable__search_issues", "teable__read_issue", "teable__update_issue",
		"teable__comment_issue", "teable__close_issue",
	} {
		if !offered[wanted] {
			t.Errorf("%s was not offered", wanted)
		}
	}
	for _, hidden := range []string{
		"teable__create_issue", "teable__create_record", "teable__edit_record",
	} {
		if offered[hidden] {
			t.Errorf("%s reached the model", hidden)
		}
	}
}

// Not being offered a tool does not stop a model naming it.
func TestFilingIsRefusedWhenGrooming(t *testing.T) {
	t.Parallel()
	session, inner := groomFixture(t, map[string][]ToolResult{})

	got := groomCall(t, session, "create_issue", map[string]any{"title": "t", "body": "b"})
	if !got.IsError {
		t.Errorf("a filing went through on a grooming definition: %q", got.Text)
	}
	if len(inner.calls) != 0 {
		t.Errorf("the tracker was called %d times for a refused filing", len(inner.calls))
	}
}

// A filing definition is offered neither, so a name it invents falls through to
// the server instead of running an edit.
func TestAFilingDefinitionCannotEditAnIssue(t *testing.T) {
	t.Parallel()
	session, inner := trackerFixture(t, map[string][]ToolResult{})

	got := groomCall(t, session, "update_issue", map[string]any{"key": groomKey, "priority": "P0"})
	if !got.IsError {
		t.Errorf("update_issue ran on a filing definition: %q", got.Text)
	}
	if len(inner.callsTo("edit_record")) != 0 {
		t.Error("an edit reached the tracker from a filing definition")
	}
}

func listedWindow() ToolResult {
	return issueRow(map[string]any{"state": "open"})
}

// The write is read back and only a stored value is reported. The edit carries
// the named fields and nothing the model added.
func TestAnUpdateIsReportedOnlyOnceStored(t *testing.T) {
	t.Parallel()
	session, inner := groomFixture(t, map[string][]ToolResult{
		"teable__list_record": {listedWindow()},
		"teable__get_record": {
			issueRow(map[string]any{"priority": "P3", "state": "open"}),
			issueRow(map[string]any{"priority": "P1", "state": "open"}),
		},
		"teable__edit_record": {{Text: "{}"}},
	})

	got := groomCall(t, session, "update_issue", map[string]any{
		"key": groomKey, "priority": "P1",
		// Neither is a groomable field, so neither may reach the write.
		"state": "closed", "org": "elsewhere",
	})
	if got.IsError || !strings.Contains(got.Text, "priority = P1") {
		t.Fatalf("a stored change was not reported: %q", got.Text)
	}
	edits := inner.callsTo("edit_record")
	if len(edits) != 1 {
		t.Fatalf("edit calls = %d, want 1", len(edits))
	}
	written, _ := json.Marshal(edits[0].arguments["record"])
	if string(written) != `{"fields":{"priority":"P1"}}` {
		t.Errorf("edit body = %s, want only the named groomable field", written)
	}
	if _, typecast := edits[0].arguments["typecast"]; typecast {
		t.Error("typecast was sent, which would let an unknown select value create a choice")
	}
}

// This API answers 200 to writes it did not make.
func TestAnUnstoredChangeIsNotReportedDone(t *testing.T) {
	t.Parallel()
	session, _ := groomFixture(t, map[string][]ToolResult{
		"teable__list_record": {listedWindow()},
		"teable__get_record": {
			issueRow(map[string]any{"priority": "P3"}),
			// Read back unchanged.
			issueRow(map[string]any{"priority": "P3"}),
		},
		"teable__edit_record": {{Text: "{}"}},
	})

	got := groomCall(t, session, "update_issue", map[string]any{"key": groomKey, "priority": "P1"})
	if !got.IsError || !strings.Contains(got.Text, "unconfirmed") {
		t.Errorf("an unstored change was reported as done: %q", got.Text)
	}
}

// One field applying does not vouch for another.
func TestAPartialUpdateNamesWhichFieldMissed(t *testing.T) {
	t.Parallel()
	session, _ := groomFixture(t, map[string][]ToolResult{
		"teable__list_record": {listedWindow()},
		"teable__get_record": {
			issueRow(map[string]any{"priority": "P3", "status": "todo"}),
			issueRow(map[string]any{"priority": "P1", "status": "todo"}),
		},
		"teable__edit_record": {{Text: "{}"}},
	})

	got := groomCall(t, session, "update_issue",
		map[string]any{"key": groomKey, "priority": "P1", "status": "done"})
	if !got.IsError || !strings.Contains(got.Text, "status") ||
		!strings.Contains(got.Text, "Applied: priority = P1") {
		t.Errorf("result = %q, want status named as missed and priority as applied", got.Text)
	}
}

// A row the harness marked unverified is not edited by the seat that grooms.
func TestAnUnverifiedRowIsNotEdited(t *testing.T) {
	t.Parallel()
	session, inner := groomFixture(t, map[string][]ToolResult{
		"teable__list_record": {listedWindow()},
		"teable__get_record":  {issueRow(map[string]any{"autonomy": sandboxAutonomy})},
	})

	got := groomCall(t, session, "update_issue", map[string]any{"key": groomKey, "priority": "P0"})
	if !got.IsError || !strings.Contains(got.Text, "left for a human") {
		t.Errorf("result = %q, want the row left for a human", got.Text)
	}
	if len(inner.callsTo("edit_record")) != 0 {
		t.Error("an unverified row was edited")
	}
}

// The marker is the runtime's to set, so writing it is refused before any read.
func TestTheUnverifiedMarkerCannotBeWritten(t *testing.T) {
	t.Parallel()
	session, inner := groomFixture(t, map[string][]ToolResult{})

	got := groomCall(t, session, "update_issue",
		map[string]any{"key": groomKey, "autonomy": sandboxAutonomy})
	if !got.IsError {
		t.Errorf("the marker was written: %q", got.Text)
	}
	if len(inner.calls) != 0 {
		t.Errorf("the tracker was called %d times before the refusal", len(inner.calls))
	}
}

func TestAnUpdateNamingNoFieldChangesNothing(t *testing.T) {
	t.Parallel()
	session, inner := groomFixture(t, map[string][]ToolResult{})

	got := groomCall(t, session, "update_issue", map[string]any{"key": groomKey, "state": "closed"})
	if !got.IsError {
		t.Errorf("an update with no groomable field succeeded: %q", got.Text)
	}
	if len(inner.calls) != 0 {
		t.Errorf("the tracker was called %d times", len(inner.calls))
	}
}

// A select's stored order is the tracker's, so the roles compare as a set.
func TestRolesAreComparedAsASet(t *testing.T) {
	t.Parallel()
	session, _ := groomFixture(t, map[string][]ToolResult{
		"teable__list_record": {listedWindow()},
		"teable__get_record": {
			issueRow(map[string]any{"roles": []any{"platform"}}),
			issueRow(map[string]any{"roles": []any{"science", "platform"}}),
		},
		"teable__edit_record": {{Text: "{}"}},
	})

	got := groomCall(t, session, "update_issue", map[string]any{
		"key": groomKey, "roles": []any{"platform", "science", "platform"},
	})
	if got.IsError {
		t.Errorf("the same roles in another order read as unconfirmed: %q", got.Text)
	}
}

// The thread is bounded, and says so, so a reply cannot claim the whole of it.
func TestReadingAnIssueBoundsItsThread(t *testing.T) {
	t.Parallel()
	total := maxGroomComments + 2
	links := make([]any, 0, total)
	comments := make([]ToolResult, 0, total)
	for i := 0; i < total; i++ {
		links = append(links, map[string]any{"id": fmt.Sprintf("c%d", i)})
		comments = append(comments, commentRow("comment", fmt.Sprintf("body %d", i)))
	}
	session, _ := groomFixture(t, map[string][]ToolResult{
		"teable__list_record": {listedWindow()},
		"teable__get_record": append([]ToolResult{
			issueRow(map[string]any{"priority": "P2", "comments": links}),
		}, comments[total-maxGroomComments:]...),
	})

	got := groomCall(t, session, "read_issue", map[string]any{"key": groomKey})
	if got.IsError {
		t.Fatalf("read: %q", got.Text)
	}
	want := fmt.Sprintf("showing the newest %d of %d", maxGroomComments, total)
	if !strings.Contains(got.Text, want) || !strings.Contains(got.Text, "priority: P2") {
		t.Errorf("result = %q, want the fields and %q", got.Text, want)
	}
	if strings.Contains(got.Text, "body 0") {
		t.Error("a comment older than the bound was returned")
	}
}

// A key outside the window is not found rather than read from another row.
func TestReadingAKeyOutsideTheWindowIsRefused(t *testing.T) {
	t.Parallel()
	session, _ := groomFixture(t, map[string][]ToolResult{
		"teable__list_record": {listedWindow()},
	})

	got := groomCall(t, session, "read_issue", map[string]any{"key": "coilyco-gaming/sirens-echo#999"})
	if !got.IsError {
		t.Errorf("an unread key was read: %q", got.Text)
	}
}

func TestLoadDefinitionRefusesABadTrackerMode(t *testing.T) {
	t.Parallel()
	for _, row := range []struct {
		name, body, mustSay string
	}{
		{"unknown mode", validDefinition + "issue_tracker_mode: file\n", "issue_tracker_mode"},
		{
			"grooming with no tracker",
			strings.Replace(validDefinition, "issue_tracker: forgejo\n", "issue_tracker_mode: groom\n", 1),
			"needs an issue_tracker",
		},
	} {
		t.Run(row.name, func(t *testing.T) {
			t.Parallel()
			_, err := LoadDefinition(writeDefinition(t, row.body))
			if err == nil || !strings.Contains(err.Error(), row.mustSay) {
				t.Errorf("err = %v, want it to say %q", err, row.mustSay)
			}
		})
	}
	if _, err := LoadDefinition(writeDefinition(t, validDefinition+"issue_tracker_mode: groom\n")); err != nil {
		t.Errorf("a grooming definition with a tracker was refused: %v", err)
	}
}
