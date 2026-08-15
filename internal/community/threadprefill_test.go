package community

import (
	"errors"
	"strconv"
	"strings"
	"testing"

	"github.com/bwmarrin/discordgo"
)

// Inside a thread the whole thread is the context, and what will not fit is
// dropped oldest first and said out loud. See sirens-echo#769.

func threadedSession(t *testing.T, kind discordgo.ChannelType) *discordgo.Session {
	t.Helper()
	session := &discordgo.Session{State: discordgo.NewState()}
	// A channel only enters the cache under a guild it belongs to, the shape
	// the gateway delivers it in.
	if err := session.State.GuildAdd(&discordgo.Guild{ID: "guild-1"}); err != nil {
		t.Fatalf("GuildAdd: %v", err)
	}
	if err := session.State.ChannelAdd(&discordgo.Channel{
		ID: "chan-1", Type: kind, GuildID: "guild-1",
	}); err != nil {
		t.Fatalf("ChannelAdd: %v", err)
	}
	return session
}

func TestAThreadIsRecognisedFromCachedStateOnly(t *testing.T) {
	t.Parallel()
	message := &discordgo.Message{ID: "m-1", ChannelID: "chan-1"}

	if !inThread(threadedSession(t, discordgo.ChannelTypeGuildPublicThread), message) {
		t.Error("a public thread was not recognised")
	}
	if inThread(threadedSession(t, discordgo.ChannelTypeGuildText), message) {
		t.Error("an ordinary channel read as a thread")
	}
	// Every absent input answers no rather than reaching for the API.
	if inThread(nil, message) || inThread(&discordgo.Session{}, message) {
		t.Error("a session without cached state reported a thread")
	}
	if inThread(threadedSession(t, discordgo.ChannelTypeGuildPublicThread), nil) {
		t.Error("a nil message reported a thread")
	}
}

// The headline. Everything that fits is carried, and what does not is dropped
// from the oldest end so the newest exchange survives.
func TestAPrefillDropsTheOldestUntilItFits(t *testing.T) {
	t.Parallel()
	history := []TranscriptEntry{
		{Author: "a", Content: strings.Repeat("x", 100)},
		{Author: "b", Content: strings.Repeat("y", 100)},
		{Author: "c", Content: strings.Repeat("z", 100)},
	}

	// Each entry costs its author plus its content, so 101. Two must go for
	// the third to fit.
	kept, dropped := boundPrefill(history, 150)

	if dropped != 2 {
		t.Fatalf("dropped %d, want the two oldest", dropped)
	}
	if len(kept) != 1 || kept[0].Author != "c" {
		t.Errorf("kept %v, want the newest entry only", kept)
	}
}

// A thread inside the budget is carried whole, which is the feature.
func TestAPrefillInsideTheBudgetCarriesEveryMessage(t *testing.T) {
	t.Parallel()
	history := make([]TranscriptEntry, 0, 200)
	for index := 0; index < 200; index++ {
		history = append(history, TranscriptEntry{
			Author: "member", Content: "message " + strconv.Itoa(index),
		})
	}

	kept, dropped := boundPrefill(history, threadPrefillRunes)

	if dropped != 0 {
		t.Errorf("dropped %d from a thread that fits", dropped)
	}
	if len(kept) != len(history) {
		t.Errorf("carried %d of %d messages", len(kept), len(history))
	}
}

// The bound is real. Without it a long thread is the context risk this issue
// was filed about.
func TestAPrefillNeverExceedsItsBudget(t *testing.T) {
	t.Parallel()
	history := make([]TranscriptEntry, 0, 400)
	for index := 0; index < 400; index++ {
		history = append(history, TranscriptEntry{
			Author: "member", Content: strings.Repeat("q", 500),
		})
	}

	kept, dropped := boundPrefill(history, threadPrefillRunes)

	total := 0
	for _, entry := range kept {
		total += prefillCost(entry)
	}
	if total > threadPrefillRunes {
		t.Errorf("prefill is %d runes, over the %d bound", total, threadPrefillRunes)
	}
	if dropped == 0 {
		t.Error("nothing was dropped from a thread far over the bound")
	}
}

// Silent truncation would make a thin answer indistinguishable from any other
// wrong answer, which the spec rejects by name.
func TestADroppedPrefillIsAlwaysAnnotated(t *testing.T) {
	t.Parallel()
	for _, dropped := range []int{1, 2, 47} {
		content, _ := fitWithOverflow("the answer", discordReplyLimit, nil, dropped)
		if !strings.Contains(content, "thread context truncated") {
			t.Errorf("dropping %d messages produced no annotation: %q", dropped, content)
		}
		if !strings.Contains(content, strconv.Itoa(dropped)) {
			t.Errorf("the annotation does not name %d dropped messages: %q", dropped, content)
		}
	}
}

// A turn that dropped nothing renders exactly what it rendered before.
func TestAnUntruncatedReplyIsUnchanged(t *testing.T) {
	t.Parallel()
	withNone, _ := fitWithOverflow("the answer", discordReplyLimit, nil, 0)

	if strings.Contains(withNone, "truncated") {
		t.Errorf("a turn that dropped nothing was annotated: %q", withNone)
	}
	if withNone != "the answer" {
		t.Errorf("reply = %q, want the answer untouched", withNone)
	}
}

// Only the Discord transport can drop context. Every other turn reports none,
// so its replies are byte-identical.
func TestATransportThatCannotDropReportsNone(t *testing.T) {
	t.Parallel()
	if got := droppedPrefill(&httpTurn{requestID: "req-1"}); got != 0 {
		t.Errorf("an HTTP turn reported %d dropped messages", got)
	}
	if got := droppedPrefill(&discordMessageTurn{prefillDropped: 3}); got != 3 {
		t.Errorf("a Discord turn reported %d, want 3", got)
	}
}

// pagingFetcher answers full pages until it runs out, so a test can drive the
// walk without a live session.
type pagingFetcher struct {
	pages int
	calls int
}

func (f *pagingFetcher) fetch(string) ([]*discordgo.Message, error) {
	f.calls++
	if f.calls > f.pages {
		return nil, nil
	}
	batch := make([]*discordgo.Message, threadPrefillPageSize)
	for index := range batch {
		batch[index] = &discordgo.Message{ID: strconv.Itoa(f.calls*1000 + index)}
	}
	return batch, nil
}

// A short page is the start of the thread, so the walk stops rather than
// spending the whole page budget on every prefill.
func TestPagingStopsAtTheStartOfTheThread(t *testing.T) {
	t.Parallel()
	fetcher := &pagingFetcher{pages: 2}

	collected, err := pageBack(fetcher.fetch, "newest")

	if err != nil {
		t.Fatalf("pageBack: %v", err)
	}
	if fetcher.calls != 3 {
		t.Errorf("made %d calls, want two full pages and one short", fetcher.calls)
	}
	if len(collected) != 2*threadPrefillPageSize {
		t.Errorf("collected %d messages", len(collected))
	}
}

// The rate-limit bound. A thread longer than the page budget stops walking.
func TestPagingIsBoundedOnAnEndlessThread(t *testing.T) {
	t.Parallel()
	fetcher := &pagingFetcher{pages: 1 << 30}

	collected, err := pageBack(fetcher.fetch, "newest")

	if err != nil {
		t.Fatalf("pageBack: %v", err)
	}
	if fetcher.calls != threadPrefillPages {
		t.Errorf("made %d calls, want the %d page bound", fetcher.calls, threadPrefillPages)
	}
	if len(collected) != threadPrefillPages*threadPrefillPageSize {
		t.Errorf("collected %d messages", len(collected))
	}
}

// A failed page fails the prefill rather than returning half a thread, because
// half a thread silently is the thing this issue is against.
func TestAFailedPageDoesNotReturnHalfAThread(t *testing.T) {
	t.Parallel()
	failing := func(string) ([]*discordgo.Message, error) {
		return nil, errors.New("discord said no")
	}

	if _, err := pageBack(failing, "newest"); err == nil {
		t.Error("a failed page reported success")
	}
}
