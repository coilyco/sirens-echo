package community

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
)

// The adapter composes the issue verbs from record calls and refuses to report
// a write it could not read back. See docs/sirens-echo-issues.md.

// recordedCall is one upstream call the fake session received.
type recordedCall struct {
	name      string
	arguments map[string]any
}

// fakeTrackerSession stands in for the tracker MCP. An unplanned call returns
// an error rather than a silent empty success.
type fakeTrackerSession struct {
	answers map[string][]ToolResult
	calls   []recordedCall
	tools   []ToolDefinition
}

func (f *fakeTrackerSession) Tools() []ToolDefinition        { return f.tools }
func (f *fakeTrackerSession) Grounding() []GroundingDocument { return nil }
func (f *fakeTrackerSession) Guidance() []ServerGuidance     { return nil }
func (f *fakeTrackerSession) Unavailable() []string          { return nil }
func (f *fakeTrackerSession) Close() error                   { return nil }
func (f *fakeTrackerSession) Open(context.Context) (ToolSession, error) {
	return f, nil
}

func (f *fakeTrackerSession) Call(
	_ context.Context, name string, arguments map[string]any,
) (ToolResult, error) {
	f.calls = append(f.calls, recordedCall{name: name, arguments: arguments})
	queued := f.answers[name]
	if len(queued) == 0 {
		return ToolResult{Text: "unplanned call " + name, IsError: true}, nil
	}
	f.answers[name] = queued[1:]
	return queued[0], nil
}

func (f *fakeTrackerSession) callsTo(verb string) []recordedCall {
	name := "teable__" + verb
	matched := make([]recordedCall, 0)
	for _, call := range f.calls {
		if call.name == name {
			matched = append(matched, call)
		}
	}
	return matched
}

// recordPayload renders what the tracker returns for a row.
func recordPayload(id, key, title, state string) string {
	payload, _ := json.Marshal(map[string]any{
		"records": []any{map[string]any{
			"id":   id,
			"name": key,
			"fields": map[string]any{
				"key": key, "title": title, "state": state,
			},
		}},
	})
	return string(payload)
}

func trackerFixture(t *testing.T, answers map[string][]ToolResult) (ToolSession, *fakeTrackerSession) {
	t.Helper()
	inner := &fakeTrackerSession{answers: answers, tools: []ToolDefinition{
		{Name: "teable__create_record", Server: "teable", Original: "create_record"},
		{Name: "teable__list_record", Server: "teable", Original: "list_record"},
		{Name: "eco__get_map", Server: "eco", Original: "get_map"},
	}}
	provider := &TrackerProvider{Inner: inner, Policy: sandboxPolicy()}
	session, err := provider.Open(context.Background())
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	return session, inner
}

// The record verbs are removed rather than left alongside the issue verbs,
// because two ways to file an issue means the unguarded one gets used.
func TestTheRecordVerbsAreNotOffered(t *testing.T) {
	t.Parallel()
	session, _ := trackerFixture(t, map[string][]ToolResult{})

	offered := make(map[string]bool)
	for _, tool := range session.Tools() {
		offered[tool.Name] = true
	}
	for _, hidden := range []string{"teable__create_record", "teable__list_record"} {
		if offered[hidden] {
			t.Errorf("%s reached the model", hidden)
		}
	}
	for _, wanted := range []string{
		"teable__create_issue", "teable__search_issues",
		"teable__comment_issue", "teable__close_issue",
	} {
		if !offered[wanted] {
			t.Errorf("%s was not offered", wanted)
		}
	}
	// A server that is not the tracker is untouched.
	if !offered["eco__get_map"] {
		t.Error("an unrelated server's tool was dropped")
	}
}

// A filing writes the issue row and then the body. See the ordering note in
// docs/sirens-echo-issues.md.
func TestFilingWritesTheIssueThenTheBody(t *testing.T) {
	t.Parallel()
	session, inner := trackerFixture(t, map[string][]ToolResult{
		"teable__create_record": {
			{Text: recordPayload("rec1", "coilyco-gaming/sirens-echo#7", "a gap", "open")},
			{Text: `{"records":[{"id":"cmt1"}]}`},
		},
		"teable__get_record": {
			{Text: recordPayload("rec1", "coilyco-gaming/sirens-echo#7", "a gap", "open")},
		},
	})

	got, err := session.Call(context.Background(), "teable__create_issue", map[string]any{
		"title": "a gap", "body": "what is missing",
	})
	if err != nil {
		t.Fatalf("file: %v", err)
	}
	if got.IsError {
		t.Fatalf("filing reported an error: %q", got.Text)
	}
	if !strings.Contains(got.Text, "coilyco-gaming/sirens-echo#7") {
		t.Errorf("result = %q, want the issue key", got.Text)
	}
	writes := inner.callsTo("create_record")
	if len(writes) != 2 {
		t.Fatalf("create_record calls = %d, want the issue and its body", len(writes))
	}
	if writes[0].arguments["tableId"] != "tblIssues" {
		t.Errorf("first write went to %v", writes[0].arguments["tableId"])
	}
	if writes[1].arguments["tableId"] != "tblComments" {
		t.Errorf("second write went to %v", writes[1].arguments["tableId"])
	}
}

// A 2xx from this API is not proof, so a filing the read-back cannot confirm
// is reported as unconfirmed rather than as a filing.
func TestAnUnconfirmedFilingIsNotReportedAsFiled(t *testing.T) {
	t.Parallel()
	for name, answers := range map[string]map[string][]ToolResult{
		"no record id": {
			"teable__create_record": {{Text: `{"records":[]}`}},
		},
		"read-back finds nothing": {
			"teable__create_record": {
				{Text: recordPayload("rec1", "org/repo#7", "a gap", "open")},
			},
			"teable__get_record": {{Text: `{"records":[]}`}},
		},
		"read-back finds another row": {
			"teable__create_record": {
				{Text: recordPayload("rec1", "org/repo#7", "a gap", "open")},
			},
			"teable__get_record": {
				{Text: recordPayload("rec1", "org/repo#7", "a different title", "open")},
			},
		},
	} {
		session, inner := trackerFixture(t, answers)
		got, err := session.Call(context.Background(), "teable__create_issue", map[string]any{
			"title": "a gap", "body": "what is missing",
		})
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if !got.IsError {
			t.Errorf("%s reported a filing that was not confirmed: %q", name, got.Text)
		}
		// The body is not written against a row that was never confirmed.
		if writes := inner.callsTo("create_record"); len(writes) > 1 {
			t.Errorf("%s wrote a body for an unconfirmed issue", name)
		}
	}
}

// The checks run before the first write, so a refused filing leaves no row
// behind for a later reader to triage.
func TestARefusedFilingWritesNothing(t *testing.T) {
	t.Parallel()
	inner := &fakeTrackerSession{answers: map[string][]ToolResult{}}
	provider := &TrackerProvider{
		Inner:  inner,
		Policy: sandboxPolicy(),
		FilingCheck: func(context.Context, string, string) error {
			return &filingRefused{Stage: filingStageValidity, Verdict: filingVerdictUnclear}
		},
	}
	session, err := provider.Open(context.Background())
	if err != nil {
		t.Fatalf("open: %v", err)
	}

	got, err := session.Call(context.Background(), "teable__create_issue", map[string]any{
		"title": "something is wrong", "body": "please fix",
	})
	if err != nil {
		t.Fatalf("file: %v", err)
	}
	if !got.IsError {
		t.Error("a refused filing was reported as a filing")
	}
	if len(inner.calls) != 0 {
		t.Errorf("a refused filing reached the tracker: %v", inner.calls)
	}
}

// A search reads a bounded window, so the result carries that bound and an
// empty window cannot become "no such issue exists".
func TestASearchCarriesTheBoundItRead(t *testing.T) {
	t.Parallel()
	session, _ := trackerFixture(t, map[string][]ToolResult{
		"teable__list_record": {{Text: recordPayload(
			"rec1", "coilyco-gaming/sirens-echo#7", "the eco map is stale", "open")}},
	})

	got, err := session.Call(context.Background(), "teable__search_issues",
		map[string]any{"query": "eco map"})
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if !strings.Contains(got.Text, "coilyco-gaming/sirens-echo#7") {
		t.Errorf("result = %q, want the matching key", got.Text)
	}
	if !strings.Contains(got.Text, "read 1 open issues") {
		t.Errorf("result = %q, want the bound it read", got.Text)
	}
}

// A close the tracker did not apply is reported as unconfirmed. This API
// returns 200 for writes it did not make.
func TestACloseIsConfirmedBeforeItIsReported(t *testing.T) {
	t.Parallel()
	listed := recordPayload("rec1", "coilyco-gaming/sirens-echo#7", "a gap", "open")
	session, _ := trackerFixture(t, map[string][]ToolResult{
		"teable__list_record": {{Text: listed}},
		"teable__edit_record": {{Text: listed}},
		// The row still reads open, so the close did not apply.
		"teable__get_record": {{Text: listed}},
	})

	got, err := session.Call(context.Background(), "teable__close_issue",
		map[string]any{"key": "coilyco-gaming/sirens-echo#7"})
	if err != nil {
		t.Fatalf("close: %v", err)
	}
	if !got.IsError {
		t.Errorf("an unapplied close was reported as closed: %q", got.Text)
	}
}

// The reply hands a member a key and nothing that resolves it, so the one
// lookup the service does offer has to accept the reference it gave out.
func TestASearchFindsTheKeyTheReplyHandedOut(t *testing.T) {
	t.Parallel()
	session, _ := trackerFixture(t, map[string][]ToolResult{
		"teable__list_record": {{Text: recordPayload(
			"rec1", "coilyco-gaming/sirens-echo#233", "the eco map is stale", "open")}},
	})

	got, err := session.Call(context.Background(), "teable__search_issues",
		map[string]any{"query": "coilyco-gaming/sirens-echo#233"})
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if got.IsError || !strings.Contains(got.Text, "the eco map is stale") {
		t.Errorf("a member quoting the key back reached nothing: %q", got.Text)
	}
}

// A key outside the window read is not found rather than guessed at, and the
// result still carries the bound so the reply cannot overstate it.
func TestAKeyOutsideTheWindowIsNotInvented(t *testing.T) {
	t.Parallel()
	session, _ := trackerFixture(t, map[string][]ToolResult{
		"teable__list_record": {{Text: recordPayload(
			"rec1", "coilyco-gaming/sirens-echo#7", "a different gap", "open")}},
	})

	got, err := session.Call(context.Background(), "teable__search_issues",
		map[string]any{"query": "coilyco-gaming/sirens-echo#999"})
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if strings.Contains(got.Text, "#999") {
		t.Errorf("an unread key was reported as found: %q", got.Text)
	}
	if !strings.Contains(got.Text, "read 1 open issues") {
		t.Errorf("result = %q, want the bound it read", got.Text)
	}
}
