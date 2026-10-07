package community

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/bwmarrin/discordgo"

	"github.com/coilyco/sirens-echo/internal/coalesce"
	"github.com/coilyco/sirens-echo/internal/ingest"
)

// The lane answers a member's rapid comments in one turn. It may never fold the
// acknowledgment, drop a comment, or read a folded comment as its own context.

func comment(id, text string) *discordgo.Message {
	return &discordgo.Message{
		ID:        id,
		ChannelID: "c-1",
		GuildID:   "g-1",
		Content:   text,
		Author:    &discordgo.User{ID: "u-1", Username: "ana"},
	}
}

func foldedTurn(texts ...string) *discordMessageTurn {
	messages := make([]*discordgo.Message, 0, len(texts))
	for at, text := range texts {
		messages = append(messages, comment(string(rune('a'+at)), text))
	}
	return &discordMessageTurn{
		message: messages[len(messages)-1],
		folded:  messages[:len(messages)-1],
	}
}

func TestAFoldedTurnAsksEveryCommentItAnswers(t *testing.T) {
	t.Parallel()
	turn := foldedTurn("what is iron worth", "and copper", "never mind copper")
	content := turn.Current().Content
	for _, want := range []string{"what is iron worth", "and copper", "never mind copper"} {
		if !strings.Contains(content, want) {
			t.Fatalf("the ask %q dropped the comment %q", content, want)
		}
	}
	// Arrival order, because the last comment is the one that revises the ones
	// before it and a reordered batch answers the wrong question.
	if strings.Index(content, "and copper") > strings.Index(content, "never mind copper") {
		t.Fatalf("the ask %q is out of arrival order", content)
	}
}

func TestAnUnfoldedTurnAsksExactlyWhatItAlwaysDid(t *testing.T) {
	t.Parallel()
	turn := &discordMessageTurn{message: comment("m-1", "what is iron worth")}
	if got := turn.Current().Content; got != "what is iron worth" {
		t.Fatalf("the ask is %q, want the message unchanged", got)
	}
	if got := turn.historyBefore(); got != "m-1" {
		t.Fatalf("history anchored at %q, want the message itself", got)
	}
}

// A folded comment is part of the ask. Reading it as history too would hand the
// model the same comment twice and call one of them context.
func TestAFoldedTurnReadsHistoryBeforeItsOldestComment(t *testing.T) {
	t.Parallel()
	turn := foldedTurn("first", "second", "third")
	if got := turn.historyBefore(); got != "a" {
		t.Fatalf("history anchored at %q, want the oldest comment in the batch", got)
	}
}

func TestAFoldedTurnCarriesEveryCommentsUploads(t *testing.T) {
	t.Parallel()
	older := comment("a", "look at this")
	older.Attachments = []*discordgo.MessageAttachment{
		{ID: "1", URL: "https://cdn.discordapp.com/one.txt", Filename: "one.txt"},
	}
	newest := comment("b", "and this")
	newest.Attachments = []*discordgo.MessageAttachment{
		{ID: "2", URL: "https://cdn.discordapp.com/two.txt", Filename: "two.txt"},
	}
	turn := &discordMessageTurn{message: newest, folded: []*discordgo.Message{older}}
	if got := len(turn.Attachments()); got != 2 {
		t.Fatalf("the turn carried %d uploads, want one per comment", got)
	}
	if got := len(turn.Current().Attachments); got != 2 {
		t.Fatalf("the ask named %d uploads, want one per comment", got)
	}
}

func askFor(seq int64, text string, summon *discordSummon) ingest.Ask {
	return ingest.Ask{
		Seq:     seq,
		Tenant:  ingest.Tenant{Surface: ingest.SurfaceDiscord, Channel: "c-1", Author: "u-1"},
		Locus:   "c-1",
		Text:    text,
		Subject: summon,
	}
}

func summonFor(id, text string) *discordSummon {
	return &discordSummon{turn: &discordMessageTurn{message: comment(id, text)}, leave: func() {}}
}

// Dedupe collapses the work and keeps the asks, so the turn answers each
// distinct request once while every comment stays covered.
func TestTheTurnAnswersEachDistinctRequestOnce(t *testing.T) {
	t.Parallel()
	first := summonFor("a", "what is iron worth")
	repeat := summonFor("b", "What is iron worth?")
	other := summonFor("c", "and copper")
	batch := coalesce.Batch{Items: []coalesce.Item{
		{
			Locus:  "c-1",
			Text:   "what is iron worth",
			Covers: []ingest.Ask{askFor(1, "what is iron worth", first), askFor(2, "x", repeat)},
		},
		{Locus: "c-1", Text: "and copper", Covers: []ingest.Ask{askFor(3, "and copper", other)}},
	}}
	distinct := distinctAsks(batch)
	if len(distinct) != 2 {
		t.Fatalf("the turn answers %d requests, want the 2 distinct ones", len(distinct))
	}
	if distinct[0].Seq != 1 || distinct[1].Seq != 3 {
		t.Fatalf("distinct asks are %d and %d, want arrival order", distinct[0].Seq, distinct[1].Seq)
	}
	if got := len(summonsIn(batch.Asks())); got != 3 {
		t.Fatalf("%d comments are covered, want every one of the 3", got)
	}
}

// The newest comment carries the reply, because that is the one the member is
// looking at when the answer arrives.
func TestTheNewestCommentCarriesTheReply(t *testing.T) {
	t.Parallel()
	oldest := summonFor("a", "first")
	newest := summonFor("c", "third")
	turn := foldTurn([]*discordSummon{oldest, summonFor("b", "second"), newest})
	if turn.message.ID != "c" {
		t.Fatalf("the reply lands on %q, want the newest comment", turn.message.ID)
	}
	if len(turn.folded) != 2 || turn.folded[0].ID != "a" {
		t.Fatalf("folded %v, want the earlier comments oldest first", turn.folded)
	}
}

// A shed ask and an answered one both settle it, and an ask settled twice would
// let shutdown stop waiting for work that is still running.
func TestASummonGivesBackItsHoldOnce(t *testing.T) {
	t.Parallel()
	var released int
	var mu sync.Mutex
	summon := &discordSummon{leave: func() {
		mu.Lock()
		defer mu.Unlock()
		released++
	}}
	summon.release()
	summon.release()
	if released != 1 {
		t.Fatalf("the hold was given back %d times, want once", released)
	}
}

// The lane's own numbers, read off the knobs a deployment sets rather than off
// the package defaults the bridge runs on.
func TestTheLaneTakesItsTuningFromTheKnobs(t *testing.T) {
	restoreKnobs(t)
	applyKnobs(fixedLookup(map[string]string{
		"SIRENS_ECHO_COALESCE_WINDOW":  "2s",
		"SIRENS_ECHO_COALESCE_BATCH":   "5",
		"SIRENS_ECHO_COALESCE_WORKERS": "4",
	}))
	policy := coalescePolicy(90 * time.Second)
	if policy.Window != 2*time.Second || policy.Batch != 5 || policy.Workers != 4 {
		t.Fatalf("policy = %+v, want the overridden window, batch, and pool", policy)
	}
	// Widening starts where the window stopped keeping up, so it moves with the
	// pool and the batch rather than being set behind their backs.
	if policy.HighWater != 20 {
		t.Fatalf("high water = %d, want the pool full of narrow batches", policy.HighWater)
	}
	if policy.Deadline != 90*time.Second {
		t.Fatalf("deadline = %s, want the request budget", policy.Deadline)
	}
}

// The acceptance the lane exists for: a member's rapid comments reach one turn,
// under the numbers this service ships rather than a test's own.
func TestRapidCommentsFromOneMemberShareOneTurn(t *testing.T) {
	restoreKnobs(t)
	// The window runs from the first ask, so a stalled runner closes it early.
	// 1s outlasts a stall, and submitting before Run keeps the exposure short.
	applyKnobs(fixedLookup(map[string]string{"SIRENS_ECHO_COALESCE_WINDOW": "1s"}))
	policy := coalescePolicy(time.Minute)
	queue := ingest.NewQueue(coalesceCapacity)
	in := ingest.NewIngress(queue, nil, nil, nil, nil)
	coalescer := coalesce.NewCoalescer(policy, queue, nil)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	tenant := ingest.Tenant{Surface: ingest.SurfaceDiscord, Guild: "g-1", Channel: "c-1", Author: "u-1"}
	for _, text := range []string{"what is iron worth", "and copper", "and gold"} {
		in.Submit(ctx, tenant, "c-1", text, summonFor("m", text))
	}
	go coalescer.Run(ctx)
	select {
	case batch := <-coalescer.Batches():
		if batch.Size() != 3 {
			t.Fatalf("the batch carried %d comments, want all 3 in one turn", batch.Size())
		}
	case <-ctx.Done():
		t.Fatal("the window never closed on three comments")
	}
}

// Two members talking in one channel are two conversations, so the pool may
// answer them at once rather than making one wait out the other.
func TestTwoMembersInOneChannelDoNotShareAShard(t *testing.T) {
	t.Parallel()
	ana := ingest.Tenant{Surface: ingest.SurfaceDiscord, Guild: "g-1", Channel: "c-1", Author: "u-1"}
	bo := ana
	bo.Author = "u-2"
	if ana.Key() == bo.Key() {
		t.Fatal("two members in one channel serialize behind each other")
	}
}

// A folded ask is multi-part, and nothing else in the prompt says so. Without
// this the model may answer the last comment and drop the ones before it.

func TestAFoldedAskTellsTheModelToCoverEveryComment(t *testing.T) {
	t.Parallel()
	prompt := BuildTurnPrompt("system", nil, TranscriptEntry{
		Author:   "ana",
		Content:  "what is iron worth\n\nand copper",
		Comments: 3,
	})
	if !strings.Contains(prompt.Context, "3 comments") {
		t.Fatalf("context %q does not say how many comments it carries", prompt.Context)
	}
	if !strings.Contains(prompt.Context, "answering every one of them") {
		t.Fatalf("context %q does not ask for every comment to be answered", prompt.Context)
	}
	// A count, never a channel, author id, or surface: the identifier guard
	// refuses a reply carrying any value this process holds.
	if strings.Contains(prompt.Context, "discord") || strings.Contains(prompt.Context, "c-1") {
		t.Fatalf("context %q leaked a transport identifier", prompt.Context)
	}
}

func TestAnUnfoldedAskSaysNothingAboutCoverage(t *testing.T) {
	t.Parallel()
	for _, comments := range []int{0, 1} {
		prompt := BuildTurnPrompt("system", nil, TranscriptEntry{
			Author:   "ana",
			Content:  "what is iron worth",
			Comments: comments,
		})
		if strings.Contains(prompt.Context, "comments") {
			t.Fatalf("a turn of %d comments framed itself as several: %q", comments, prompt.Context)
		}
	}
}

func TestTheFoldedTurnReportsHowManyCommentsItCarries(t *testing.T) {
	t.Parallel()
	if got := foldedTurn("first", "second", "third").Current().Comments; got != 3 {
		t.Fatalf("the ask carries %d comments, want all 3", got)
	}
	plain := &discordMessageTurn{message: comment("m-1", "only one")}
	if got := plain.Current().Comments; got != 1 {
		t.Fatalf("an unfolded ask carries %d comments, want 1", got)
	}
}
