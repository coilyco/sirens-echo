package community

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

// CompletionClient is the inference boundary used by the Discord runtime.
type CompletionClient interface {
	Complete(ctx context.Context, systemPrompt, userPrompt, requestID string) (string, error)
}

// ProxyClient calls Agent Proxy's OpenAI-compatible chat surface.
type ProxyClient struct {
	BaseURL      string
	Model        string
	Role         string
	Seat         string
	IntentRoutes []IntentRoute
	HTTPClient   *http.Client
}

type chatRequest struct {
	Model       string        `json:"model"`
	Messages    []chatMessage `json:"messages"`
	Stream      bool          `json:"stream"`
	Temperature float64       `json:"temperature"`
	MaxTokens   int           `json:"max_tokens"`
	Metadata    chatMetadata  `json:"metadata"`
}

type chatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type chatMetadata struct {
	RequestID string   `json:"request_id,omitempty"`
	Role      string   `json:"role"`
	Seat      string   `json:"seat"`
	Intents   []string `json:"intents"`
	Harnesses []string `json:"harnesses"`
}

type chatResponse struct {
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	} `json:"choices"`
}

// Complete submits one non-streaming community response request.
func (c ProxyClient) Complete(ctx context.Context, systemPrompt, userPrompt, requestID string) (string, error) {
	intents := make([]string, 0, len(c.IntentRoutes))
	harnesses := make([]string, 0, len(c.IntentRoutes))
	for _, route := range c.IntentRoutes {
		intents = append(intents, route.Intent)
		harnesses = append(harnesses, route.Harness)
	}
	payload := chatRequest{
		Model: c.Model,
		Messages: []chatMessage{
			{Role: "system", Content: systemPrompt},
			{Role: "user", Content: userPrompt},
		},
		Stream:      false,
		Temperature: 0.2,
		MaxTokens:   900,
		Metadata: chatMetadata{
			RequestID: requestID,
			Role:      c.Role,
			Seat:      c.Seat,
			Intents:   intents,
			Harnesses: harnesses,
		},
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("marshal Agent Proxy request: %w", err)
	}
	endpoint := strings.TrimRight(c.BaseURL, "/") + "/v1/chat/completions"
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(raw))
	if err != nil {
		return "", fmt.Errorf("build Agent Proxy request: %w", err)
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("X-Request-ID", requestID)
	request.Header.Set("X-Ward-Role", "community")
	request.Header.Set("X-Ward-Harness", "discord")
	request.Header.Set("X-Ward-Target-Repo", "coilyco-gaming/sirens-discord-ops")

	response, err := c.HTTPClient.Do(request)
	if err != nil {
		return "", fmt.Errorf("Agent Proxy request: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 4096))
		return "", fmt.Errorf("Agent Proxy returned HTTP %d", response.StatusCode)
	}
	var completion chatResponse
	if err := json.NewDecoder(io.LimitReader(response.Body, 2*1024*1024)).Decode(&completion); err != nil {
		return "", fmt.Errorf("decode Agent Proxy response: %w", err)
	}
	if len(completion.Choices) == 0 || strings.TrimSpace(completion.Choices[0].Message.Content) == "" {
		return "", fmt.Errorf("Agent Proxy response contained no message")
	}
	return completion.Choices[0].Message.Content, nil
}
