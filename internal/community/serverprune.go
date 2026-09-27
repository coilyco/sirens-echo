package community

import "context"

// droppedServersKey carries route.jev's server verdict to the answer's
// Complete call only. See docs/sirens-echo-tools.md.
type droppedServersKey struct{}

func withDroppedServers(ctx context.Context, names []string) context.Context {
	if len(names) == 0 {
		return ctx
	}
	set := make(map[string]bool, len(names))
	for _, name := range names {
		set[name] = true
	}
	return context.WithValue(ctx, droppedServersKey{}, set)
}

// droppedServersFrom is nil, which drops nothing, when no verdict was attached.
func droppedServersFrom(ctx context.Context) map[string]bool {
	set, _ := ctx.Value(droppedServersKey{}).(map[string]bool)
	return set
}

// replyRequiredKey marks a turn whose caller asked directly and has no other
// channel, where silence is a blank, not a choice. See sirens-echo#8326.
type replyRequiredKey struct{}

// withReplyRequired marks every transport but Discord, which alone can express
// a chosen silence by posting nothing.
func withReplyRequired(ctx context.Context, transport string) context.Context {
	if transport == transportDiscord {
		return ctx
	}
	return context.WithValue(ctx, replyRequiredKey{}, true)
}

func replyRequiredFrom(ctx context.Context) bool {
	required, _ := ctx.Value(replyRequiredKey{}).(bool)
	return required
}

// blankForCaller reports an empty reply to a caller that asked directly, from a
// turn whose calls only read. A write may have been the answer (#895).
func blankForCaller(ctx context.Context, reply string, executed []ExecutedTool) bool {
	if reply != "" || !replyRequiredFrom(ctx) {
		return false
	}
	for _, call := range executed {
		if !call.ReadOnly {
			return false
		}
	}
	return true
}
