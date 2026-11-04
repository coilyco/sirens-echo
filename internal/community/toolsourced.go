package community

import (
	"context"
	"strings"
	"sync"
)

// A name from a tool result is never resolved to a mention (COI-3283).
// See docs/sirens-echo-mentions.md.

// toolSourced collects the text a turn's tools returned or rendered.
type toolSourced struct {
	mu    sync.Mutex
	texts []string
}

type toolSourcedKey struct{}

func withToolSourced(ctx context.Context) (context.Context, *toolSourced) {
	sourced := &toolSourced{}
	return context.WithValue(ctx, toolSourcedKey{}, sourced), sourced
}

// noteToolSourced records tool text on the turn in ctx. A context with no turn
// has nothing to protect, so it records nothing.
func noteToolSourced(ctx context.Context, texts ...string) {
	sourced, ok := ctx.Value(toolSourcedKey{}).(*toolSourced)
	if !ok {
		return
	}
	sourced.mu.Lock()
	sourced.texts = append(sourced.texts, texts...)
	sourced.mu.Unlock()
}

func toolSourcedText(ctx context.Context) string {
	sourced, ok := ctx.Value(toolSourcedKey{}).(*toolSourced)
	if !ok {
		return ""
	}
	sourced.mu.Lock()
	defer sourced.mu.Unlock()
	return strings.Join(sourced.texts, "\n")
}

// withoutNamesIn drops each account with a name in text, matched ignoring case.
// The whole account goes, so one spelling stops the others. Fails closed.
func (r mentionRoster) withoutNamesIn(text string) mentionRoster {
	if len(r) == 0 || strings.TrimSpace(text) == "" {
		return r
	}
	lowered := strings.ToLower(text)
	dropped := make(map[string]bool)
	for name, userID := range r {
		if strings.Contains(lowered, strings.ToLower(name)) {
			dropped[userID] = true
		}
	}
	if len(dropped) == 0 {
		return r
	}
	kept := make(mentionRoster, len(r))
	for name, userID := range r {
		if !dropped[userID] {
			kept[name] = userID
		}
	}
	return kept
}

// resolveReplyMentions is the one place a reply's names become mentions. Both
// the model path and the direct-tool path reach it through send.
func resolveReplyMentions(ctx context.Context, roster mentionRoster, reply string) (string, []string) {
	return roster.withoutNamesIn(toolSourcedText(ctx)).resolveMentions(reply)
}
