package community

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"go.opentelemetry.io/otel/attribute"
)

const transportSlack = "slack"

// slackNames holds display names in memory for slackNameCacheTTL. Slack's API
// terms bar long-term storage of what its API returns, so nothing is written.
type slackNames struct {
	api  slackAPI
	now  func() time.Time
	mu   sync.Mutex
	held map[string]heldSlackName
}

type heldSlackName struct {
	name string
	at   time.Time
}

func newSlackNames(api slackAPI) *slackNames {
	return &slackNames{api: api, now: time.Now, held: make(map[string]heldSlackName)}
}

// Name returns a display name, or "" when Slack cannot name the user.
func (n *slackNames) Name(ctx context.Context, userID string) string {
	if userID == "" {
		return ""
	}
	n.mu.Lock()
	entry, ok := n.held[userID]
	fresh := ok && n.now().Sub(entry.at) < slackNameCacheTTL
	n.mu.Unlock()
	if fresh {
		return entry.name
	}
	name, err := n.api.DisplayName(ctx, userID)
	if err != nil {
		return ""
	}
	name = cleanTranscriptText(name, 80)
	n.mu.Lock()
	// Bounded, so a workspace of many members cannot grow it without limit.
	if len(n.held) >= 512 {
		n.held = make(map[string]heldSlackName)
	}
	n.held[userID] = heldSlackName{name: name, at: n.now()}
	n.mu.Unlock()
	return name
}

// slackTurn is one summon from Slack. It reads what the turn needs from Slack
// and keeps none of it.
type slackTurn struct {
	api       slackAPI
	names     *slackNames
	botUserID string
	team      string
	channel   string
	user      string
	ts        string
	// threadTS is the parent of the thread the summon arrived in, empty for a
	// top-level message.
	threadTS string
	text     string
	direct   bool
	limit    int
}

// replyThread is where the reply goes: the thread the summon is in, a new
// thread under a channel mention, or inline in a direct message.
func (t *slackTurn) replyThread() string {
	if t.threadTS != "" {
		return t.threadTS
	}
	if t.direct {
		return ""
	}
	return t.ts
}

func (t *slackTurn) RequestID() string { return "slack:" + t.channel + ":" + t.ts }

func (t *slackTurn) Requester() string { return "slack:" + t.user }

func (t *slackTurn) Transport() string { return transportSlack }

func (t *slackTurn) resolve(ctx context.Context) func(string) string {
	return func(id string) string { return t.names.Name(ctx, id) }
}

func (t *slackTurn) Current() TranscriptEntry {
	ctx := context.Background()
	author := t.names.Name(ctx, t.user)
	if author == "" {
		author = "member"
	}
	return TranscriptEntry{
		Author:      author,
		Content:     plainSlackText(stripLeadingMention(t.text, t.botUserID), t.resolve(ctx)),
		Counterpart: CounterpartHuman,
		Comments:    1,
	}
}

// History reads the thread this summon is in, or the messages before it in the
// channel, each turn, and drops them with the turn.
func (t *slackTurn) History(ctx context.Context) ([]TranscriptEntry, error) {
	var messages []slackMessage
	var err error
	if t.threadTS != "" {
		messages, err = t.api.Replies(ctx, t.channel, t.threadTS, t.limit+1)
	} else {
		messages, err = t.api.History(ctx, t.channel, t.ts, t.limit)
		// History returns newest first and a transcript reads oldest first.
		for i, j := 0, len(messages)-1; i < j; i, j = i+1, j-1 {
			messages[i], messages[j] = messages[j], messages[i]
		}
	}
	if err != nil {
		return nil, err
	}
	entries := make([]TranscriptEntry, 0, len(messages))
	for _, message := range messages {
		if message.TS == t.ts || message.TS > t.ts || !readableSlackMessage(message) {
			continue
		}
		counterpart, author := CounterpartHuman, t.names.Name(ctx, message.User)
		if message.BotID != "" || message.User == t.botUserID {
			counterpart = CounterpartAgent
		}
		if author == "" {
			author = "member"
		}
		entries = append(entries, TranscriptEntry{
			Author:      author,
			Content:     plainSlackText(message.Text, t.resolve(ctx)),
			Counterpart: counterpart,
		})
	}
	if len(entries) > t.limit {
		entries = entries[len(entries)-t.limit:]
	}
	return entries, nil
}

// readableSlackMessage skips joins, topic changes and the other system rows,
// which carry a subtype and no member words.
func readableSlackMessage(message slackMessage) bool {
	return strings.TrimSpace(message.Text) != "" &&
		(message.SubType == "" || message.SubType == "thread_broadcast" || message.SubType == "bot_message")
}

func (t *slackTurn) Reply(ctx context.Context, content string) error {
	_, err := t.api.Post(ctx, t.channel, t.replyThread(), escapeSlack(content))
	return err
}

func (t *slackTurn) React(ctx context.Context, glyph string) error {
	name, ok := slackReactionName(glyph)
	if !ok {
		return fmt.Errorf("no Slack name for reaction %q", glyph)
	}
	return t.api.React(ctx, t.channel, t.ts, name)
}

func (t *slackTurn) Unreact(ctx context.Context, glyph string) error {
	name, ok := slackReactionName(glyph)
	if !ok {
		return nil
	}
	return t.api.Unreact(ctx, t.channel, t.ts, name)
}

func (t *slackTurn) ReplyLimit() int { return slackReplyLimit }

// SpanAttributes places a turn by opaque ids and never by content. A direct
// message contributes nothing, as it does on Discord.
func (t *slackTurn) SpanAttributes() []attribute.KeyValue {
	if t.direct {
		return nil
	}
	return []attribute.KeyValue{
		attribute.String("slack.team.id", t.team),
		attribute.String("slack.channel.id", t.channel),
		attribute.String("slack.user.id", t.user),
	}
}

func (t *slackTurn) LocationLabel() string {
	if t.direct {
		return "a direct message"
	}
	return "a Slack channel"
}

// SessionID shares a workspace with everyone in the thread, and pairs the
// channel with the requester outside one.
func (t *slackTurn) SessionID() SessionID {
	if t.threadTS != "" {
		return ThreadSession("slack:" + t.channel + ":" + t.threadTS)
	}
	return DirectSession("slack:"+t.channel, t.Requester())
}

func (t *slackTurn) ProgressSink() TurnProgressSink {
	return slackProgress{api: t.api, channel: t.channel, thread: t.replyThread()}
}

// slackProgress narrates a long turn as one message edited in place. The rich
// worklog is not built, so the notice lines are the whole surface.
type slackProgress struct {
	api     slackAPI
	channel string
	thread  string
}

func (p slackProgress) Post(ctx context.Context, notice string) (string, error) {
	return p.api.Post(ctx, p.channel, p.thread, escapeSlack(notice))
}

func (p slackProgress) Edit(ctx context.Context, ts, notice string) error {
	return p.api.Update(ctx, p.channel, ts, escapeSlack(notice))
}

func (p slackProgress) Delete(ctx context.Context, ts string) error {
	return p.api.Delete(ctx, p.channel, ts)
}

var (
	_ turnIO               = (*slackTurn)(nil)
	_ reactor              = (*slackTurn)(nil)
	_ unreactor            = (*slackTurn)(nil)
	_ replyBudget          = (*slackTurn)(nil)
	_ spanTagger           = (*slackTurn)(nil)
	_ progressSinkProvider = (*slackTurn)(nil)
	_ turnLocated          = (*slackTurn)(nil)
	_ sessionOwner         = (*slackTurn)(nil)
	_ TurnProgressSink     = slackProgress{}
)
