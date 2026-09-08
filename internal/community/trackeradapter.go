package community

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
)

// The issue tracker as the model sees it, composed from the record verbs its
// MCP publishes. See docs/sirens-echo-issues.md.

// The verbs the model is offered. They carry the roster server's own name, so
// a tool still reads as belonging to the tracker it reaches.
const (
	trackerSearchTool  = "search_issues"
	trackerFileTool    = "create_issue"
	trackerCommentTool = "comment_issue"
	trackerCloseTool   = "close_issue"
)

// The record verbs this adapter calls, by the guardfile's own names.
const (
	recordCreateTool = "create_record"
	recordEditTool   = "edit_record"
	recordListTool   = "list_record"
	recordGetTool    = "get_record"
)

// TrackerProvider hides the tracker server's record verbs and offers the issue
// verbs instead. An inactive policy wraps nothing.
type TrackerProvider struct {
	Inner  ToolProvider
	Policy trackerPolicy
	// FilingCheck refuses a ticket with nothing to act on. Nil files
	// everything. See docs/sirens-echo-issues.md.
	FilingCheck func(ctx context.Context, title, body string) error
}

func (p *TrackerProvider) Open(ctx context.Context) (ToolSession, error) {
	inner, err := p.Inner.Open(ctx)
	if err != nil {
		return nil, err
	}
	if !p.Policy.active() {
		return inner, nil
	}
	return &trackerSession{
		inner:       inner,
		policy:      p.Policy,
		filingCheck: p.FilingCheck,
	}, nil
}

type trackerSession struct {
	inner       ToolSession
	policy      trackerPolicy
	filingCheck func(ctx context.Context, title, body string) error
}

func (s *trackerSession) Close() error                   { return s.inner.Close() }
func (s *trackerSession) Grounding() []GroundingDocument { return s.inner.Grounding() }
func (s *trackerSession) Guidance() []ServerGuidance     { return s.inner.Guidance() }
func (s *trackerSession) Unavailable() []string          { return s.inner.Unavailable() }

// Tools swaps the record verbs for the issue verbs. Removed rather than left
// alongside: two ways to file means the unguarded one gets used.
func (s *trackerSession) Tools() []ToolDefinition {
	kept := make([]ToolDefinition, 0)
	for _, tool := range s.inner.Tools() {
		if tool.Server == s.policy.Tracker {
			continue
		}
		kept = append(kept, tool)
	}
	return append(kept, s.issueTools()...)
}

func (s *trackerSession) issueTools() []ToolDefinition {
	return []ToolDefinition{
		s.tool(trackerSearchTool,
			"Search open issues on this service's tracker by words in the title. "+
				"Reads a bounded window of the open issues rather than the whole "+
				"tracker, so an empty result means none matched in what it read.",
			scratchObjectSchema(map[string]any{
				"query": scratchStringProperty("Words to match against issue titles."),
			}, []string{"query"}),
		),
		s.tool(trackerFileTool,
			"File one issue on this service's tracker. Supply the title and the body "+
				"only: where the issue lands, what it is prioritised as, and that its "+
				"contents are unverified are all set by the runtime and are not "+
				"arguments. The result carries the issue's key, which is the reference "+
				"to give the member.",
			scratchObjectSchema(map[string]any{
				"title": scratchStringProperty("One short line naming the gap, never the member."),
				"body":  scratchStringProperty("What is missing or wrong, and what would resolve it."),
			}, []string{"title", "body"}),
		),
		s.tool(trackerCommentTool,
			"Add a comment to an issue already on this service's tracker.",
			scratchObjectSchema(map[string]any{
				"key":  scratchStringProperty("The issue key, such as coilyco-gaming/sirens-echo#123."),
				"body": scratchStringProperty("The comment text."),
			}, []string{"key", "body"}),
		),
		s.tool(trackerCloseTool,
			"Close an issue on this service's tracker.",
			scratchObjectSchema(map[string]any{
				"key": scratchStringProperty("The issue key, such as coilyco-gaming/sirens-echo#123."),
			}, []string{"key"}),
		),
	}
}

func (s *trackerSession) tool(name, description string, schema map[string]any) ToolDefinition {
	proxied, err := proxyToolName(s.policy.Tracker, name)
	if err != nil {
		// Unreachable for a tracker that loaded: the roster validated the
		// server name against this same pattern.
		proxied = name
	}
	return ToolDefinition{
		Name:        proxied,
		Server:      s.policy.Tracker,
		Original:    name,
		Description: description,
		InputSchema: schema,
	}
}

// issueToolName is the proxied name a synthesized verb answers to.
func (s *trackerSession) issueToolName(verb string) string {
	if proxied, err := proxyToolName(s.policy.Tracker, verb); err == nil {
		return proxied
	}
	return verb
}

func (s *trackerSession) Call(
	ctx context.Context,
	name string,
	arguments map[string]any,
) (ToolResult, error) {
	switch name {
	case s.issueToolName(trackerSearchTool):
		return s.searchIssues(ctx, arguments)
	case s.issueToolName(trackerFileTool):
		return s.fileIssue(ctx, arguments)
	case s.issueToolName(trackerCommentTool):
		return s.commentIssue(ctx, arguments)
	case s.issueToolName(trackerCloseTool):
		return s.closeIssue(ctx, arguments)
	}
	return s.inner.Call(ctx, name, arguments)
}

// record calls one of the tracker MCP's own verbs, the adapter's single
// crossing of the guardfile bound.
func (s *trackerSession) record(
	ctx context.Context,
	verb string,
	arguments map[string]any,
) (ToolResult, error) {
	name, err := proxyToolName(s.policy.Tracker, verb)
	if err != nil {
		return ToolResult{}, fmt.Errorf("tracker verb %q: %w", verb, err)
	}
	return s.inner.Call(ctx, name, arguments)
}

// fileIssue writes the issue row, confirms it, then writes the body as its
// first comment. Order matters: see docs/sirens-echo-issues.md.
func (s *trackerSession) fileIssue(
	ctx context.Context,
	arguments map[string]any,
) (ToolResult, error) {
	title := strings.TrimSpace(scratchStringArg(arguments, "title"))
	body := strings.TrimSpace(scratchStringArg(arguments, "body"))
	if title == "" || body == "" {
		return scratchRefusal("title and body are both required to file an issue")
	}
	// Before the first write, so a refused filing leaves no row behind.
	if s.filingCheck != nil {
		if refused := s.filingCheck(ctx, title, body); refused != nil {
			return ToolResult{Text: refused.Error(), IsError: true}, nil
		}
	}
	created, err := s.record(ctx, recordCreateTool, map[string]any{
		"tableId":      s.policy.IssuesTable,
		"fieldKeyType": "name",
		"typecast":     true,
		"records": []any{
			map[string]any{"fields": s.policy.issueFields(title)},
		},
	})
	if err != nil {
		return ToolResult{}, err
	}
	if created.IsError {
		return created, nil
	}
	recordID, key := firstRecordIdentity(created.Text)
	if recordID == "" {
		// A 2xx from this API is not proof. See the read-back section of
		// docs/sirens-echo-issues.md.
		return scratchRefusal(
			"the tracker accepted the issue but returned no record id, so the " +
				"filing is unconfirmed: do not tell the member an issue was filed")
	}
	// The read-back a declarative guardfile cannot do.
	confirmedKey, err := s.confirmIssue(ctx, recordID, title)
	if err != nil {
		return ToolResult{}, err
	}
	if confirmedKey == "" {
		return scratchRefusal(
			"the tracker accepted the issue but reading it back did not find it, " +
				"so the filing is unconfirmed: do not tell the member an issue was filed")
	}
	if key == "" {
		key = confirmedKey
	}
	if commentErr := s.writeComment(ctx, recordID, body); commentErr != nil {
		return ToolResult{}, commentErr
	}
	return ToolResult{
		Text: fmt.Sprintf("filed %s\n%s", key, title),
		// The key came back from the tracker, so it is safe to display where
		// argument text would not be.
		Detail: key,
	}, nil
}

// confirmIssue returns the row's key only when the row is the one just
// written. A title that does not match is a different row.
func (s *trackerSession) confirmIssue(
	ctx context.Context,
	recordID string,
	title string,
) (string, error) {
	read, err := s.record(ctx, recordGetTool, map[string]any{
		"tableId":      s.policy.IssuesTable,
		"recordId":     recordID,
		"fieldKeyType": "name",
	})
	if err != nil {
		return "", err
	}
	if read.IsError {
		return "", nil
	}
	stored := decodeRecords(read.Text)
	if len(stored) == 0 {
		return "", nil
	}
	if strings.TrimSpace(stored[0].field("title")) != title {
		return "", nil
	}
	return stored[0].key(), nil
}

// writeComment files the body against the issue row. The link takes a record
// id: the key is a formula the API will not resolve back to a row.
func (s *trackerSession) writeComment(
	ctx context.Context,
	recordID string,
	body string,
) error {
	written, err := s.record(ctx, recordCreateTool, map[string]any{
		"tableId":      s.policy.CommentsTable,
		"fieldKeyType": "name",
		"typecast":     true,
		"records": []any{
			map[string]any{"fields": map[string]any{
				"issue": []any{map[string]any{"id": recordID}},
				"body":  body,
			}},
		},
	})
	if err != nil {
		return err
	}
	if written.IsError {
		return fmt.Errorf("tracker refused the issue body")
	}
	return nil
}

func (s *trackerSession) commentIssue(
	ctx context.Context,
	arguments map[string]any,
) (ToolResult, error) {
	key := strings.TrimSpace(scratchStringArg(arguments, "key"))
	body := strings.TrimSpace(scratchStringArg(arguments, "body"))
	if key == "" || body == "" {
		return scratchRefusal("key and body are both required to comment")
	}
	recordID, err := s.resolveKey(ctx, key)
	if err != nil {
		return ToolResult{}, err
	}
	if recordID == "" {
		return scratchRefusal("no open issue in the window read has the key %s", key)
	}
	if commentErr := s.writeComment(ctx, recordID, body); commentErr != nil {
		return ToolResult{}, commentErr
	}
	return ToolResult{Text: "commented on " + key, Detail: key}, nil
}

func (s *trackerSession) closeIssue(
	ctx context.Context,
	arguments map[string]any,
) (ToolResult, error) {
	key := strings.TrimSpace(scratchStringArg(arguments, "key"))
	if key == "" {
		return scratchRefusal("key is required to close an issue")
	}
	recordID, err := s.resolveKey(ctx, key)
	if err != nil {
		return ToolResult{}, err
	}
	if recordID == "" {
		return scratchRefusal("no open issue in the window read has the key %s", key)
	}
	edited, err := s.record(ctx, recordEditTool, map[string]any{
		"tableId":      s.policy.IssuesTable,
		"recordId":     recordID,
		"fieldKeyType": "name",
		"typecast":     true,
		"record": map[string]any{
			"fields": map[string]any{"state": trackerStateClosed},
		},
	})
	if err != nil {
		return ToolResult{}, err
	}
	if edited.IsError {
		return edited, nil
	}
	// The same read-back the filing path does.
	read, err := s.record(ctx, recordGetTool, map[string]any{
		"tableId":      s.policy.IssuesTable,
		"recordId":     recordID,
		"fieldKeyType": "name",
	})
	if err != nil {
		return ToolResult{}, err
	}
	stored := decodeRecords(read.Text)
	if len(stored) == 0 || stored[0].field("state") != trackerStateClosed {
		return scratchRefusal(
			"the tracker accepted the close but the issue still reads as open, "+
				"so %s is unconfirmed: do not tell the member it closed", key)
	}
	return ToolResult{Text: "closed " + key, Detail: key}, nil
}

// resolveKey maps a key to its record id within the window a search reads. A
// key outside it is not found rather than guessed at.
func (s *trackerSession) resolveKey(ctx context.Context, key string) (string, error) {
	rows, err := s.openIssues(ctx)
	if err != nil {
		return "", err
	}
	for _, row := range rows {
		if strings.EqualFold(row.key(), key) {
			return row.ID, nil
		}
	}
	return "", nil
}

func (s *trackerSession) searchIssues(
	ctx context.Context,
	arguments map[string]any,
) (ToolResult, error) {
	query := strings.TrimSpace(scratchStringArg(arguments, "query"))
	if query == "" {
		return scratchRefusal("query is required to search issues")
	}
	rows, err := s.openIssues(ctx)
	if err != nil {
		return ToolResult{}, err
	}
	matched := make([]string, 0)
	for _, row := range rows {
		// Key as well as title: the reply hands a member the key, so a member
		// quoting it back has to reach the issue it names.
		if matchesQuery(row.key()+" "+row.field("title"), query) {
			matched = append(matched, row.key()+"  "+row.field("title"))
		}
	}
	// The bound travels with the result, so an empty window cannot become
	// "no such issue exists".
	bound := fmt.Sprintf("read %d open issues, the most recent in the tracker", len(rows))
	if len(matched) == 0 {
		return ToolResult{Text: "no open issue matched in what was read\n" + bound}, nil
	}
	return ToolResult{
		Text: strings.Join(matched, "\n") + "\n\n" + bound,
	}, nil
}

// openIssues reads the window the view defines. The view carries the filter
// and take the ceiling. See docs/sirens-echo-issues.md.
func (s *trackerSession) openIssues(ctx context.Context) ([]trackerRecord, error) {
	arguments := map[string]any{
		"tableId":      s.policy.IssuesTable,
		"fieldKeyType": "name",
		"take":         strconv.Itoa(maxTrackerSearchRows),
	}
	if view := strings.TrimSpace(s.policy.OpenIssuesView); view != "" {
		arguments["viewId"] = view
	}
	listed, err := s.record(ctx, recordListTool, arguments)
	if err != nil {
		return nil, err
	}
	if listed.IsError {
		return nil, nil
	}
	return decodeRecords(listed.Text), nil
}

// matchesQuery matches word-wise over a row's key and title, the tracker's own
// search being unreachable through this MCP.
func matchesQuery(haystack, query string) bool {
	haystack = strings.ToLower(haystack)
	for _, word := range strings.Fields(strings.ToLower(query)) {
		if !strings.Contains(haystack, word) {
			return false
		}
	}
	return true
}

// trackerRecord is the part of a record this adapter reads.
type trackerRecord struct {
	ID     string         `json:"id"`
	Name   string         `json:"name"`
	Fields map[string]any `json:"fields"`
}

func (r trackerRecord) field(name string) string {
	value, _ := r.Fields[name].(string)
	return value
}

// key prefers the primary value, falling back to the envelope name that
// carries the same formula.
func (r trackerRecord) key() string {
	if value := r.field("key"); value != "" {
		return value
	}
	return r.Name
}

// decodeRecords reads the one-record and many-record shapes. An unreadable
// body yields nothing, which callers treat as unconfirmed.
func decodeRecords(payload string) []trackerRecord {
	var envelope struct {
		Result json.RawMessage `json:"result"`
	}
	body := []byte(payload)
	if err := json.Unmarshal(body, &envelope); err == nil && len(envelope.Result) > 0 {
		body = envelope.Result
	}
	var many struct {
		Records []trackerRecord `json:"records"`
	}
	if err := json.Unmarshal(body, &many); err == nil && len(many.Records) > 0 {
		return many.Records
	}
	var one trackerRecord
	if err := json.Unmarshal(body, &one); err == nil && one.ID != "" {
		return []trackerRecord{one}
	}
	return nil
}

// firstRecordIdentity reads the id and key of the row a create returned.
func firstRecordIdentity(payload string) (string, string) {
	records := decodeRecords(payload)
	if len(records) == 0 {
		return "", ""
	}
	return records[0].ID, records[0].key()
}
