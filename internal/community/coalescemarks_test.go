package community

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/bwmarrin/discordgo"

	"github.com/coilyco/sirens-echo/internal/coalesce"
	"github.com/coilyco/sirens-echo/internal/ingest"
)

// recordingMarker stands in for the session half a reaction uses, which is the
// whole reason the Discord side of the lane had no coverage. sirens-echo#988.
type recordingMarker struct {
	mu      sync.Mutex
	added   []string
	removed []string
	fail    error
}

func (m *recordingMarker) MessageReactionAdd(
	_, messageID, emoji string, _ ...discordgo.RequestOption,
) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.added = append(m.added, messageID+":"+emoji)
	return m.fail
}

func (m *recordingMarker) MessageReactionRemove(
	_, messageID, emoji, _ string, _ ...discordgo.RequestOption,
) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.removed = append(m.removed, messageID+":"+emoji)
	return m.fail
}

func (m *recordingMarker) marks() ([]string, []string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([]string(nil), m.added...), append([]string(nil), m.removed...)
}

// markedSummon builds one summon whose marks land in the recorder rather than
// on Discord, and reports whether its hold came back.
func markedSummon(marker *recordingMarker, id string, released *int) *discordSummon {
	return &discordSummon{
		turn: &discordMessageTurn{
			message: &discordgo.Message{ID: id, ChannelID: "channel-1"},
			marker:  marker,
		},
		ctx:   context.Background(),
		leave: func() { *released++ },
	}
}

func markedAgent(t *testing.T) *Agent {
	t.Helper()
	agent := &Agent{telemetry: telemetryOrNoop(nil)}
	agent.ensureRuntimeDefaults()
	return agent
}

// An ask marks its comment on arrival, before the queue can hold it. Folding
// the work is allowed, folding the ack is not.
func TestAnAskMarksItsCommentOnArrival(t *testing.T) {
	t.Parallel()
	marker := &recordingMarker{}
	released := 0
	ack := &discordAck{agent: markedAgent(t)}

	if err := ack.Queued(context.Background(), ingest.Ask{
		Subject: markedSummon(marker, "comment-1", &released),
	}); err != nil {
		t.Fatalf("Queued: %v", err)
	}

	added, removed := marker.marks()
	if len(added) != 1 || added[0] != "comment-1:"+reactionAccepted {
		t.Errorf("added = %v, want the arrival mark on comment-1", added)
	}
	if len(removed) != 0 {
		t.Errorf("removed = %v, want nothing retracted on arrival", removed)
	}
	if released != 0 {
		t.Errorf("a queued ask gave back its hold %d times, want 0", released)
	}
}

// A shed ask has its mark retracted and replaced by the failure mark, because
// the arrival mark promised an answer nobody will now produce.
func TestAShedAskRetractsItsArrivalMark(t *testing.T) {
	t.Parallel()
	marker := &recordingMarker{}
	released := 0
	ack := &discordAck{agent: markedAgent(t)}

	if err := ack.Shed(context.Background(), ingest.Ask{
		Subject: markedSummon(marker, "comment-1", &released),
	}); err != nil {
		t.Fatalf("Shed: %v", err)
	}

	added, removed := marker.marks()
	if len(removed) != 1 || removed[0] != "comment-1:"+reactionAccepted {
		t.Errorf("removed = %v, want the arrival mark retracted", removed)
	}
	if len(added) != 1 || added[0] != "comment-1:"+reactionFailed {
		t.Errorf("added = %v, want the failure mark in its place", added)
	}
	if released != 1 {
		t.Errorf("a shed ask gave back its hold %d times, want exactly 1", released)
	}
}

// A batch clears the arrival mark from every folded comment and not from the
// one the turn answered, whose own turn clears it.
func TestABatchClearsEveryFoldedMarkButTheAnsweredOne(t *testing.T) {
	t.Parallel()
	marker := &recordingMarker{}
	released := 0
	runner := &batchRunner{agent: markedAgent(t)}
	distinct := []*discordSummon{
		markedSummon(marker, "older-1", &released),
		markedSummon(marker, "older-2", &released),
		markedSummon(marker, "newest", &released),
	}

	runner.settle(context.Background(), distinct, distinct)

	_, removed := marker.marks()
	if len(removed) != 2 {
		t.Fatalf("removed = %v, want the two folded comments cleared", removed)
	}
	for _, want := range []string{"older-1:" + reactionAccepted, "older-2:" + reactionAccepted} {
		if !containsString(removed, want) {
			t.Errorf("removed = %v, want %q", removed, want)
		}
	}
	if containsString(removed, "newest:"+reactionAccepted) {
		t.Error("the answered comment's mark was cleared twice, once here and once by its turn")
	}
}

// Every ask in a batch gives back its drain hold exactly once, on the served
// path, or a shutdown waits forever on a hold nobody returns.
func TestEveryAskInABatchGivesBackItsHoldExactlyOnce(t *testing.T) {
	t.Parallel()
	marker := &recordingMarker{}
	released := 0
	runner := &batchRunner{agent: markedAgent(t)}
	covered := []*discordSummon{
		markedSummon(marker, "one", &released),
		markedSummon(marker, "two", &released),
		markedSummon(marker, "three", &released),
	}

	runner.settle(context.Background(), covered, covered)
	// Twice, because a settle that ran again must not double-count.
	runner.settle(context.Background(), covered, covered)

	if released != len(covered) {
		t.Errorf("holds returned = %d, want exactly %d", released, len(covered))
	}
}

// A shelved batch marks and gives its holds back, since the gateway a reply
// would need is what is closing.
func TestAShelvedBatchMarksAndReleases(t *testing.T) {
	t.Parallel()
	marker := &recordingMarker{}
	released := 0
	shelf := &batchShelf{agent: markedAgent(t)}
	summon := markedSummon(marker, "comment-1", &released)

	shelf.Shelve(context.Background(), coalesceBatchOf(summon), errors.New("shutting down"))

	added, _ := marker.marks()
	if len(added) != 1 || added[0] != "comment-1:"+reactionFailed {
		t.Errorf("added = %v, want the failure mark", added)
	}
	if released != 1 {
		t.Errorf("a shelved ask gave back its hold %d times, want exactly 1", released)
	}
}

// A marking failure is swallowed, because a missing ADD_REACTIONS permission is
// an operator question rather than a turn failure.
func TestAMarkThatFailsDoesNotFailTheAsk(t *testing.T) {
	t.Parallel()
	marker := &recordingMarker{fail: errors.New("missing ADD_REACTIONS")}
	released := 0
	ack := &discordAck{agent: markedAgent(t)}

	if err := ack.Queued(context.Background(), ingest.Ask{
		Subject: markedSummon(marker, "comment-1", &released),
	}); err != nil {
		t.Errorf("Queued returned %v, want the failure swallowed", err)
	}
}

// A batch whose turn panics still settles its holds and tells the member,
// because a turn that died silently reads as being ignored.
func TestABatchWhoseTurnPanicsSettlesItsHoldsAndTellsTheMember(t *testing.T) {
	t.Parallel()
	marker := &recordingMarker{}
	released := 0
	runner := &batchRunner{agent: markedAgent(t)}
	distinct := []*discordSummon{
		markedSummon(marker, "older", &released),
		markedSummon(marker, "newest", &released),
	}
	// transportDiscord, because sendReply routes on it and the crash notice
	// has to take the path a real batch would.
	told := &httpTurn{requestID: "crashed", transport: transportDiscord}

	// The same order Run defers them in, which runs settle first and then the
	// recovery, so a panic cannot strand a hold behind the notice.
	func() {
		defer runner.recoverTurn(context.Background(), told)
		defer runner.settle(context.Background(), distinct, distinct)
		panic("the model client crashed")
	}()

	if released != len(distinct) {
		t.Errorf("holds returned = %d, want %d, so a shutdown waits forever", released, len(distinct))
	}
	if told.reply != noticeTurnCrashed {
		t.Errorf("member was told %q, want %q", told.reply, noticeTurnCrashed)
	}
}

func containsString(haystack []string, needle string) bool {
	for _, entry := range haystack {
		if entry == needle {
			return true
		}
	}
	return false
}

// coalesceBatchOf wraps summons the way the pool hands them to the shelf.
func coalesceBatchOf(summons ...*discordSummon) coalesce.Batch {
	covers := make([]ingest.Ask, 0, len(summons))
	for index, summon := range summons {
		covers = append(covers, ingest.Ask{Seq: int64(index), Subject: summon})
	}
	return coalesce.Batch{Items: []coalesce.Item{{Covers: covers}}}
}
