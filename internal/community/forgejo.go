package community

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
)

// IssueTracker is the narrow Forgejo write boundary.
type IssueTracker interface {
	EnsureIssue(ctx context.Context, draft IssueDraft) (string, error)
}

// ForgejoMCPClient creates ordinary issues through Echo's guarded Forgejo MCP
// and reuses an exact-title open issue. Echo never receives a Forgejo token.
type ForgejoMCPClient struct {
	MCPURL     string
	HTTPClient *http.Client
}

type forgejoIssue struct {
	Title   string `json:"title"`
	Body    string `json:"body"`
	HTMLURL string `json:"html_url,omitempty"`
}

type mcpToolResult struct {
	IsError           bool `json:"isError"`
	StructuredContent struct {
		Result json.RawMessage `json:"result"`
	} `json:"structuredContent"`
}

// EnsureIssue returns an existing exact-title issue or creates one without
// labels, assignees, milestones, or any other tracker mutation.
func (c ForgejoMCPClient) EnsureIssue(ctx context.Context, draft IssueDraft) (string, error) {
	title := issueTitle(draft)
	existing, err := c.findOpenIssue(ctx, title)
	if err != nil {
		return "", err
	}
	if existing != "" {
		return existing, nil
	}
	body := fmt.Sprintf(`## Observed gap

%s

## Expected follow-through

Update the repository-local Sirens Echo skillpack and add a regression
conversation that demonstrates the corrected behavior.

Sirens Echo opened this issue from a sanitized #bots interaction. The issue
contains no member identity, raw Discord payload, or Discord identifier.`,
		draft.Body,
	)
	var created forgejoIssue
	if err := c.callTool(ctx, "create_issue", forgejoIssue{Title: title, Body: body}, &created); err != nil {
		return "", err
	}
	if created.HTMLURL == "" {
		return "", fmt.Errorf("created Forgejo issue has no URL")
	}
	return created.HTMLURL, nil
}

func (c ForgejoMCPClient) findOpenIssue(ctx context.Context, title string) (string, error) {
	arguments := map[string]any{
		"state": "open",
		"type":  "issues",
		"q":     title,
		"limit": 3,
	}
	var issues []forgejoIssue
	if err := c.callTool(ctx, "list_issue", arguments, &issues); err != nil {
		return "", err
	}
	for _, issue := range issues {
		if strings.EqualFold(strings.TrimSpace(issue.Title), title) && issue.HTMLURL != "" {
			return issue.HTMLURL, nil
		}
	}
	return "", nil
}

func (c ForgejoMCPClient) callTool(ctx context.Context, tool string, arguments any, result any) error {
	payload, err := json.Marshal(arguments)
	if err != nil {
		return fmt.Errorf("marshal Forgejo MCP %s arguments: %w", tool, err)
	}
	endpoint, err := c.toolURL(tool)
	if err != nil {
		return err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(payload))
	if err != nil {
		return fmt.Errorf("build Forgejo MCP %s request: %w", tool, err)
	}
	request.Header.Set("Accept", "application/json")
	request.Header.Set("Content-Type", "application/json")
	response, err := c.HTTPClient.Do(request)
	if err != nil {
		return fmt.Errorf("call Forgejo MCP %s: %w", tool, err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 4096))
		return fmt.Errorf("Forgejo MCP %s returned HTTP %d", tool, response.StatusCode)
	}
	var toolResult mcpToolResult
	if err := json.NewDecoder(io.LimitReader(response.Body, 2*1024*1024)).Decode(&toolResult); err != nil {
		return fmt.Errorf("decode Forgejo MCP %s result: %w", tool, err)
	}
	if toolResult.IsError {
		return fmt.Errorf("Forgejo MCP %s failed", tool)
	}
	if len(toolResult.StructuredContent.Result) == 0 {
		return fmt.Errorf("Forgejo MCP %s returned no structured result", tool)
	}
	if err := json.Unmarshal(toolResult.StructuredContent.Result, result); err != nil {
		return fmt.Errorf("decode Forgejo MCP %s structured result: %w", tool, err)
	}
	return nil
}

func (c ForgejoMCPClient) toolURL(tool string) (string, error) {
	endpoint, err := url.Parse(c.MCPURL)
	if err != nil || endpoint.Scheme == "" || endpoint.Host == "" {
		return "", fmt.Errorf("invalid Forgejo MCP URL")
	}
	path := strings.TrimSuffix(endpoint.Path, "/")
	if !strings.HasSuffix(path, "/mcp") {
		return "", fmt.Errorf("Forgejo MCP URL must end in /mcp")
	}
	endpoint.Path = strings.TrimSuffix(path, "/mcp") + "/api/" + url.PathEscape(tool)
	endpoint.RawPath = ""
	endpoint.RawQuery = ""
	endpoint.Fragment = ""
	return endpoint.String(), nil
}

func issueTitle(draft IssueDraft) string {
	prefix := "Knowledge gap: "
	if draft.Kind == "correction" {
		prefix = "Correction: "
	}
	return prefix + strings.TrimSpace(draft.Title)
}
