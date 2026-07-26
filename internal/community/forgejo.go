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

// ForgejoClient creates ordinary issues and reuses an exact-title open issue.
type ForgejoClient struct {
	BaseURL    string
	Token      string
	Owner      string
	Repo       string
	HTTPClient *http.Client
}

type forgejoIssue struct {
	Title   string `json:"title"`
	Body    string `json:"body"`
	HTMLURL string `json:"html_url"`
}

// EnsureIssue returns an existing exact-title issue or creates one without
// labels, assignees, milestones, or any other tracker mutation.
func (c ForgejoClient) EnsureIssue(ctx context.Context, draft IssueDraft) (string, error) {
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
	payload, err := json.Marshal(forgejoIssue{Title: title, Body: body})
	if err != nil {
		return "", fmt.Errorf("marshal Forgejo issue: %w", err)
	}
	endpoint := c.repoIssuesURL()
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(payload))
	if err != nil {
		return "", fmt.Errorf("build Forgejo create request: %w", err)
	}
	c.setHeaders(request)
	response, err := c.HTTPClient.Do(request)
	if err != nil {
		return "", fmt.Errorf("create Forgejo issue: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusCreated {
		_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 4096))
		return "", fmt.Errorf("Forgejo create returned HTTP %d", response.StatusCode)
	}
	var created forgejoIssue
	if err := json.NewDecoder(io.LimitReader(response.Body, 1024*1024)).Decode(&created); err != nil {
		return "", fmt.Errorf("decode created Forgejo issue: %w", err)
	}
	if created.HTMLURL == "" {
		return "", fmt.Errorf("created Forgejo issue has no URL")
	}
	return created.HTMLURL, nil
}

func (c ForgejoClient) findOpenIssue(ctx context.Context, title string) (string, error) {
	query := url.Values{
		"state": {"open"},
		"type":  {"issues"},
		"q":     {title},
		"limit": {"50"},
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, c.repoIssuesURL()+"?"+query.Encode(), nil)
	if err != nil {
		return "", fmt.Errorf("build Forgejo list request: %w", err)
	}
	c.setHeaders(request)
	response, err := c.HTTPClient.Do(request)
	if err != nil {
		return "", fmt.Errorf("list Forgejo issues: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 4096))
		return "", fmt.Errorf("Forgejo list returned HTTP %d", response.StatusCode)
	}
	var issues []forgejoIssue
	if err := json.NewDecoder(io.LimitReader(response.Body, 2*1024*1024)).Decode(&issues); err != nil {
		return "", fmt.Errorf("decode Forgejo issues: %w", err)
	}
	for _, issue := range issues {
		if strings.EqualFold(strings.TrimSpace(issue.Title), title) && issue.HTMLURL != "" {
			return issue.HTMLURL, nil
		}
	}
	return "", nil
}

func (c ForgejoClient) repoIssuesURL() string {
	return fmt.Sprintf(
		"%s/api/v1/repos/%s/%s/issues",
		strings.TrimRight(c.BaseURL, "/"),
		url.PathEscape(c.Owner),
		url.PathEscape(c.Repo),
	)
}

func (c ForgejoClient) setHeaders(request *http.Request) {
	request.Header.Set("Accept", "application/json")
	request.Header.Set("Authorization", "token "+c.Token)
	if request.Body != nil {
		request.Header.Set("Content-Type", "application/json")
	}
}

func issueTitle(draft IssueDraft) string {
	prefix := "Knowledge gap: "
	if draft.Kind == "correction" {
		prefix = "Correction: "
	}
	return prefix + strings.TrimSpace(draft.Title)
}
