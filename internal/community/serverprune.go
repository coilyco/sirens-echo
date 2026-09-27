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

// blankForCaller reports an empty reply from a turn whose calls only read. Every
// turn is a summon, and only a call that may have spoken earns silence (#895, #8326).
func blankForCaller(reply string, executed []ExecutedTool) bool {
	if reply != "" {
		return false
	}
	for _, call := range executed {
		if !call.ReadOnly {
			return false
		}
	}
	return true
}
