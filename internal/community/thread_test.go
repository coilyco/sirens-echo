package community

import (
	"strings"
	"testing"

	"github.com/bwmarrin/discordgo"
)

// A long answer gets somewhere of its own, and a thread that cannot be made
// must never cost a member their reply. See sirens-echo#239.

// The defect in #1036: seven THREAD_CREATED notices in #moxn-temporal carried
// another member's sentence cut mid-word, attributed to the agent.
func TestAThreadNameCarriesNoMemberText(t *testing.T) {
	t.Parallel()
	member := "feed cracked 104 exhibitors full roster attached endpoint for the thing"
	name := threadCreationName()
	for _, word := range []string{"feed", "cracked", "exhibitors", "roster", "endpoint"} {
		if strings.Contains(name, word) {
			t.Errorf("thread name %q carries %q from the summoning message", name, word)
		}
	}
	if strings.Contains(member, name) {
		t.Errorf("thread name %q is a substring of the member's message", name)
	}
}

// The name is the same whatever was asked, because the harness authors it.
func TestAThreadNameIsTheSameForEveryTurn(t *testing.T) {
	t.Parallel()
	if first, second := threadCreationName(), threadCreationName(); first != second {
		t.Errorf("two threads got %q and %q, want one authored name", first, second)
	}
	if threadCreationName() == "" {
		t.Error("a thread would be created with no name")
	}
}

func TestAThreadNameStaysInsideDiscordsCap(t *testing.T) {
	t.Parallel()
	if got := len([]rune(threadCreationName())); got > threadTitleRunes {
		t.Errorf("thread name is %d runes, want at most %d", got, threadTitleRunes)
	}
}

func TestAThreadThatCannotBeMadeCostsNothing(t *testing.T) {
	t.Parallel()
	if _, threaded := threadForReply(nil, &discordgo.Message{ID: "1"}); threaded {
		t.Error("a nil session reported a thread")
	}
	session := statefulSession(t)
	if _, threaded := threadForReply(session, nil); threaded {
		t.Error("a nil message reported a thread")
	}
	// A turn already in a thread does not nest. Resolved from cached state, so
	// this makes no API call and the unreachable session is never dialled.
	thread := &discordgo.Channel{
		ID:       "1390000000000000009",
		ParentID: "1390000000000000003",
		Type:     discordgo.ChannelTypeGuildPublicThread,
		GuildID:  "1390000000000000002",
	}
	if err := session.State.ChannelAdd(thread); err != nil {
		t.Fatalf("seed channel state: %v", err)
	}
	inThread := &discordgo.Message{ID: "1401", ChannelID: thread.ID, Content: "again"}
	if _, threaded := threadForReply(session, inThread); threaded {
		t.Error("a turn inside a thread started a nested one")
	}
}

// A turn that never posted a progress line has nothing in the channel pointing
// at a thread, so it does not get one however long it took.
func TestATurnWithNoProgressLineGetsNoThread(t *testing.T) {
	t.Parallel()
	var quiet *turnProgress
	if quiet.longEnough() {
		t.Error("a nil progress reported a long turn")
	}
	if turnLongReply(t.Context()) {
		t.Error("a context with no progress reported a long turn")
	}
}

// The window is the wait plus two beats, whatever the wait is set to. See
// docs/sirens-echo-progress.md and sirens-echo#375.
func TestTheThreadWindowIsTheOneThatWasAskedFor(t *testing.T) {
	t.Parallel()
	if turnLongReplyAfter != turnProgressAfter+turnProgressEvery*2 {
		t.Errorf("the thread window is %s, want the wait plus two beats", turnLongReplyAfter)
	}
	if turnLongReplyAfter <= turnProgressAfter {
		t.Error("a turn could cross the thread window before it posts a progress line")
	}
}
