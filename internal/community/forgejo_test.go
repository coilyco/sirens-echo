package community

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestForgejoClientCreatesIssueWithoutLabels(t *testing.T) {
	t.Parallel()
	var posts atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		switch request.Method {
		case http.MethodGet:
			writer.Header().Set("Content-Type", "application/json")
			_, _ = writer.Write([]byte(`[]`))
		case http.MethodPost:
			posts.Add(1)
			if request.Header.Get("Authorization") != "token secret" {
				t.Errorf("authorization header was not set")
			}
			var body map[string]any
			if err := json.NewDecoder(request.Body).Decode(&body); err != nil {
				t.Errorf("decode body: %v", err)
			}
			if _, exists := body["labels"]; exists {
				t.Error("create payload included labels")
			}
			if body["title"] != "Knowledge gap: Event time" {
				t.Errorf("title = %#v", body["title"])
			}
			writer.Header().Set("Content-Type", "application/json")
			writer.WriteHeader(http.StatusCreated)
			_, _ = writer.Write([]byte(`{"title":"Knowledge gap: Event time","html_url":"https://forgejo.example/issues/1"}`))
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

func TestForgejoClientReusesExactOpenIssue(t *testing.T) {
	t.Parallel()
	var posts atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Method == http.MethodPost {
			posts.Add(1)
		}
		if !strings.Contains(request.URL.RawQuery, "state=open") {
			t.Errorf("query = %q", request.URL.RawQuery)
		}
		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write([]byte(`[{"title":"Correction: Server rule","html_url":"https://forgejo.example/issues/2"}]`))
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

func testForgejoClient(baseURL string) ForgejoClient {
	return ForgejoClient{
		BaseURL:    baseURL,
		Token:      "secret",
		Owner:      "coilyco-gaming",
		Repo:       "sirens-echo",
		HTTPClient: &http.Client{Timeout: time.Second},
	}
}
