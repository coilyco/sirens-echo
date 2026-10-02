package community

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

func newSlackTurn(api *fakeSlack, mutate func(*slackTurn)) *slackTurn {
	turn := &slackTurn{
		api:       api,
		names:     newSlackNames(api),
		botUserID: "U0BOT0001",
		team:      "T0COILYCO",
		channel:   "C0GENERAL",
		user:      "U0KAI0001",
		ts:        "1700000000.000500",
		text:      "<@U0BOT0001> what changed?",
		limit:     3,
	}
	if mutate != nil {
		mutate(turn)
	}
	return turn
}

func TestACurrentMessageReadsAsTheMemberTypedIt(t *testing.T) {
	t.Parallel()
	entry := newSlackTurn(newFakeSlack(), nil).Current()
	if entry.Author != "Kai" || entry.Content != "what changed?" || entry.Counterpart != CounterpartHuman {
		t.Fatalf("entry = %+v", entry)
	}
	nameless := newSlackTurn(newFakeSlack(), func(turn *slackTurn) { turn.user = "U0NOBODY1" }).Current()
	if nameless.Author != "member" {
		t.Fatalf("an unnameable user must read as a member, got %q", nameless.Author)
	}
}

func TestChannelHistoryReadsOldestFirstAndStopsBeforeTheSummon(t *testing.T) {
	t.Parallel()
	api := newFakeSlack()
	// conversations.history is newest first.
	api.history = []slackMessage{
		{TS: "1700000000.000400", User: "U0KAI0001", Text: "fourth"},
		{TS: "1700000000.000300", User: "U0BOT0001", Text: "third, from the bot"},
		{TS: "1700000000.000250", User: "U0KAI0001", SubType: "channel_join", Text: "joined"},
		{TS: "1700000000.000200", User: "U0KAI0001", Text: "second"},
		{TS: "1700000000.000100", User: "U0KAI0001", Text: "first"},
	}
	entries, err := newSlackTurn(api, nil).History(context.Background())
	if err != nil {
		t.Fatalf("history: %v", err)
	}
	var got []string
	for _, entry := range entries {
		got = append(got, entry.Content)
	}
	if want := "second|third, from the bot|fourth"; strings.Join(got, "|") != want {
		t.Fatalf("history = %v, want the last 3 readable messages oldest first (%s)", got, want)
	}
	if entries[1].Counterpart != CounterpartAgent {
		t.Fatalf("the bot's own earlier reply must read as an agent, got %q", entries[1].Counterpart)
	}
}

func TestThreadHistoryReadsTheThreadAndNotTheSummonOrAnythingAfterIt(t *testing.T) {
	t.Parallel()
	api := newFakeSlack()
	// conversations.replies is oldest first and includes the parent and the summon.
	api.replies = []slackMessage{
		{TS: "1700000000.000100", User: "U0KAI0001", Text: "parent"},
		{TS: "1700000000.000200", User: "U0BOT0001", Text: "earlier answer"},
		{TS: "1700000000.000500", User: "U0KAI0001", Text: "the summon itself"},
		{TS: "1700000000.000600", User: "U0KAI0001", Text: "arrived later"},
	}
	turn := newSlackTurn(api, func(t *slackTurn) { t.threadTS = "1700000000.000100" })
	entries, err := turn.History(context.Background())
	if err != nil {
		t.Fatalf("history: %v", err)
	}
	if len(entries) != 2 || entries[0].Content != "parent" || entries[1].Content != "earlier answer" {
		t.Fatalf("entries = %+v", entries)
	}
}

func TestAHistoryReadFailureFailsTheTurnsReadAndStoresNothing(t *testing.T) {
	t.Parallel()
	api := newFakeSlack()
	api.readErr = errors.New("ratelimited")
	if _, err := newSlackTurn(api, nil).History(context.Background()); err == nil {
		t.Fatal("a failed read must surface so the turn path can report it")
	}
}

func TestRepliesLandWhereTheConversationIs(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name   string
		mutate func(*slackTurn)
		want   string
	}{
		{"a channel mention opens a thread under it", nil, "1700000000.000500"},
		{"a mention inside a thread stays in it", func(t *slackTurn) { t.threadTS = "1700000000.000100" }, "1700000000.000100"},
		{"a direct message is answered inline", func(t *slackTurn) { t.direct = true }, ""},
		{"a direct message thread stays in it", func(t *slackTurn) { t.direct, t.threadTS = true, "1700000000.000100" }, "1700000000.000100"},
	}
	for _, tc := range cases {
		api := newFakeSlack()
		if err := newSlackTurn(api, tc.mutate).Reply(context.Background(), "ok"); err != nil {
			t.Fatalf("%s: %v", tc.name, err)
		}
		if got := api.postsSnapshot()[0].Thread; got != tc.want {
			t.Errorf("%s: thread = %q, want %q", tc.name, got, tc.want)
		}
	}
}

func TestEveryReplyAndProgressLineIsEscapedBeforeItIsSent(t *testing.T) {
	t.Parallel()
	api := newFakeSlack()
	turn := newSlackTurn(api, nil)
	if err := turn.Reply(context.Background(), "ping <!channel> a&b"); err != nil {
		t.Fatalf("reply: %v", err)
	}
	sink := turn.ProgressSink()
	ts, err := sink.Post(context.Background(), "reading <@U0KAI0001>")
	if err != nil {
		t.Fatalf("post: %v", err)
	}
	if err := sink.Edit(context.Background(), ts, "thinking <!here>"); err != nil {
		t.Fatalf("edit: %v", err)
	}
	if err := sink.Delete(context.Background(), ts); err != nil {
		t.Fatalf("delete: %v", err)
	}
	var sent []string
	for _, post := range api.postsSnapshot() {
		sent = append(sent, post.Text)
	}
	sent = append(sent, api.updates...)
	for _, text := range sent {
		if strings.ContainsAny(text, "<>") {
			t.Errorf("unescaped text reached Slack: %q", text)
		}
	}
	if len(api.deletes) != 1 {
		t.Fatalf("deletes = %v", api.deletes)
	}
}

func TestReactionsUseSlackNamesAndAnUnmappedGlyphIsAnErrorNotAGuess(t *testing.T) {
	t.Parallel()
	api := newFakeSlack()
	turn := newSlackTurn(api, nil)
	if err := turn.React(context.Background(), reactionAccepted); err != nil {
		t.Fatalf("react: %v", err)
	}
	if err := turn.Unreact(context.Background(), reactionAccepted); err != nil {
		t.Fatalf("unreact: %v", err)
	}
	if api.reactions[0] != "1700000000.000500:eyes" || api.removed[0] != "1700000000.000500:eyes" {
		t.Fatalf("reactions = %v removed = %v", api.reactions, api.removed)
	}
	if err := turn.React(context.Background(), "\U0001F9FF"); err == nil {
		t.Fatal("a glyph with no Slack name must be reported so the turn path falls back to words")
	}
	if err := turn.Unreact(context.Background(), "\U0001F9FF"); err != nil {
		t.Fatalf("removing a mark that was never placed is not an error: %v", err)
	}
}

func TestASlackTurnSharesAWorkspaceInAThreadAndPairsItOutside(t *testing.T) {
	t.Parallel()
	inThread := newSlackTurn(newFakeSlack(), func(t *slackTurn) { t.threadTS = "1700000000.000100" }).SessionID()
	other := newSlackTurn(newFakeSlack(), func(t *slackTurn) {
		t.threadTS, t.user = "1700000000.000100", "U0OTHER01"
	}).SessionID()
	if inThread != other {
		t.Fatal("two members in one thread must share a workspace")
	}
	alone := newSlackTurn(newFakeSlack(), nil).SessionID()
	elsewhere := newSlackTurn(newFakeSlack(), func(t *slackTurn) { t.user = "U0OTHER01" }).SessionID()
	if alone == elsewhere || !alone.Valid() {
		t.Fatal("outside a thread, two members must not share a workspace")
	}
}

func TestADirectMessageContributesNoIdentifiersToTheTurnSpan(t *testing.T) {
	t.Parallel()
	if got := newSlackTurn(newFakeSlack(), func(t *slackTurn) { t.direct = true }).SpanAttributes(); got != nil {
		t.Fatalf("a direct message must contribute nothing, got %v", got)
	}
	if got := newSlackTurn(newFakeSlack(), nil).SpanAttributes(); len(got) != 3 {
		t.Fatalf("a channel turn carries team, channel and user ids, got %v", got)
	}
}

func TestADisplayNameIsHeldBrieflyAndOnlyInMemory(t *testing.T) {
	t.Parallel()
	api := newFakeSlack()
	names := newSlackNames(api)
	now := time.Unix(1_700_000_000, 0)
	names.now = func() time.Time { return now }
	ctx := context.Background()

	if got := names.Name(ctx, "U0KAI0001"); got != "Kai" {
		t.Fatalf("name = %q", got)
	}
	api.names["U0KAI0001"] = "Renamed"
	if got := names.Name(ctx, "U0KAI0001"); got != "Kai" {
		t.Fatalf("a fresh entry must be served from memory, got %q", got)
	}
	now = now.Add(slackNameCacheTTL + time.Second)
	if got := names.Name(ctx, "U0KAI0001"); got != "Renamed" {
		t.Fatalf("an expired entry must be read again, got %q", got)
	}
}
