package community

import (
	"context"
	"sync"
)

// How an accepted turn ended, a closed set because a count is taken over it.
// One is logged per turn as turn.terminal. See docs/sirens-echo-worklog.md.
const (
	// terminalReplyDelivered is words that a send returned for.
	terminalReplyDelivered = "reply_delivered"
	// terminalReactionOnly is a mark on the member's message that was the whole answer.
	terminalReactionOnly = "reaction_only"
	// terminalChosenSilence is a model that answered through a tool and left a mark.
	terminalChosenSilence = "chosen_silence"
	// terminalFailureNotice is a visible failure: the member was told it failed.
	terminalFailureNotice = "failure_notice"
	// terminalFailureUnnotified is a failure whose notice did not land either.
	terminalFailureUnnotified = "failure_unnotified"
	// terminalShutdownDrain is a turn a restart took, which the boot sweep reports.
	terminalShutdownDrain = "shutdown_drain"
	// terminalPanicked is a turn that crashed. handleMessage sends the crash notice.
	terminalPanicked = "panicked"
	// terminalUnresolved is a turn that returned having recorded no ending. It is
	// the defect this set exists to expose, so a count of it should be zero.
	terminalUnresolved = "unresolved"
)

// abnormalTerminal reports an ending that is a defect rather than an outcome.
func abnormalTerminal(kind string) bool {
	return kind == terminalPanicked || kind == terminalUnresolved
}

// turnEnding is the last ending a turn recorded. Last write wins, so a failure
// notice sent through the delivering send still leaves the failure standing.
type turnEnding struct {
	mu   sync.Mutex
	kind string
}

type turnEndingKey struct{}

func withTurnEnding(ctx context.Context) (context.Context, *turnEnding) {
	ending := &turnEnding{}
	return context.WithValue(ctx, turnEndingKey{}, ending), ending
}

func (e *turnEnding) get() string {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.kind
}

// endTurnAs records how the turn in ctx ended. A context with no turn is a
// send outside a turn, which has no ending to record.
func endTurnAs(ctx context.Context, kind string) {
	ending, ok := ctx.Value(turnEndingKey{}).(*turnEnding)
	if !ok {
		return
	}
	ending.mu.Lock()
	ending.kind = kind
	ending.mu.Unlock()
}

// endTurnUnlessFailed records a success ending only when the call it follows
// succeeded, so a failed send leaves the failure it recorded standing.
func endTurnUnlessFailed(ctx context.Context, err error, kind string) {
	if err == nil {
		endTurnAs(ctx, kind)
	}
}

// failureEnding names a failure by whether its notice reached the member.
func failureEnding(noticeErr error) string {
	if noticeErr != nil {
		return terminalFailureUnnotified
	}
	return terminalFailureNotice
}
