package community

import (
	"context"
	"fmt"
	"log/slog"
	"regexp"
	"sort"
	"strings"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

// Some answers are a mark on the member's own message rather than a message of
// their own. See docs/sirens-echo-progress.md.

// reactInvocation is what the model writes to answer with a mark. The same
// shape a phrase takes, so one alphabet covers both.
var reactInvocation = regexp.MustCompile(`\{\{react:([^}]*)\}\}`)

// replyReactions is the closed set a reply may invoke, keyed rather than
// spelled. Disjoint from the harness marks. See docs/sirens-echo-phrases.md.
var replyReactions = map[string]string{
	"agree":       "\U0001F44D",   // 👍
	"disagree":    "\U0001F44E",   // 👎
	"acknowledge": "\u2705",       // ✅
	"wave":        "\U0001F44B",   // 👋
	"heart":       "\u2764\uFE0F", // ❤️
	"laugh":       "\U0001F602",   // 😂
	"celebrate":   "\U0001F389",   // 🎉
	"sad":         "\U0001F622",   // 😢
}

// reactionMeanings tells the model and Jev what each key answers, so the glyph
// is picked for its meaning rather than guessed from its name.
var reactionMeanings = map[string]string{
	"agree":       "yes, correct, confirmed, or it is up",
	"disagree":    "no, incorrect, or it is down",
	"acknowledge": "noted, done, or okay",
	"wave":        "a greeting or a goodbye",
	"heart":       "thanks or appreciation",
	"laugh":       "a joke or something funny",
	"celebrate":   "good news or an achievement",
	"sad":         "bad news or a loss",
}

// snapReactions may answer before the model runs, because none needs a lookup.
// agree and disagree state a fact, so a tool must answer first. sirens-echo#8161.
var snapReactions = map[string]bool{
	"acknowledge": true,
	"wave":        true,
	"heart":       true,
	"laugh":       true,
	"celebrate":   true,
	"sad":         true,
}

// reactKeys names every key, for the prompt that tells the model what it may
// invoke. Sorted, so the prompt is the same string on every boot.
func reactKeys() []string {
	keys := make([]string, 0, len(replyReactions))
	for key := range replyReactions {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

// reactInvoked reports whether a reply carries an invocation at all, so an
// ordinary reply is left untouched.
func reactInvoked(reply string) bool { return reactInvocation.MatchString(reply) }

// reactTerminal reports whether one invocation is the whole reply. Exactly one,
// because two marks is not a mark and a mark beside prose is prose.
func reactTerminal(reply string) bool {
	if len(reactInvocation.FindAllStringIndex(reply, -1)) != 1 {
		return false
	}
	return strings.TrimSpace(reactInvocation.ReplaceAllString(reply, "")) == ""
}

// resolveReaction turns an invocation into its key and glyph. An unknown key
// is an error rather than a marker a member reads.
func (a *Agent) resolveReaction(ctx context.Context, reply string) (string, string, error) {
	if !reactTerminal(reply) {
		return "", "", fmt.Errorf("reply invokes a reaction alongside other text")
	}
	key := strings.TrimSpace(reactInvocation.FindStringSubmatch(reply)[1])
	glyph, known := replyReactions[key]
	if !known {
		return "", "", fmt.Errorf("reply invokes unknown reaction %q", key)
	}
	// The key set is closed and service-authored, so it is safe as a label where
	// a member-supplied value would not be. See docs/sirens-echo-admission.md.
	a.telemetry.Info(ctx, "response.reaction.invoked", slog.String("reaction.key", key))
	trace.SpanFromContext(ctx).SetAttributes(attribute.String("response.reaction", key))
	return key, glyph, nil
}

// reactionRecorder is a transport that reports which key answered, since it
// cannot place a mark and its caller cannot tell a glyph reply from a mark.
type reactionRecorder interface {
	RecordReaction(key string)
}

func recordReaction(turn turnIO, key string) {
	if recorder, ok := turn.(reactionRecorder); ok {
		recorder.RecordReaction(key)
	}
}

// finishWithSnap answers a social turn with a mark before any model call, on
// route.jev's shape verdict. See docs/sirens-echo-phrases.md.
func (a *Agent) finishWithSnap(
	ctx context.Context,
	turn turnIO,
	progress *turnProgress,
	key string,
) error {
	glyph := replyReactions[key]
	a.telemetry.Info(ctx, "response.reaction.snapped", slog.String("reaction.key", key))
	trace.SpanFromContext(ctx).SetAttributes(
		attribute.String("response.reaction", key),
		attribute.String("response.reaction.path", "jev"),
	)
	recordReaction(turn, key)
	if target, markable := turn.(reactor); markable {
		return a.finishByReacting(ctx, turn, progress, target, glyph)
	}
	if err := a.deliverOrReport(ctx, turn, glyph, nothingWithheld); err != nil {
		return err
	}
	a.clearTurnMarks(ctx)
	a.beats.reply()
	return nil
}

// finishByReacting ends a turn whose whole answer is a mark. The mark is the
// delivery here, so a failed one falls back to words rather than being dropped.
func (a *Agent) finishByReacting(
	ctx context.Context,
	turn turnIO,
	progress *turnProgress,
	target reactor,
	glyph string,
) error {
	// A line that just went up should be readable before the answer lands, the
	// same courtesy a sent reply gets. See docs/sirens-echo-progress.md.
	a.settleWithSpan(ctx, progress.settleDelay(), progress.Settle)
	if err := turnReactor(ctx, target).React(ctx, glyph); err != nil {
		a.telemetry.Info(
			ctx,
			"response.reaction.undelivered",
			slog.String("reaction", glyph),
			slog.String("error", err.Error()),
		)
		// Likeliest a missing ADD_REACTIONS permission, which must not cost the
		// member the answer. The glyph it stood for is sent as the reply.
		if err := a.deliverOrReport(ctx, turn, glyph, nothingWithheld); err != nil {
			return err
		}
		a.clearTurnMarks(ctx)
		a.beats.reply()
		return nil
	}
	a.telemetry.Info(ctx, "turn.reply.reaction", slog.String("reaction", glyph))
	// The mark is the outcome, so nothing is left to describe work in flight.
	a.clearTurnMarks(ctx)
	a.beats.reply()
	endTurnAs(ctx, terminalReactionOnly)
	return nil
}

// withReactionPolicy names the keys a reply may invoke. Appended for every
// deployment, because the set is compiled in rather than configured.
func withReactionPolicy(prompt string) string {
	keys := make([]string, 0, len(replyReactions))
	for _, key := range reactKeys() {
		keys = append(keys, key+" ("+reactionMeanings[key]+")")
	}
	return prompt + "\n" + fmt.Sprintf(
		`Some answers are a reaction on the member's own message rather than a reply.
Invoke one by writing {{react:key}} and nothing else, because an invocation is
the whole reply and a reaction beside other text is refused. Prefer one whenever
a single key carries the entire answer: a greeting, thanks, a joke, news, or a
yes or no question once any tool has settled it. Answer in words when the
member needs an explanation, a list, steps, or a number. Available keys: %s.`,
		strings.Join(keys, "; "),
	) + "\n"
}

// blankReplyReaction is the mark a turn leaves when it chose silence after a tool
// answered. A blank reply is a failure now. See docs/sirens-echo-worklog.md.
const blankReplyReaction = "acknowledge"
