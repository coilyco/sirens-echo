package community

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/bwmarrin/discordgo"
)

// The outbound hop destroyed the remainder while the inbound and tool-result
// paths saved theirs. See issue 791.

// A reply inside the budget must reach the member exactly as it does today,
// because the overflow path is an addition rather than a rewrite.
func TestAReplyThatFitsIsUnchangedAndWithholdsNothing(t *testing.T) {
	t.Parallel()
	answer := "Twelve rows were repaired."
	content, whole := fitWithOverflow(answer, discordReplyLimit, nil, 0)
	if want := AssembleReply(answer, discordReplyLimit); content != want {
		t.Errorf("content = %q, want %q", content, want)
	}
	if whole != nothingWithheld {
		t.Errorf("a reply that fits withheld %d bytes", len(whole))
	}
}

// A transport with no ceiling declares zero, and nothing about it changes.
func TestAnUnboundedTransportWithholdsNothing(t *testing.T) {
	t.Parallel()
	answer := strings.Repeat("long. ", 2000)
	content, whole := fitWithOverflow(answer, unboundedReply, nil, 0)
	if whole != nothingWithheld {
		t.Errorf("an unbounded transport withheld %d bytes", len(whole))
	}
	if content != AssembleReply(answer, unboundedReply) {
		t.Error("an unbounded transport did not send the whole assembly")
	}
}

// The acceptance on 791: the whole reply reaches the member rather than the
// first 1990 characters of it.
func TestAnOverflowingReplyKeepsTheWholeText(t *testing.T) {
	t.Parallel()
	answer := strings.Repeat("a sentence that runs on. ", 400)
	content, whole := fitWithOverflow(answer, discordReplyLimit, nil, 0)
	if whole == nothingWithheld {
		t.Fatal("a reply over the budget withheld nothing")
	}
	if whole != AssembleReply(answer, unboundedReply) {
		t.Error("the withheld text is not the whole assembled reply")
	}
	// The whole reply, not the tail. A member reading the file reads one
	// document rather than stitching two together.
	if !strings.HasPrefix(whole, answer[:64]) {
		t.Errorf("the file starts at %q, want the beginning of the answer", whole[:64])
	}
	if runeLen(content) > discordReplyLimit {
		t.Errorf("the message is %d runes, over the %d budget",
			runeLen(content), discordReplyLimit)
	}
}

// The second acceptance: the message states that the rest exists and how large
// it is, matching what the inbound and tool-result paths tell the model.
func TestTheMessageNamesTheFileAndTheTrueSize(t *testing.T) {
	t.Parallel()
	answer := strings.Repeat("a sentence that runs on. ", 400)
	content, whole := fitWithOverflow(answer, discordReplyLimit, nil, 0)
	if !strings.Contains(content, overflowFileName) {
		t.Errorf("the message does not name the file: %q", tail(content))
	}
	if !strings.Contains(content, fmt.Sprintf("%d bytes", len(whole))) {
		t.Errorf("the message does not carry the true size %d: %q", len(whole), tail(content))
	}
}

// A reply larger than the attachment bound falls back rather than failing the
// turn, the way every scratchpad failure already does.
func TestAReplyTooLargeToAttachFallsBack(t *testing.T) {
	t.Parallel()
	answer := strings.Repeat("x", replyAttachmentBytes+1)
	content, whole := fitWithOverflow(answer, discordReplyLimit, nil, 0)
	if whole != nothingWithheld {
		t.Errorf("a reply over the attachment bound withheld %d bytes", len(whole))
	}
	if content != AssembleReply(answer, discordReplyLimit) {
		t.Error("the fallback is not the previous behaviour")
	}
}

// A budget with no room for the notice cannot advertise the file, so it keeps
// the old behaviour rather than sending a message that is only the notice.
func TestABudgetTooSmallToSayItFallsBack(t *testing.T) {
	t.Parallel()
	answer := strings.Repeat("y", 200)
	content, whole := fitWithOverflow(answer, 8, nil, 0)
	if whole != nothingWithheld {
		t.Errorf("a budget of 8 runes advertised an attachment")
	}
	if runeLen(content) > 8 {
		t.Errorf("content = %d runes, over the 8 budget", runeLen(content))
	}
}

// A transport that cannot carry a file is handed the cut message, which is what
// keeps HTTP and the MCP turn exactly as they are.
func TestATransportWithNoAttachmentGetsTheMessageAlone(t *testing.T) {
	t.Parallel()
	turn := &recordingReplyTurn{}
	if err := deliverWithOverflow(context.Background(), turn, "the message", "the whole thing"); err != nil {
		t.Fatalf("deliverWithOverflow: %v", err)
	}
	if len(turn.sent) != 1 || turn.sent[0] != "the message" {
		t.Errorf("the turn received %v, want the message alone", turn.sent)
	}
}

// The seam, driven rather than asserted about: a carrier receives both halves.
func TestACarrierReceivesTheWholeReply(t *testing.T) {
	t.Parallel()
	turn := &carryingTurn{}
	if err := deliverWithOverflow(context.Background(), turn, "the message", "the whole thing"); err != nil {
		t.Fatalf("deliverWithOverflow: %v", err)
	}
	if turn.content != "the message" || turn.whole != "the whole thing" {
		t.Errorf("the carrier received (%q, %q)", turn.content, turn.whole)
	}
}

// Nothing withheld must not reach the carrier, or every ordinary reply would
// arrive with an empty file beside it.
func TestACarrierIsNotUsedWhenNothingWasCut(t *testing.T) {
	t.Parallel()
	turn := &carryingTurn{}
	if err := deliverWithOverflow(context.Background(), turn, "the message", nothingWithheld); err != nil {
		t.Fatalf("deliverWithOverflow: %v", err)
	}
	if turn.whole != "" || len(turn.sent) != 1 {
		t.Errorf("an ordinary reply took the carrier path: %q", turn.whole)
	}
}

// The one test that reaches Discord's own request builder, so the file is
// pinned where it is actually sent rather than where it is decided.
func TestTheAttachmentReachesDiscordAsAFile(t *testing.T) {
	t.Parallel()
	session, err := discordgo.New("Bot test-token")
	if err != nil {
		t.Fatalf("discordgo.New: %v", err)
	}
	capture := &capturingTransport{}
	session.Client = &http.Client{Transport: capture}
	turn := &discordMessageTurn{
		session: session,
		message: &discordgo.Message{ID: "m-1", ChannelID: "c-1"},
	}
	whole := strings.Repeat("the whole answer. ", 200)
	// The send fails, because the fake responds with no message. The request is
	// what this asserts, and it is built before the response is read.
	_ = turn.ReplyWithOverflow(context.Background(), "the message", whole)
	if capture.body == "" {
		t.Fatal("no request reached Discord")
	}
	if !strings.Contains(capture.body, overflowFileName) {
		t.Errorf("the request does not name %s", overflowFileName)
	}
	if !strings.Contains(capture.body, whole) {
		t.Error("the request does not carry the whole reply")
	}
	if !strings.HasPrefix(capture.contentType, "multipart/form-data") {
		t.Errorf("content type = %q, want multipart", capture.contentType)
	}
}

// An ordinary reply must stay a plain JSON send, so the file is added only
// where an overflow exists.
func TestAnOrdinaryReplySendsNoFile(t *testing.T) {
	t.Parallel()
	session, err := discordgo.New("Bot test-token")
	if err != nil {
		t.Fatalf("discordgo.New: %v", err)
	}
	capture := &capturingTransport{}
	session.Client = &http.Client{Transport: capture}
	turn := &discordMessageTurn{
		session: session,
		message: &discordgo.Message{ID: "m-1", ChannelID: "c-1"},
	}
	_ = turn.Reply(context.Background(), "the message")
	if strings.Contains(capture.contentType, "multipart") {
		t.Errorf("an ordinary reply sent a multipart body: %q", capture.contentType)
	}
	if strings.Contains(capture.body, overflowFileName) {
		t.Error("an ordinary reply named an attachment")
	}
}

func tail(value string) string {
	runes := []rune(value)
	if len(runes) <= 120 {
		return value
	}
	return string(runes[len(runes)-120:])
}

// capturingTransport reads the outgoing request and answers with a refusal, so
// no test reaches Discord and the request survives for assertions.
type capturingTransport struct {
	body        string
	contentType string
}

func (c *capturingTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	if request.Body != nil {
		body, _ := io.ReadAll(request.Body)
		c.body = string(body)
	}
	c.contentType = request.Header.Get("Content-Type")
	return &http.Response{
		StatusCode: http.StatusForbidden,
		Body:       io.NopCloser(strings.NewReader(`{"code":50013,"message":"Missing Permissions"}`)),
		Header:     http.Header{},
		Request:    request,
	}, nil
}

type recordingReplyTurn struct{ sent []string }

func (t *recordingReplyTurn) RequestID() string        { return "overflow" }
func (t *recordingReplyTurn) Requester() string        { return "318190481467244544" }
func (t *recordingReplyTurn) Transport() string        { return transportHTTP }
func (t *recordingReplyTurn) Current() TranscriptEntry { return TranscriptEntry{} }
func (t *recordingReplyTurn) History(context.Context) ([]TranscriptEntry, error) {
	return nil, nil
}

func (t *recordingReplyTurn) Reply(_ context.Context, content string) error {
	t.sent = append(t.sent, content)
	return nil
}

type carryingTurn struct {
	recordingReplyTurn
	content string
	whole   string
}

func (t *carryingTurn) ReplyWithOverflow(_ context.Context, content, whole string) error {
	t.content, t.whole = content, whole
	return nil
}
