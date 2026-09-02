package community

import (
	"context"
	"testing"
)

// The field policy is covered. The line that invokes it was not, so deleting
// the call left every test green. See sirens-echo#208.

// theFiledRow returns the fields the adapter sent for the issue row.
func theFiledRow(t *testing.T, inner *fakeTrackerSession) map[string]any {
	t.Helper()
	writes := inner.callsTo("create_record")
	if len(writes) == 0 {
		t.Fatal("nothing was written to the tracker")
	}
	records, ok := writes[0].arguments["records"].([]any)
	if !ok || len(records) != 1 {
		t.Fatalf("records = %v, want exactly one row", writes[0].arguments["records"])
	}
	row, ok := records[0].(map[string]any)
	if !ok {
		t.Fatalf("row = %v", records[0])
	}
	fields, ok := row["fields"].(map[string]any)
	if !ok {
		t.Fatalf("fields = %v", row["fields"])
	}
	return fields
}

// Every issue this service files is marked unverified, on the write itself
// rather than by a second call that could fail and leave the row unmarked.
func TestTheSandboxMarkerIsOnTheWriteItself(t *testing.T) {
	t.Parallel()
	session, inner := trackerFixture(t, map[string][]ToolResult{
		"teable__create_record": {
			{Text: recordPayload("rec1", "org/repo#7", "a gap", "open")},
			{Text: `{"records":[{"id":"cmt1"}]}`},
		},
		"teable__get_record": {{Text: recordPayload("rec1", "org/repo#7", "a gap", "open")}},
	})

	if _, err := session.Call(context.Background(), "teable__create_issue", map[string]any{
		"title": "a gap", "body": "what is missing",
	}); err != nil {
		t.Fatalf("file: %v", err)
	}
	if got := theFiledRow(t, inner)["autonomy"]; got != sandboxAutonomy {
		t.Errorf("autonomy = %v, want the sandbox marker on the filing call", got)
	}
}

// A model that could name its own destination could route its own issue away
// from the people who read this tracker, so what it supplies is discarded.
func TestAModelCannotChooseWhereItsIssueLands(t *testing.T) {
	t.Parallel()
	session, inner := trackerFixture(t, map[string][]ToolResult{
		"teable__create_record": {
			{Text: recordPayload("rec1", "org/repo#7", "a gap", "open")},
			{Text: `{"records":[{"id":"cmt1"}]}`},
		},
		"teable__get_record": {{Text: recordPayload("rec1", "org/repo#7", "a gap", "open")}},
	})

	if _, err := session.Call(context.Background(), "teable__create_issue", map[string]any{
		"title": "a gap", "body": "what is missing",
		// None of these are arguments the tool declares. A model that sends
		// them anyway must not have them honoured.
		"autonomy": "headless", "repo": "inbox", "org": "coilyco-bridge",
		"priority": "P0", "state": "closed",
	}); err != nil {
		t.Fatalf("file: %v", err)
	}
	fields := theFiledRow(t, inner)
	for key, want := range map[string]any{
		"autonomy": sandboxAutonomy,
		"repo":     "sirens-echo",
		"org":      "coilyco-gaming",
		"priority": "P3",
		"state":    trackerStateOpen,
	} {
		if fields[key] != want {
			t.Errorf("%s = %v, want the harness's own %v", key, fields[key], want)
		}
	}
}
