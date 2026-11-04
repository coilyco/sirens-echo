package community

import (
	"context"
	"errors"
	"testing"
)

// A blank message reads as the thing being broken while carrying none of the
// information a real failure would (#1035). The send refuses it (COI-3278).

func blankReplyAgent() *Agent {
	return &Agent{telemetry: telemetryOrNoop(nil)}
}

func TestABlankReplyIsNeverSent(t *testing.T) {
	t.Parallel()
	for _, content := range []string{"", " ", "\n", "\t\n  ", " "} {
		turn := &markableTurn{}
		if err := blankReplyAgent().sendReply(t.Context(), turn, content, ""); !errors.Is(err, ErrReplySilent) {
			t.Fatalf("sendReply(%q) returned %v, want ErrReplySilent", content, err)
		}
		if len(turn.replies) != 0 {
			t.Errorf("content %q was sent as %q, want nothing sent", content, turn.replies)
		}
	}
}

// The send no longer marks the message with an acknowledgement in place of the
// answer. That mark read as a reply that never came, and the turn recorded ok.
func TestABlankReplyIsNotPassedOffAsAnAcknowledgement(t *testing.T) {
	t.Parallel()
	turn := &markableTurn{}
	if err := blankReplyAgent().sendReply(t.Context(), turn, "   ", ""); !errors.Is(err, ErrReplySilent) {
		t.Fatalf("sendReply returned %v, want ErrReplySilent", err)
	}
	if marked(turn, replyReactions[blankReplyReaction]) {
		t.Errorf("marks = %v, a blank must not be answered with %q", turn.applied, replyReactions[blankReplyReaction])
	}
}

// quietTurn has no reaction surface, and must still not fall back to sending
// the blank it could not mark.
type quietTurn struct{ replies []string }

func (t *quietTurn) RequestID() string                                  { return "req-quiet" }
func (t *quietTurn) Requester() string                                  { return "member" }
func (t *quietTurn) Transport() string                                  { return transportHTTP }
func (t *quietTurn) Current() TranscriptEntry                           { return TranscriptEntry{} }
func (t *quietTurn) History(context.Context) ([]TranscriptEntry, error) { return nil, nil }

func (t *quietTurn) Reply(_ context.Context, content string) error {
	t.replies = append(t.replies, content)
	return nil
}

func TestATransportThatCannotMarkStillSendsNoBlank(t *testing.T) {
	t.Parallel()
	turn := &quietTurn{}
	if err := blankReplyAgent().sendReply(t.Context(), turn, "", ""); !errors.Is(err, ErrReplySilent) {
		t.Fatalf("sendReply returned %v, want ErrReplySilent", err)
	}
	if len(turn.replies) != 0 {
		t.Errorf("a blank was sent as %q on a transport that cannot mark", turn.replies)
	}
}

// The guard must not swallow real answers, including short ones.
func TestARealReplyIsUnaffected(t *testing.T) {
	t.Parallel()
	turn := &quietTurn{}
	if err := blankReplyAgent().sendReply(t.Context(), turn, "ok", ""); err != nil {
		t.Fatalf("sendReply returned %v", err)
	}
	if len(turn.replies) != 1 || turn.replies[0] != "ok" {
		t.Errorf("replies = %v, want one \"ok\"", turn.replies)
	}
}
