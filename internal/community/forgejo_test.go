package community

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

func TestForgejoMCPClientCreatesIssueWithoutLabelsOrCredential(t *testing.T) {
	t.Parallel()
	var posts atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Header.Get("Authorization") != "" {
			t.Error("Echo sent an authorization header to the Forgejo MCP")
		}
		switch request.URL.Path {
		case "/api/list_issue":
			writer.Header().Set("Content-Type", "application/json")
			_, _ = writer.Write([]byte(`{"structuredContent":{"result":[]}}`))
		case "/api/create_issue":
			posts.Add(1)
			var body map[string]any
			if err := json.NewDecoder(request.Body).Decode(&body); err != nil {
				t.Errorf("decode body: %v", err)
			}
			if _, exists := body["labels"]; exists {
				t.Error("create payload included labels")
			}
			if _, exists := body["html_url"]; exists {
				t.Error("create payload included a response-only field")
			}
			if body["title"] != "Knowledge gap: Event time" {
				t.Errorf("title = %#v", body["title"])
			}
			writer.Header().Set("Content-Type", "application/json")
			_, _ = writer.Write([]byte(`{"structuredContent":{"result":{"title":"Knowledge gap: Event time","html_url":"https://forgejo.example/issues/1"}}}`))
		default:
			t.Errorf("unexpected path %q", request.URL.Path)
		}
	}))
	defer server.Close()
	client := testForgejoClient(server.URL)
	url, err := client.EnsureIssue(context.Background(), IssueDraft{
		Kind:  "knowledge-gap",
		Title: "Event time",
		Body:  "The current skillpack has no verified event schedule.",
	})
	if err != nil {
		t.Fatalf("EnsureIssue: %v", err)
	}
	if url != "https://forgejo.example/issues/1" || posts.Load() != 1 {
		t.Fatalf("url = %q, posts = %d", url, posts.Load())
	}
}

func TestForgejoMCPClientReusesExactOpenIssue(t *testing.T) {
	t.Parallel()
	var posts atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/api/list_issue" {
			posts.Add(1)
			t.Errorf("unexpected path %q", request.URL.Path)
		}
		var body map[string]any
		if err := json.NewDecoder(request.Body).Decode(&body); err != nil {
			t.Errorf("decode body: %v", err)
		}
		if body["state"] != "open" || body["type"] != "issues" || body["limit"] != float64(3) {
			t.Errorf("list arguments = %#v", body)
		}
		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write([]byte(`{"structuredContent":{"result":[{"title":"Correction: Server rule","html_url":"https://forgejo.example/issues/2"}]}}`))
	}))
	defer server.Close()
	client := testForgejoClient(server.URL)
	url, err := client.EnsureIssue(context.Background(), IssueDraft{
		Kind:  "correction",
		Title: "Server rule",
		Body:  "A documented answer may be wrong.",
	})
	if err != nil {
		t.Fatalf("EnsureIssue: %v", err)
	}
	if url != "https://forgejo.example/issues/2" || posts.Load() != 0 {
		t.Fatalf("url = %q, posts = %d", url, posts.Load())
	}
}

func testForgejoClient(baseURL string) ForgejoMCPClient {
	return ForgejoMCPClient{
		MCPURL:     baseURL + "/mcp",
		HTTPClient: &http.Client{Timeout: time.Second},
	}
}
