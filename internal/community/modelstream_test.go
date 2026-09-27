package community

import (
	"context"
	"errors"
	"io"
	"strings"
	"testing"
	"time"
)

// The acceptance criteria on sirens-echo#171 are the spec these pin.

// slowReader releases each chunk after a delay, so a test can produce silence
// without sleeping through a real backend.
type slowReader struct {
	chunks []string
	gap    time.Duration
	at     int
}

func (r *slowReader) Read(into []byte) (int, error) {
	if r.at >= len(r.chunks) {
		return 0, io.EOF
	}
	time.Sleep(r.gap)
	n := copy(into, r.chunks[r.at])
	r.at++
	return n, nil
}

func streamOf(lines ...string) io.Reader {
	return strings.NewReader(strings.Join(lines, "\n") + "\n")
}

// The plain path: deltas concatenate into one answer.
func TestAStreamedCompletionAssemblesItsDeltas(t *testing.T) {
	t.Parallel()
	choice, err := readModelStream(context.Background(), streamOf(
		`data: {"model":"ornith:35b","choices":[{"delta":{"role":"assistant"}}]}`,
		`data: {"choices":[{"delta":{"content":"Par"}}]}`,
		`data: {"choices":[{"delta":{"content":"is"}}]}`,
		`data: {"choices":[{"delta":{},"finish_reason":"stop"}]}`,
		`data: [DONE]`,
	), time.Second, nil)
	if err != nil {
		t.Fatalf("readModelStream: %v", err)
	}
	if choice.Message.Content.Text != "Paris" {
		t.Errorf("content = %q, want %q", choice.Message.Content.Text, "Paris")
	}
	if choice.FinishReason != "stop" {
		t.Errorf("finish reason = %q", choice.FinishReason)
	}
	if choice.ServedModel != "ornith:35b" {
		t.Errorf("served model = %q, and a fallback answering as another model is "+
			"exactly what this field exists to record", choice.ServedModel)
	}
}

// A tool call arrives in fragments and has to survive being reassembled, or the
// executor gets a truncated argument string and fails for the wrong reason.
func TestAStreamedToolCallReassemblesItsFragments(t *testing.T) {
	t.Parallel()
	choice, err := readModelStream(context.Background(), streamOf(
		`data: {"choices":[{"delta":{"tool_calls":[{"index":0,"id":"call_1","type":"function","function":{"name":"eco_get_"}}]}}]}`,
		`data: {"choices":[{"delta":{"tool_calls":[{"index":0,"function":{"name":"market"}}]}}]}`,
		`data: {"choices":[{"delta":{"tool_calls":[{"index":0,"function":{"arguments":"{\"item\":"}}]}}]}`,
		`data: {"choices":[{"delta":{"tool_calls":[{"index":0,"function":{"arguments":"\"wood\"}"}}]}}]}`,
		`data: {"choices":[{"delta":{},"finish_reason":"tool_calls"}]}`,
		`data: [DONE]`,
	), time.Second, nil)
	if err != nil {
		t.Fatalf("readModelStream: %v", err)
	}
	if len(choice.Message.ToolCalls) != 1 {
		t.Fatalf("got %d tool calls, want 1", len(choice.Message.ToolCalls))
	}
	call := choice.Message.ToolCalls[0]
	if call.Function.Name != "eco_get_market" {
		t.Errorf("name = %q, want the two fragments joined", call.Function.Name)
	}
	if call.Function.Arguments != `{"item":"wood"}` {
		t.Errorf("arguments = %q, want the four fragments joined", call.Function.Arguments)
	}
	if call.ID != "call_1" || call.Type != "function" {
		t.Errorf("identity lost: id=%q type=%q", call.ID, call.Type)
	}
}

// Two calls in one round must reach the executor in the order the model meant.
func TestTwoStreamedToolCallsKeepTheirOrder(t *testing.T) {
	t.Parallel()
	choice, err := readModelStream(context.Background(), streamOf(
		`data: {"choices":[{"delta":{"tool_calls":[{"index":1,"id":"b","function":{"name":"second"}}]}}]}`,
		`data: {"choices":[{"delta":{"tool_calls":[{"index":0,"id":"a","function":{"name":"first"}}]}}]}`,
		`data: [DONE]`,
	), time.Second, nil)
	if err != nil {
		t.Fatalf("readModelStream: %v", err)
	}
	if len(choice.Message.ToolCalls) != 2 {
		t.Fatalf("got %d calls, want 2", len(choice.Message.ToolCalls))
	}
	// First seen, not lowest index: the stream's order is the model's order.
	if choice.Message.ToolCalls[0].Function.Name != "second" {
		t.Errorf("order = %q first, want arrival order",
			choice.Message.ToolCalls[0].Function.Name)
	}
}

// Acceptance 1: a turn receiving heartbeats survives past what a total deadline
// would have cut, because a heartbeat is activity.
func TestHeartbeatsKeepAStreamAliveThroughSilence(t *testing.T) {
	t.Parallel()
	idle := 120 * time.Millisecond
	body := &slowReader{
		gap: 60 * time.Millisecond,
		chunks: []string{
			": {\"state\":\"attempt\",\"n\":1,\"of\":2,\"backend\":\"tower-3026\"}\n",
			": {\"state\":\"attempt\",\"n\":2,\"of\":2,\"backend\":\"litellm\"}\n",
			": {\"state\":\"upstream_started\"}\n",
			"data: {\"choices\":[{\"delta\":{\"content\":\"late\"}}]}\n",
			"data: [DONE]\n",
		},
	}
	seen := make([]string, 0, 3)
	choice, err := readModelStream(context.Background(), body, idle, func(b streamHeartbeat) {
		seen = append(seen, b.State)
	})
	if err != nil {
		t.Fatalf("a heartbeating stream was cut: %v", err)
	}
	if choice.Message.Content.Text != "late" {
		t.Errorf("content = %q", choice.Message.Content.Text)
	}
	// The whole read outlives the idle timeout, which is the point.
	if len(seen) != 3 {
		t.Errorf("heartbeats seen = %v, want three states", seen)
	}
	if seen[0] != "attempt" || seen[2] != "upstream_started" {
		t.Errorf("states = %v", seen)
	}
}

// Acceptance 2: a backend sending nothing at all fails at the idle timeout,
// faster than the turn ceiling, and says which failure it was.
func TestASilentBackendFailsAtTheIdleTimeout(t *testing.T) {
	t.Parallel()
	idle := 80 * time.Millisecond
	// Never yields a line, and never returns either. A total deadline would sit
	// on this for the whole turn budget.
	body := &slowReader{gap: 10 * time.Second, chunks: []string{"data: {}\n"}}

	started := time.Now()
	_, err := readModelStream(context.Background(), body, idle, nil)
	elapsed := time.Since(started)

	if !errors.Is(err, ErrModelSilent) {
		t.Fatalf("err = %v, want ErrModelSilent", err)
	}
	if elapsed > time.Second {
		t.Errorf("gave up after %s, which is not faster than a turn ceiling", elapsed)
	}
}

// Acceptance 3, the harness half: the two failures do not share a notice, and
// the retry advice differs because only one of them can work.
func TestSilenceAndTheCeilingAreDifferentSentences(t *testing.T) {
	t.Parallel()
	silent := turnFailureNotice(stageModel, ErrModelSilent)
	ceiling := turnFailureNotice(stageModel, context.DeadlineExceeded)
	if silent == ceiling {
		t.Fatalf("both failures render %q, so a member cannot tell them apart", silent)
	}
	if got := failureCause(ErrModelSilent); got != causeModelSilent {
		t.Errorf("failure cause = %q, want %q", got, causeModelSilent)
	}
	if got := failureCause(context.DeadlineExceeded); got != causeTimeout {
		t.Errorf("the ceiling stopped reporting as a timeout: %q", got)
	}
	for _, notice := range []string{silent, ceiling} {
		if !noticeShape.MatchString(notice) {
			t.Errorf("%q is not a harness notice", notice)
		}
	}
}

// The turn ceiling still ends the read, and reports as itself rather than as
// silence, or the two would collapse back into one failure.
func TestTheTurnCeilingStillEndsAStream(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithTimeout(context.Background(), 40*time.Millisecond)
	defer cancel()
	body := &slowReader{gap: 10 * time.Second, chunks: []string{"data: {}\n"}}

	_, err := readModelStream(ctx, body, time.Minute, nil)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("err = %v, want the ceiling", err)
	}
	if errors.Is(err, ErrModelSilent) {
		t.Error("the ceiling reported as silence, so the distinction is lost")
	}
}

// A comment line that is not JSON is still activity. Refusing to parse one must
// never fail the turn, because the data frames are the contract.
func TestAnUnparsableHeartbeatIsIgnoredRatherThanFatal(t *testing.T) {
	t.Parallel()
	choice, err := readModelStream(context.Background(), streamOf(
		`: not json at all`,
		`: {"state":"attempt","n":1,"of":1}`,
		`data: {"choices":[{"delta":{"content":"ok"}}]}`,
		`data: [DONE]`,
	), time.Second, nil)
	if err != nil {
		t.Fatalf("a bad comment failed the turn: %v", err)
	}
	if choice.Message.Content.Text != "ok" {
		t.Errorf("content = %q", choice.Message.Content.Text)
	}
}

// A stream that ends without [DONE] is still a completed read, because the body
// closing is the other legitimate end.
func TestAStreamEndingWithoutDoneStillAssembles(t *testing.T) {
	t.Parallel()
	choice, err := readModelStream(context.Background(), streamOf(
		`data: {"choices":[{"delta":{"content":"partial"},"finish_reason":"stop"}]}`,
	), time.Second, nil)
	if err != nil {
		t.Fatalf("readModelStream: %v", err)
	}
	if choice.Message.Content.Text != "partial" || choice.FinishReason != "stop" {
		t.Errorf("choice = %#v", choice)
	}
}

// Reasoning is echoed back on the next round, and an unnamed field differs from
// an empty one, so the stream must not invent one.
func TestAStreamWithoutReasoningLeavesTheFieldUnnamed(t *testing.T) {
	t.Parallel()
	choice, err := readModelStream(context.Background(), streamOf(
		`data: {"choices":[{"delta":{"content":"hi"}}]}`,
		`data: [DONE]`,
	), time.Second, nil)
	if err != nil {
		t.Fatalf("readModelStream: %v", err)
	}
	if choice.Message.ReasoningContent != nil {
		t.Errorf("reasoning = %q, want nil for a stream that carried none",
			*choice.Message.ReasoningContent)
	}
}

// Parallel calls sent whole at one index must stay two calls. Joined, they
// became "eco-game__get_storeseco-game__find_trade" (sirens-echo#8071).
func TestWholeCallsSharingAnIndexStaySeparate(t *testing.T) {
	t.Parallel()
	cases := map[string][]string{
		"distinct ids": {
			`data: {"choices":[{"delta":{"tool_calls":[{"index":0,"id":"a","function":{"name":"eco-game__get_stores","arguments":"{}"}}]}}]}`,
			`data: {"choices":[{"delta":{"tool_calls":[{"index":0,"id":"b","function":{"name":"eco-game__find_trade","arguments":"{\"item\":\"iron\"}"}}]}}]}`,
			`data: [DONE]`,
		},
		"no ids": {
			`data: {"choices":[{"delta":{"tool_calls":[{"index":0,"function":{"name":"eco-game__get_stores","arguments":"{}"}}]}}]}`,
			`data: {"choices":[{"delta":{"tool_calls":[{"index":0,"function":{"name":"eco-game__find_trade","arguments":"{}"}}]}}]}`,
			`data: [DONE]`,
		},
	}
	for name, lines := range cases {
		choice, err := readModelStream(context.Background(), streamOf(lines...), time.Second, nil)
		if err != nil {
			t.Fatalf("%s: readModelStream: %v", name, err)
		}
		calls := choice.Message.ToolCalls
		if len(calls) != 2 || calls[0].Function.Name != "eco-game__get_stores" ||
			calls[1].Function.Name != "eco-game__find_trade" {
			t.Fatalf("%s: calls = %+v, want two separate calls in order", name, calls)
		}
	}
}
