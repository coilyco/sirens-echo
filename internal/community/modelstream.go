package community

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"
)

// Reading the completion as a stream, so silence and slowness stop looking
// alike. See docs/sirens-echo-model-call.md.

// ErrModelSilent is a backend that sent nothing at all for the idle timeout. It
// is a different fact from the turn running out of budget while work continued.
var ErrModelSilent = errors.New("no bytes from the model backend")

// streamHeartbeat is Agent Proxy's SSE comment payload. Only State is read
// here; the rest is carried so a log line can say which backend was trying.
type streamHeartbeat struct {
	State   string `json:"state"`
	Attempt int    `json:"n"`
	Of      int    `json:"of"`
	Backend string `json:"backend"`
	Regime  string `json:"regime"`
}

// streamChunk is one `data:` frame. Agent Proxy sends OpenAI chunk shape, so
// the fields that matter are the delta and the finish reason.
type streamChunk struct {
	Model   string `json:"model"`
	Choices []struct {
		Delta struct {
			Content          string                `json:"content"`
			ReasoningContent *string               `json:"reasoning_content"`
			ToolCalls        []streamToolCallDelta `json:"tool_calls"`
		} `json:"delta"`
		FinishReason string `json:"finish_reason"`
	} `json:"choices"`
}

// streamToolCallDelta is one fragment of one tool call, keyed by index.
type streamToolCallDelta struct {
	Index    int    `json:"index"`
	ID       string `json:"id"`
	Type     string `json:"type"`
	Function *struct {
		Name      string `json:"name"`
		Arguments string `json:"arguments"`
	} `json:"function"`
}

// assembledToolCall accumulates one call's fragments in arrival order.
type assembledToolCall struct {
	id        string
	kind      string
	name      strings.Builder
	arguments strings.Builder
}

// streamLine is one line off the wire, or the end of it.
type streamLine struct {
	text string
	err  error
}

// readModelStream assembles one completion, treating any line as activity so a
// queued turn outlives the idle timer and a hung one does not.
func readModelStream(
	ctx context.Context,
	body io.Reader,
	idle time.Duration,
	onHeartbeat func(streamHeartbeat),
) (chatChoice, error) {
	lines := make(chan streamLine)
	go scanStream(ctx, body, lines)

	var (
		choice    chatChoice
		content   strings.Builder
		reasoning strings.Builder
		sawReason bool
		calls     = map[int]*assembledToolCall{}
		order     []*assembledToolCall
		bytesSeen int
		done      bool
	)
	timer := time.NewTimer(idle)
	defer timer.Stop()
	for !done {
		select {
		case <-ctx.Done():
			return chatChoice{}, ctx.Err()
		case <-timer.C:
			return chatChoice{}, fmt.Errorf(
				"Agent Proxy sent nothing for %s: %w", idle, ErrModelSilent,
			)
		case line, ok := <-lines:
			if !ok {
				done = true
				break
			}
			if line.err != nil {
				return chatChoice{}, fmt.Errorf("read Agent Proxy stream: %w", line.err)
			}
			// Reset before the work, so a slow parse is not counted as idle.
			if !timer.Stop() {
				select {
				case <-timer.C:
				default:
				}
			}
			timer.Reset(idle)

			bytesSeen += len(line.text)
			if bytesSeen > maxAgentProxyResponseBytes {
				return chatChoice{}, fmt.Errorf(
					"%w: stream of %d bytes", ErrResponseTooLarge, maxAgentProxyResponseBytes,
				)
			}
			text := strings.TrimRight(line.text, "\r")
			if text == "" {
				continue
			}
			// A comment carries the heartbeat. A client that ignored it would
			// still see every data frame, which is why the proxy chose this shape.
			if strings.HasPrefix(text, ":") {
				var beat streamHeartbeat
				if err := json.Unmarshal(
					[]byte(strings.TrimSpace(strings.TrimPrefix(text, ":"))), &beat,
				); err == nil && onHeartbeat != nil {
					onHeartbeat(beat)
				}
				continue
			}
			payload, isData := strings.CutPrefix(text, "data:")
			if !isData {
				continue
			}
			payload = strings.TrimSpace(payload)
			if payload == "[DONE]" {
				done = true
				break
			}
			var chunk streamChunk
			if err := json.Unmarshal([]byte(payload), &chunk); err != nil {
				return chatChoice{}, fmt.Errorf("decode Agent Proxy chunk: %w", err)
			}
			if chunk.Model != "" {
				choice.ServedModel = chunk.Model
			}
			if len(chunk.Choices) == 0 {
				continue
			}
			first := chunk.Choices[0]
			content.WriteString(first.Delta.Content)
			if first.Delta.ReasoningContent != nil {
				sawReason = true
				reasoning.WriteString(*first.Delta.ReasoningContent)
			}
			for _, delta := range first.Delta.ToolCalls {
				call, known := calls[delta.Index]
				// Some backends send parallel calls whole, all at index 0, which
				// joined two names into one call. See sirens-echo#8071.
				if !known || startsAnotherCall(call, delta) {
					call = &assembledToolCall{}
					calls[delta.Index] = call
					order = append(order, call)
				}
				if delta.ID != "" {
					call.id = delta.ID
				}
				if delta.Type != "" {
					call.kind = delta.Type
				}
				if delta.Function != nil {
					call.name.WriteString(delta.Function.Name)
					call.arguments.WriteString(delta.Function.Arguments)
				}
			}
			if first.FinishReason != "" {
				choice.FinishReason = first.FinishReason
			}
		}
	}

	choice.Message.Content = chatContent{Text: content.String()}
	if sawReason {
		text := reasoning.String()
		choice.Message.ReasoningContent = &text
	}
	choice.Message.ToolCalls = collectToolCalls(order)
	return choice, nil
}

// startsAnotherCall reports a fragment that cannot continue the call at its
// index: a different id, or a name after the call already has name and arguments.
func startsAnotherCall(call *assembledToolCall, delta streamToolCallDelta) bool {
	if delta.ID != "" && call.id != "" && delta.ID != call.id {
		return true
	}
	return delta.Function != nil && delta.Function.Name != "" &&
		call.name.Len() > 0 && call.arguments.Len() > 0
}

// collectToolCalls returns the assembled calls in the order they began, so a
// two-call round reaches the executor the way the model meant it.
func collectToolCalls(order []*assembledToolCall) []chatToolCall {
	if len(order) == 0 {
		return nil
	}
	assembled := make([]chatToolCall, 0, len(order))
	for _, call := range order {
		kind := call.kind
		if kind == "" {
			kind = "function"
		}
		assembled = append(assembled, chatToolCall{
			ID:   call.id,
			Type: kind,
			Function: chatCalledFunction{
				Name:      call.name.String(),
				Arguments: call.arguments.String(),
			},
		})
	}
	return assembled
}

// scanStream turns the body into lines. It closes the channel at EOF so the
// reader can tell a finished stream from one that stopped talking.
func scanStream(ctx context.Context, body io.Reader, lines chan<- streamLine) {
	defer close(lines)
	scanner := bufio.NewScanner(body)
	scanner.Buffer(make([]byte, 0, 64*1024), maxAgentProxyResponseBytes)
	for scanner.Scan() {
		select {
		case lines <- streamLine{text: scanner.Text()}:
		case <-ctx.Done():
			return
		}
	}
	// A cancelled context surfaces here as a read error, and the reader is
	// already returning ctx.Err() for that case.
	if err := scanner.Err(); err != nil && ctx.Err() == nil {
		select {
		case lines <- streamLine{err: err}:
		case <-ctx.Done():
		}
	}
}
