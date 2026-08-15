package community

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"testing"

	"github.com/bwmarrin/discordgo"
)

// A delivery failure that records only that it failed cannot be diagnosed, and
// the reply it dropped was already paid for. See issue 292.

func attrValue(attrs []slog.Attr, key string) (slog.Value, bool) {
	for _, attr := range attrs {
		if attr.Key == key {
			return attr.Value, true
		}
	}
	return slog.Value{}, false
}

// The four candidates QA named are all separable by status plus code, which is
// what makes these two fields sufficient rather than a starting point.
func TestARestErrorIsRecordedByStatusAndCode(t *testing.T) {
	t.Parallel()
	rest := &discordgo.RESTError{
		Response: &http.Response{StatusCode: http.StatusTooManyRequests},
		Message:  &discordgo.APIErrorMessage{Code: 20028},
	}
	attrs := discordFailureAttrs(errors.Join(errors.New("send: "), rest))
	status, ok := attrValue(attrs, "discord_status")
	if !ok || status.Int64() != http.StatusTooManyRequests {
		t.Errorf("status = %v, want 429", status)
	}
	code, ok := attrValue(attrs, "discord_code")
	if !ok || code.Int64() != 20028 {
		t.Errorf("code = %v, want 20028", code)
	}
}

// A gateway that dropped produces no HTTP exchange at all, and that absence is
// a classification rather than a gap.
func TestAFailureWithNoResponseIsItsOwnClass(t *testing.T) {
	t.Parallel()
	attrs := discordFailureAttrs(errors.New("websocket: close 1006"))
	failure, ok := attrValue(attrs, "discord_failure")
	if !ok || failure.String() != "no_response" {
		t.Errorf("failure = %v, want no_response", failure)
	}
	if _, present := attrValue(attrs, "discord_status"); present {
		t.Error("a failure with no response reported a status")
	}
}

// A REST error missing either half must still record the half it has, since a
// partial classification beats none.
func TestAPartialRestErrorStillRecordsWhatItHas(t *testing.T) {
	t.Parallel()
	attrs := discordFailureAttrs(&discordgo.RESTError{
		Response: &http.Response{StatusCode: http.StatusForbidden},
	})
	status, ok := attrValue(attrs, "discord_status")
	if !ok || status.Int64() != http.StatusForbidden {
		t.Errorf("status = %v, want 403", status)
	}
	if _, present := attrValue(attrs, "discord_code"); present {
		t.Error("a REST error with no message reported a code")
	}
}

func TestNoErrorRecordsNothing(t *testing.T) {
	t.Parallel()
	if attrs := discordFailureAttrs(nil); len(attrs) != 0 {
		t.Errorf("a nil error produced %d attributes", len(attrs))
	}
}

// A timeout and a backend outage both fail at the model stage, so error_type
// collapses them. The cause is what an alert can actually separate.
func TestATimeoutAndAnOutageDoNotShareACause(t *testing.T) {
	t.Parallel()
	timeout := failureCause(context.DeadlineExceeded)
	outage := failureCause(errors.New("proxy returned 503"))
	if timeout == outage {
		t.Fatalf("both classified as %q", timeout)
	}
	if timeout != causeTimeout {
		t.Errorf("timeout cause = %q, want %q", timeout, causeTimeout)
	}
}

// The label and the phrase a member reads are picked by the same order, so a
// notice can never describe a condition the telemetry classified differently.
func TestTheCauseAgreesWithTheNoticeAMemberReads(t *testing.T) {
	t.Parallel()
	for _, probe := range []struct {
		cause  error
		notice string
		want   string
	}{
		{context.DeadlineExceeded, noticeTimedOut, causeTimeout},
		{ErrToolRoundsExhausted, noticeRoundsSpent, causeRoundsSpent},
		{errors.New("backend refused"), noticeModelFailed, causeStage},
	} {
		if got := failureCause(probe.cause); got != probe.want {
			t.Errorf("cause = %q, want %q", got, probe.want)
		}
		if got := turnFailureNotice(stageModel, probe.cause); got != probe.notice {
			t.Errorf("notice = %q, want %q", got, probe.notice)
		}
	}
}

// A budget this service chose is not Discord failing to answer, and reading a
// run of them as an outage is the mistake the split prevents. See #648.
func TestASendWeAbandonedIsNotASendDiscordIgnored(t *testing.T) {
	t.Parallel()
	for _, testCase := range []struct {
		name string
		err  error
	}{
		{"the reply budget expired", context.DeadlineExceeded},
		{"the turn was cancelled", context.Canceled},
		{"wrapped, which is how it arrives", errors.Join(
			errors.New("discord reply: "), context.DeadlineExceeded,
		)},
	} {
		attrs := discordFailureAttrs(testCase.err)
		failure, ok := attrValue(attrs, "discord_failure")
		if !ok || failure.String() != "abandoned" {
			t.Errorf("%s: failure = %v, want abandoned", testCase.name, failure)
		}
	}
}

// Everything Discord genuinely failed to answer keeps its label, so the split
// costs the existing series nothing it should have kept.
func TestATransportFailureIsStillNoResponse(t *testing.T) {
	t.Parallel()
	for _, err := range []error{
		errors.New("websocket: close 1006"),
		errors.New("dial tcp: connection refused"),
	} {
		failure, ok := attrValue(discordFailureAttrs(err), "discord_failure")
		if !ok || failure.String() != "no_response" {
			t.Errorf("%v: failure = %v, want no_response", err, failure)
		}
	}
}

// A rejection Discord did answer outranks nothing here, because a REST error
// carrying a cancelled context is still Discord's verdict.
func TestARestErrorIsStillClassifiedByItsStatus(t *testing.T) {
	t.Parallel()
	rest := &discordgo.RESTError{
		Response: &http.Response{StatusCode: http.StatusForbidden},
	}
	failure, ok := attrValue(discordFailureAttrs(rest), "discord_failure")
	if !ok || failure.String() != "rest_error" {
		t.Errorf("failure = %v, want rest_error", failure)
	}
}

// The sentence above was true of the comment and not of the code: only a bare
// rejection was passed, and a joined one reported abandoned. See #292.
func TestARejectionKeepsItsVerdictBesideAContextError(t *testing.T) {
	t.Parallel()
	rejection := func() *discordgo.RESTError {
		return &discordgo.RESTError{
			Response: &http.Response{StatusCode: http.StatusForbidden},
			Message:  &discordgo.APIErrorMessage{Code: 50013},
		}
	}
	// The turn returns errors.Join(send, notice), and the notice runs on a
	// context the send has often just exhausted. Both orders, deliberately.
	for name, err := range map[string]error{
		"send first":   errors.Join(rejection(), context.DeadlineExceeded),
		"notice first": errors.Join(context.DeadlineExceeded, rejection()),
		"cancelled":    errors.Join(rejection(), context.Canceled),
		"wrapped":      errors.Join(errors.New("reply: "), rejection(), context.Canceled),
	} {
		attrs := discordFailureAttrs(err)
		failure, ok := attrValue(attrs, "discord_failure")
		if !ok || failure.String() != "rest_error" {
			t.Errorf("%s: failure = %v, want rest_error", name, failure)
			continue
		}
		// Naming the class without the status leaves an operator where they
		// started, which is the whole complaint on 292.
		status, hasStatus := attrValue(attrs, "discord_status")
		if !hasStatus || status.String() != "403" {
			t.Errorf("%s: discord_status = %v, want 403", name, status)
		}
		code, hasCode := attrValue(attrs, "discord_code")
		if !hasCode || code.String() != "50013" {
			t.Errorf("%s: discord_code = %v, want 50013", name, code)
		}
	}
}

// Thirteen of fourteen classified turn failures were stage failures wearing a
// Discord verdict, which is why every sample read no_response. See #292.
func TestATurnThatNeverSentIsNotADiscordVerdict(t *testing.T) {
	t.Parallel()
	for name, err := range map[string]error{
		"the model stage failed":     errors.New("proxy returned 503"),
		"the turn ran out of budget": context.DeadlineExceeded,
		"the queue gave up first":    errors.New("turn waited longer than 30s for the execution slot"),
		"a stage failure joined its notice": errors.Join(
			context.DeadlineExceeded, errors.New("still refused"),
		),
	} {
		failure, ok := attrValue(turnFailureAttrs(err), "discord_failure")
		if !ok || failure.String() != "not_attempted" {
			t.Errorf("%s: failure = %v, want not_attempted", name, failure)
		}
	}
}

// Driven through the seam rather than asserted about the marker, so classifying
// the turn error directly again fails here.
func TestASendThatFailedKeepsItsVerdictAtTheTurn(t *testing.T) {
	t.Parallel()
	agent := &Agent{telemetry: telemetryOrNoop(nil)}
	turn := &rejectingTurn{code: 50013, status: http.StatusForbidden}
	err := agent.deliverOrReport(context.Background(), turn, "the answer", nothingWithheld)
	if err == nil {
		t.Fatal("a failed send reported success")
	}
	attrs := turnFailureAttrs(err)
	failure, ok := attrValue(attrs, "discord_failure")
	if !ok || failure.String() != "rest_error" {
		t.Fatalf("failure = %v, want rest_error", failure)
	}
	status, ok := attrValue(attrs, "discord_status")
	if !ok || status.String() != "403" {
		t.Errorf("discord_status = %v, want 403", status)
	}
	code, ok := attrValue(attrs, "discord_code")
	if !ok || code.String() != "50013" {
		t.Errorf("discord_code = %v, want 50013", code)
	}
}

// rejectingTurn answers every send with the rejection Discord would.
type rejectingTurn struct {
	code   int
	status int
}

func (t *rejectingTurn) RequestID() string        { return "rejected" }
func (t *rejectingTurn) Requester() string        { return "318190481467244544" }
func (t *rejectingTurn) Transport() string        { return transportDiscord }
func (t *rejectingTurn) Current() TranscriptEntry { return TranscriptEntry{} }
func (t *rejectingTurn) History(context.Context) ([]TranscriptEntry, error) {
	return nil, nil
}

func (t *rejectingTurn) Reply(context.Context, string) error {
	return &discordgo.RESTError{
		Response: &http.Response{StatusCode: t.status},
		Message:  &discordgo.APIErrorMessage{Code: t.code},
	}
}

// A rejection carrying no response still names its class, so the reordering
// trades a wrong label for a correct one rather than for a missing one.
func TestABareRejectionStillClassifiesWithoutInventingAStatus(t *testing.T) {
	t.Parallel()
	attrs := discordFailureAttrs(errors.Join(&discordgo.RESTError{}, context.DeadlineExceeded))
	failure, ok := attrValue(attrs, "discord_failure")
	if !ok || failure.String() != "rest_error" {
		t.Errorf("failure = %v, want rest_error", failure)
	}
	if status, present := attrValue(attrs, "discord_status"); present {
		t.Errorf("a bare rejection invented a status: %v", status)
	}
}
