package community

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestProxyClientSendsBoundedCommunityRequest(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/v1/chat/completions" {
			t.Errorf("path = %s", request.URL.Path)
		}
		if request.Header.Get("X-Ward-Role") != "community" {
			t.Errorf("role header = %q", request.Header.Get("X-Ward-Role"))
		}
		if request.Header.Get("X-Ward-Harness") != "discord" {
			t.Errorf("harness header = %q", request.Header.Get("X-Ward-Harness"))
		}
		var body chatRequest
		if err := json.NewDecoder(request.Body).Decode(&body); err != nil {
			t.Errorf("decode body: %v", err)
		}
		if body.Model != "ornith:35b" {
			t.Errorf("model = %q", body.Model)
		}
		if body.Stream {
			t.Error("stream must be false")
		}
		if len(body.Messages) != 2 || body.Messages[0].Role != "system" || body.Messages[1].Role != "user" {
			t.Errorf("messages = %#v", body.Messages)
		}
		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write([]byte(`{"choices":[{"message":{"content":"{\"reply\":\"hello\",\"issue\":null}"}}]}`))
	}))
	defer server.Close()

	client := ProxyClient{
		BaseURL:    server.URL,
		Model:      "ornith:35b",
		HTTPClient: &http.Client{Timeout: time.Second},
	}
	got, err := client.Complete(context.Background(), "system", "user", "request")
	if err != nil {
		t.Fatalf("Complete: %v", err)
	}
	if got != `{"reply":"hello","issue":null}` {
		t.Fatalf("completion = %q", got)
	}
}

func TestProxyClientRejectsNonSuccess(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		http.Error(writer, "internal detail should not escape", http.StatusTooManyRequests)
	}))
	defer server.Close()
	client := ProxyClient{
		BaseURL:    server.URL,
		Model:      "ornith:35b",
		HTTPClient: &http.Client{Timeout: time.Second},
	}
	if _, err := client.Complete(context.Background(), "system", "user", "request"); err == nil {
		t.Fatal("expected HTTP error")
	}
}
