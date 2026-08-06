package community

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

const (
	maxAgentProxyResponseBytes = 2 * 1024 * 1024
	maxToolRounds              = 6
	maxResponseRepairs         = 1
)

const neutralResponseRepairPrompt = `The previous assistant response violated the required response contract.
Return only one valid JSON object with a non-empty "reply" string and an
"issue" value that is either null or an object. Do not use Markdown fences or
add commentary outside the JSON object. Rewrite the reply in neutral, concise,
impersonal language. Remove greetings, emojis, exclamation marks, first-person
or collective pronouns, banter, apologies, thanks, sign-offs, personality, and
offers of more help.`

const socialResponseRepairPrompt = `The previous assistant response violated the required response contract.
Return only one valid JSON object with a non-empty "reply" string and an
"issue" value that is either null or an object. Do not use Markdown fences or
add commentary outside the JSON object. Preserve the useful answer and the
selected social tone while fixing the JSON structure.`

// ExecutedTool records one model-requested tool that the runtime completed.
type ExecutedTool struct {
	Name      string
	Arguments string
	Result    string
}

// CompletionResult is the final model content plus its executed tool path.
type CompletionResult struct {
	Content   string
	ToolCalls []ExecutedTool
}

// CompletionClient is the inference boundary used by the Discord runtime.
type CompletionClient interface {
	Complete(
		ctx context.Context,
		systemPrompt, userPrompt, requestID string,
	) (CompletionResult, error)
}

// ProxyClient calls Agent Proxy's OpenAI-compatible chat surface and owns the
// model-to-MCP continuation loop.
type ProxyClient struct {
	BaseURL       string
	Model         string
	AuditRole     string
	Attribution   string
	ResponseStyle string
	HTTPClient    *http.Client
	Tools         ToolProvider
	Telemetry     *Telemetry
}

type chatRequest struct {
	Model          string             `json:"model"`
	Messages       []chatMessage      `json:"messages"`
	Tools          []chatTool         `json:"tools,omitempty"`
	ResponseFormat chatResponseFormat `json:"response_format"`
	Stream         bool               `json:"stream"`
	Temperature    float64            `json:"temperature"`
	MaxTokens      int                `json:"max_tokens"`
	Metadata       chatMetadata       `json:"metadata"`
}

type chatResponseFormat struct {
	Type string `json:"type"`
}

type chatMessage struct {
	Role             string         `json:"role"`
	Content          any            `json:"content"`
	ReasoningContent string         `json:"reasoning_content,omitempty"`
	ToolCalls        []chatToolCall `json:"tool_calls,omitempty"`
	ToolCallID       string         `json:"tool_call_id,omitempty"`
	Name             string         `json:"name,omitempty"`
}

type chatTool struct {
	Type     string           `json:"type"`
	Function chatToolFunction `json:"function"`
}

type chatToolFunction struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	Parameters  any    `json:"parameters"`
}

type chatToolCall struct {
	ID       string             `json:"id"`
	Type     string             `json:"type"`
	Function chatCalledFunction `json:"function"`
}

type chatCalledFunction struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

type chatMetadata struct {
	RequestID string `json:"request_id,omitempty"`
	Role      string `json:"role"`
	Seat      string `json:"seat"`
}

type chatResponse struct {
	Choices []struct {
		Message chatResponseMessage `json:"message"`
	} `json:"choices"`
}

type chatResponseMessage struct {
	Content          chatContent    `json:"content"`
	ReasoningContent string         `json:"reasoning_content"`
	ToolCalls        []chatToolCall `json:"tool_calls"`
}

type chatContent struct {
	Text string
}

// UnmarshalJSON accepts OpenAI string or null content plus the text-part arrays
// returned by some compatible gateways.
func (c *chatContent) UnmarshalJSON(raw []byte) error {
	if bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
		c.Text = ""
		return nil
	}
	var text string
	if err := json.Unmarshal(raw, &text); err == nil {
		c.Text = text
		return nil
	}
	var parts []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	}
	if err := json.Unmarshal(raw, &parts); err == nil {
		var output strings.Builder
		for _, part := range parts {
			if part.Type == "" || part.Type == "text" || part.Type == "output_text" {
				output.WriteString(part.Text)
			}
		}
		c.Text = output.String()
		return nil
	}
	var object struct {
		Text string `json:"text"`
	}
	if err := json.Unmarshal(raw, &object); err == nil && object.Text != "" {
		c.Text = object.Text
		return nil
	}
	return fmt.Errorf("unsupported Agent Proxy message content")
}

// Complete submits a non-streaming request, executes requested MCP tools, and
// continues until Agent Proxy returns member-facing content.
func (c ProxyClient) Complete(
	ctx context.Context,
	systemPrompt, userPrompt, requestID string,
) (CompletionResult, error) {
	telemetry := telemetryOrNoop(c.Telemetry)
	var toolSession ToolSession
	var tools []chatTool
	toolDefinitions := make(map[string]ToolDefinition)
	if c.Tools != nil {
		listCtx, listSpan := telemetry.StartSpan(ctx, "mcp.tools.list")
		opened, err := c.Tools.Open(listCtx)
		if err != nil {
			telemetry.MarkSpanError(listSpan, exceptionMCPToolsListFailed)
			listSpan.End()
			return CompletionResult{}, err
		}
		toolSession = opened
		defer func() {
			if err := toolSession.Close(); err != nil {
				telemetry.MarkSpanError(
					trace.SpanFromContext(ctx),
					exceptionMCPSessionCloseFailed,
				)
				telemetry.Error(
					ctx,
					"mcp.session.close.failed",
					slog.String("error_type", "mcp_session_close_failed"),
				)
			}
		}()
		for _, definition := range toolSession.Tools() {
			tools = append(tools, chatTool{
				Type: "function",
				Function: chatToolFunction{
					Name:        definition.Name,
					Description: definition.Description,
					Parameters:  definition.InputSchema,
				},
			})
			toolDefinitions[definition.Name] = definition
		}
		listSpan.SetAttributes(attribute.Int("mcp.tool.count", len(tools)))
		listSpan.End()
		telemetry.Info(
			ctx,
			"mcp.tools.discovered",
			slog.Int("tool_count", len(tools)),
		)
	}

	messages := []chatMessage{
		{Role: "system", Content: systemPrompt},
		{Role: "user", Content: userPrompt},
	}
	executed := make([]ExecutedTool, 0)
	toolRounds := 0
	repairAttempts := 0
	maxModelCalls := maxToolRounds + maxResponseRepairs + 1
	for round := 0; round < maxModelCalls; round++ {
		requestTools := tools
		if repairAttempts > 0 {
			requestTools = nil
		}
		payload := chatRequest{
			Model:          c.Model,
			Messages:       messages,
			Tools:          requestTools,
			ResponseFormat: chatResponseFormat{Type: "json_object"},
			Stream:         false,
			Temperature:    0,
			MaxTokens:      900,
			Metadata: chatMetadata{
				RequestID: requestID,
				Role:      c.AuditRole,
				Seat:      c.Attribution,
			},
		}
		message, err := c.completeOnce(ctx, payload, requestID, round)
		if err != nil {
			return CompletionResult{}, err
		}
		if repairAttempts > 0 && len(message.ToolCalls) > 0 {
			return CompletionResult{}, fmt.Errorf(
				"Agent Proxy returned a tool call during response repair",
			)
		}
		if len(message.ToolCalls) == 0 {
			content := strings.TrimSpace(message.Content.Text)
			contractErr := fmt.Errorf("Agent Proxy returned invalid structured output")
			if validJSONObject(content) {
				decision, err := ParseDecision(content)
				if err == nil {
					err = ValidateResponseStyle(c.ResponseStyle, decision)
				}
				contractErr = err
			}
			if contractErr != nil {
				if repairAttempts >= maxResponseRepairs {
					return CompletionResult{}, fmt.Errorf(
						"Agent Proxy returned invalid response after %d repair attempt: %w",
						repairAttempts,
						contractErr,
					)
				}
				repairAttempts++
				telemetry.Info(
					ctx,
					"model.response.repair",
					slog.Int("attempt", repairAttempts),
				)
				if content != "" {
					messages = append(
						messages,
						chatMessage{Role: "assistant", Content: content},
					)
				}
				messages = append(
					messages,
					chatMessage{Role: "user", Content: responseRepairPrompt(c.ResponseStyle)},
				)
				continue
			}
			return CompletionResult{Content: content, ToolCalls: executed}, nil
		}
		if toolRounds == maxToolRounds {
			return CompletionResult{}, fmt.Errorf("Agent Proxy exceeded %d MCP tool rounds", maxToolRounds)
		}
		toolRounds++
		if toolSession == nil {
			return CompletionResult{}, fmt.Errorf("Agent Proxy requested a tool with no MCP roster")
		}
		assistantContent := any(nil)
		if message.Content.Text != "" {
			assistantContent = message.Content.Text
		}
		messages = append(messages, chatMessage{
			Role:             "assistant",
			Content:          assistantContent,
			ReasoningContent: message.ReasoningContent,
			ToolCalls:        message.ToolCalls,
		})
		for _, call := range message.ToolCalls {
			if call.ID == "" || call.Function.Name == "" {
				return CompletionResult{}, fmt.Errorf("Agent Proxy returned an incomplete tool call")
			}
			definition, exists := toolDefinitions[call.Function.Name]
			if !exists {
				return CompletionResult{}, fmt.Errorf(
					"Agent Proxy requested unavailable MCP tool %q",
					call.Function.Name,
				)
			}
			arguments := make(map[string]any)
			if strings.TrimSpace(call.Function.Arguments) != "" {
				if err := json.Unmarshal([]byte(call.Function.Arguments), &arguments); err != nil {
					return CompletionResult{}, fmt.Errorf(
						"parse arguments for MCP tool %s: %w",
						call.Function.Name,
						err,
					)
				}
			}
			toolCtx, toolSpan := telemetry.StartSpan(
				ctx,
				"mcp.tool.call",
				attribute.String("mcp.server.name", definition.Server),
				attribute.String("mcp.tool.name", definition.Original),
			)
			telemetry.Info(
				toolCtx,
				"mcp.tool.input",
				slog.String("server", definition.Server),
				slog.String("tool", definition.Original),
				slog.Int("input_bytes", len(call.Function.Arguments)),
			)
			result, err := toolSession.Call(toolCtx, call.Function.Name, arguments)
			if err != nil {
				telemetry.RecordToolCall(toolCtx, definition.Server, definition.Original, "error")
				telemetry.MarkSpanError(toolSpan, exceptionMCPToolCallFailed)
				toolSpan.End()
				return CompletionResult{}, err
			}
			telemetry.Info(
				toolCtx,
				"mcp.tool.result",
				slog.String("server", definition.Server),
				slog.String("tool", definition.Original),
				slog.Int("result_bytes", len(result)),
			)
			telemetry.RecordToolCall(toolCtx, definition.Server, definition.Original, "ok")
			toolSpan.End()
			executed = append(executed, ExecutedTool{
				Name:      call.Function.Name,
				Arguments: call.Function.Arguments,
				Result:    result,
			})
			messages = append(messages, chatMessage{
				Role:       "tool",
				Content:    result,
				ToolCallID: call.ID,
				Name:       call.Function.Name,
			})
		}
	}
	return CompletionResult{}, fmt.Errorf("Agent Proxy tool loop ended unexpectedly")
}

func responseRepairPrompt(style string) string {
	if style == ResponseStyleSocial {
		return socialResponseRepairPrompt
	}
	return neutralResponseRepairPrompt
}

func validJSONObject(raw string) bool {
	var object map[string]json.RawMessage
	return json.Unmarshal([]byte(raw), &object) == nil && object != nil
}

func (c ProxyClient) completeOnce(
	ctx context.Context,
	payload chatRequest,
	requestID string,
	round int,
) (chatResponseMessage, error) {
	telemetry := telemetryOrNoop(c.Telemetry)
	modelCtx, modelSpan := telemetry.StartSpan(
		ctx,
		"model.chat",
		attribute.Int("model.round", round),
	)
	raw, err := json.Marshal(payload)
	if err != nil {
		telemetry.MarkSpanError(modelSpan, exceptionModelRequestMarshalFailed)
		modelSpan.End()
		return chatResponseMessage{}, fmt.Errorf("marshal Agent Proxy request: %w", err)
	}
	telemetry.Info(
		modelCtx,
		"model.request",
		slog.Int("round", round),
		slog.Int("request_bytes", len(raw)),
		slog.Int("message_count", len(payload.Messages)),
		slog.Int("tool_count", len(payload.Tools)),
	)
	endpoint := strings.TrimRight(c.BaseURL, "/") + "/v1/chat/completions"
	request, err := http.NewRequestWithContext(modelCtx, http.MethodPost, endpoint, bytes.NewReader(raw))
	if err != nil {
		telemetry.MarkSpanError(modelSpan, exceptionModelRequestBuildFailed)
		modelSpan.End()
		return chatResponseMessage{}, fmt.Errorf("build Agent Proxy request: %w", err)
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("X-Request-ID", requestID)
	request.Header.Set("X-Ward-Role", c.AuditRole)
	request.Header.Set("X-Ward-Harness", "discord")
	request.Header.Set("X-Ward-Target-Repo", "coilyco-gaming/sirens-echo")

	response, err := c.HTTPClient.Do(request)
	if err != nil {
		telemetry.RecordModelCall(modelCtx, "error")
		telemetry.MarkSpanError(modelSpan, exceptionModelTransportFailed)
		modelSpan.End()
		return chatResponseMessage{}, fmt.Errorf("Agent Proxy request: %w", err)
	}
	defer response.Body.Close()
	responseRaw, err := io.ReadAll(io.LimitReader(response.Body, maxAgentProxyResponseBytes+1))
	if err != nil {
		telemetry.RecordModelCall(modelCtx, "error")
		telemetry.MarkSpanError(modelSpan, exceptionModelResponseReadFailed)
		modelSpan.End()
		return chatResponseMessage{}, fmt.Errorf("read Agent Proxy response: %w", err)
	}
	telemetry.Info(
		modelCtx,
		"model.response",
		slog.Int("round", round),
		slog.Int("status", response.StatusCode),
		slog.Int("response_bytes", len(responseRaw)),
	)
	if len(responseRaw) > maxAgentProxyResponseBytes {
		err := fmt.Errorf("Agent Proxy response exceeded %d bytes", maxAgentProxyResponseBytes)
		telemetry.RecordModelCall(modelCtx, "error")
		telemetry.MarkSpanError(modelSpan, exceptionModelResponseTooLarge)
		modelSpan.End()
		return chatResponseMessage{}, err
	}
	if response.StatusCode != http.StatusOK {
		err := fmt.Errorf("Agent Proxy returned HTTP %d", response.StatusCode)
		telemetry.RecordModelCall(modelCtx, "error")
		telemetry.MarkSpanError(modelSpan, exceptionModelResponseHTTPError)
		modelSpan.End()
		return chatResponseMessage{}, err
	}
	var completion chatResponse
	if err := json.Unmarshal(responseRaw, &completion); err != nil {
		telemetry.RecordModelCall(modelCtx, "error")
		telemetry.MarkSpanError(modelSpan, exceptionModelResponseDecodeFailed)
		modelSpan.End()
		return chatResponseMessage{}, fmt.Errorf("decode Agent Proxy response: %w", err)
	}
	if len(completion.Choices) == 0 {
		err := fmt.Errorf("Agent Proxy response contained no choices")
		telemetry.RecordModelCall(modelCtx, "error")
		telemetry.MarkSpanError(modelSpan, exceptionModelResponseMissingChoice)
		modelSpan.End()
		return chatResponseMessage{}, err
	}
	telemetry.RecordModelCall(modelCtx, "ok")
	modelSpan.End()
	return completion.Choices[0].Message, nil
}
