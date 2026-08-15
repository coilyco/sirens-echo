package community

import (
	"context"
	"fmt"
)

// The outbound hop was the one place a large payload was destroyed rather than
// saved. See docs/sirens-echo-reply-overflow.md.

const (
	// overflowNotice names the file and the true size, the way the inbound and
	// tool-result paths already name theirs.
	overflowNotice = "\n[the whole reply is attached as %s, %d bytes]"
	// overflowFileName is fixed, so nothing member-supplied or model-supplied
	// reaches an attachment name.
	overflowFileName = "reply.txt"
	// nothingWithheld is what a send carries when the message is all of it.
	nothingWithheld = ""
	// prefillNotice names context the turn could not carry, so a thin answer is
	// attributable. See docs/sirens-echo-thread-prefill.md.
	prefillNotice = "\n[thread context truncated, %d older messages dropped]"
)

// prefillReporter is an optional turn capability, asserted like the reactor. A
// transport whose prefill can drop context declares it.
type prefillReporter interface {
	PrefillDropped() int
}

// droppedPrefill reads what a turn's prefill could not carry. A transport that
// never drops any reports nothing and renders exactly as it does today.
func droppedPrefill(turn turnIO) int {
	reporter, ok := turn.(prefillReporter)
	if !ok {
		return 0
	}
	return reporter.PrefillDropped()
}

// prefillTruncationNotice is empty when the prefill carried everything, so a
// turn that dropped nothing is byte-identical to today.
func prefillTruncationNotice(dropped int) string {
	if dropped <= 0 {
		return ""
	}
	return fmt.Sprintf(prefillNotice, dropped)
}

// overflowCarrier is an optional turn capability, asserted like the reactor: a
// transport that can carry text its message budget cut declares it.
type overflowCarrier interface {
	ReplyWithOverflow(ctx context.Context, content, whole string) error
}

// deliverWithOverflow attaches the whole reply where the transport can carry
// it, and falls back to the cut message where it cannot.
func deliverWithOverflow(ctx context.Context, turn turnIO, content, whole string) error {
	carrier, ok := turn.(overflowCarrier)
	if !ok || whole == nothingWithheld {
		return turn.Reply(ctx, content)
	}
	return carrier.ReplyWithOverflow(ctx, content, whole)
}

// fitWithOverflow returns the message to send and the whole reply that message
// is a prefix of. The second return is empty when nothing was cut.
func fitWithOverflow(
	answer string, limit int, executed []ExecutedTool, dropped int,
) (content, whole string) {
	// Reserved before the answer is fitted, because a notice that vanishes
	// under load reads as context that was never missing.
	missing := prefillTruncationNotice(dropped)
	budget := limit - runeLen(missing)
	whole = AssembleReply(answer, unboundedReply, executed...) + missing
	if limit <= 0 || runeLen(whole) <= limit {
		return whole, nothingWithheld
	}
	notice := fmt.Sprintf(overflowNotice, overflowFileName, len(whole))
	room := budget - runeLen(notice)
	// Too large to attach, or too small a budget to say so. Both fall back to
	// the previous behaviour rather than failing the turn.
	if len(whole) > replyAttachmentBytes || room <= 0 {
		return AssembleReply(answer, budget, executed...) + missing, nothingWithheld
	}
	return AssembleReply(answer, room, executed...) + missing + notice, whole
}
