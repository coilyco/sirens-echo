package community

import (
	"context"
	"testing"
)

// answeredThroughToolClient is the shape a write tool aimed at the reply
// channel produces: the model answered through it and returned nothing.
type answeredThroughToolClient struct{}

func (answeredThroughToolClient) Complete(
	context.Context, TurnPrompt, string,
) (CompletionResult, error) {
	return CompletionResult{
		Content: "",
		ToolCalls: []ExecutedTool{{
			Name: "demo-discord__create_channel-message", Result: "sent", Outcome: ToolOutcomeOK,
		}},
	}, nil
}

func silentTurnAgent(client CompletionClient) *Agent {
	agent := &Agent{
		cfg:          Config{Definition: Definition{MaxContextMessages: 12}},
		completions:  client,
		systemPrompt: "neutral model policy and local knowledge",
		telemetry:    telemetryOrNoop(nil),
		slots:        make(chan struct{}, 1),
	}
	agent.ensureRuntimeDefaults()
	return agent
}

// A turn that already spoke through a tool does not answer twice (#895), and
// marks the message rather than end silent (#8364).
func TestATurnThatAlreadyAnsweredThroughAToolMarksTheMessage(t *testing.T) {
	t.Parallel()
	agent := silentTurnAgent(answeredThroughToolClient{})
	turn := &markableTurn{}

	if err := agent.runTurn(context.Background(), turn, nil); err != nil {
		t.Fatalf("runTurn: %v", err)
	}
	if len(turn.replies) != 0 {
		t.Errorf("the harness posted %q after the model had already spoken", turn.replies)
	}
	if !marked(turn, replyReactions[blankReplyReaction]) {
		t.Errorf("the turn ended with no reply and no mark, applied = %q", turn.applied)
	}
}

// A transport that cannot mark gets the glyph and the key the caller reads.
func TestAnUnmarkableTurnThatAnsweredThroughAToolReportsTheReaction(t *testing.T) {
	t.Parallel()
	agent := silentTurnAgent(answeredThroughToolClient{})
	turn := &httpTurn{requestID: "silent-turn", current: TranscriptEntry{
		Author: "member", Content: "what is the vibe check?",
	}}

	if err := agent.runTurn(context.Background(), turn, nil); err != nil {
		t.Fatalf("runTurn: %v", err)
	}
	if turn.reaction != blankReplyReaction {
		t.Errorf("reaction = %q, want %q", turn.reaction, blankReplyReaction)
	}
	if turn.reply != replyReactions[blankReplyReaction] {
		t.Errorf("reply = %q, want the %s glyph", turn.reply, blankReplyReaction)
	}
}

// readOnlyBlankClient reads and then says nothing, the Discord shape of the
// probes that came back blank over /v1/turn (sirens-echo#8326, #8328).
type readOnlyBlankClient struct{}

func (readOnlyBlankClient) Complete(
	context.Context, TurnPrompt, string,
) (CompletionResult, error) {
	return CompletionResult{
		Content: "",
		ToolCalls: []ExecutedTool{{
			Name: "eco__get_market", Result: "iron 3", Outcome: ToolOutcomeOK, ReadOnly: true,
		}},
	}, nil
}

// On Discord a blank after reads only is never silence: the member gets words.
func TestADiscordTurnThatOnlyReadAndSaidNothingIsNotSilent(t *testing.T) {
	t.Parallel()
	agent := silentTurnAgent(readOnlyBlankClient{})
	turn := &markableTurn{}

	_ = agent.runTurn(context.Background(), turn, nil)
	if len(turn.replies) == 0 || turn.replies[len(turn.replies)-1] == "" {
		t.Errorf("a Discord turn after reads only ended silent, replies = %q", turn.replies)
	}
}

// Silence stays distinguishable from a turn that simply produced nothing, which
// is the condition the decision reserved the right to reopen on.
func TestATurnThatDidNothingAtAllIsStillAFailure(t *testing.T) {
	t.Parallel()
	agent := silentTurnAgent(answeringClient{reply: ""})
	turn := &httpTurn{requestID: "empty-turn", current: TranscriptEntry{
		Author: "member", Content: "what is the vibe check?",
	}}

	err := agent.runTurn(context.Background(), turn, nil)
	if err == nil && turn.reply == "" {
		t.Error("a turn that ran no tool and said nothing passed as chosen silence")
	}
}
