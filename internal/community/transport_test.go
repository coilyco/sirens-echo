package community

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/bwmarrin/discordgo"
)

type fakeTransport struct {
	name    string
	log     *[]string
	startOn error
}

func (f fakeTransport) Name() string { return f.name }

func (f fakeTransport) Start(context.Context) (func(), error) {
	*f.log = append(*f.log, "start "+f.name)
	if f.startOn != nil {
		return nil, f.startOn
	}
	return func() { *f.log = append(*f.log, "stop "+f.name) }, nil
}

func TestTransportsStartInOrderAndStopInReverse(t *testing.T) {
	var log []string
	stop, err := startTransports(context.Background(), []Transport{
		fakeTransport{name: "a", log: &log},
		fakeTransport{name: "b", log: &log},
	})
	if err != nil {
		t.Fatalf("startTransports: %v", err)
	}
	stop()
	if got, want := strings.Join(log, ","), "start a,start b,stop b,stop a"; got != want {
		t.Fatalf("order = %s, want %s", got, want)
	}
}

func TestAFailedTransportStopsTheOnesAlreadyRunning(t *testing.T) {
	var log []string
	boom := errors.New("gateway refused")
	stop, err := startTransports(context.Background(), []Transport{
		fakeTransport{name: "a", log: &log},
		fakeTransport{name: "b", log: &log, startOn: boom},
		fakeTransport{name: "c", log: &log},
	})
	if stop != nil {
		t.Fatal("a failed start must not return a stop to defer")
	}
	if !errors.Is(err, boom) || !strings.Contains(err.Error(), "b transport") {
		t.Fatalf("err = %v, want it to wrap the cause and name the transport", err)
	}
	if got, want := strings.Join(log, ","), "start a,start b,stop a"; got != want {
		t.Fatalf("order = %s, want %s: c must never start, a must stop", got, want)
	}
}

func TestNoTransportsStartsAndStopsCleanly(t *testing.T) {
	stop, err := startTransports(context.Background(), nil)
	if err != nil {
		t.Fatalf("startTransports: %v", err)
	}
	stop()
}

func TestDiscordIsATransportOnlyWhenTheDeploymentHasASession(t *testing.T) {
	if got := (&Agent{}).transports(); len(got) != 0 {
		t.Fatalf("no session must list no transport, got %d", len(got))
	}
	session, err := discordgo.New("Bot test")
	if err != nil {
		t.Fatalf("discordgo.New: %v", err)
	}
	got := (&Agent{session: session}).transports()
	if len(got) != 1 || got[0].Name() != transportDiscord {
		t.Fatalf("transports = %v, want exactly the discord transport", got)
	}
}

type narratedTurn struct {
	httpTurn
	sink TurnProgressSink
}

func (n *narratedTurn) ProgressSink() TurnProgressSink { return n.sink }

type recordedSink struct{}

func (recordedSink) Post(context.Context, string) (string, error) { return "", nil }
func (recordedSink) Edit(context.Context, string, string) error   { return nil }
func (recordedSink) Delete(context.Context, string) error         { return nil }

func TestProgressIsOfferedOnlyToATurnWhoseTransportCanNarrate(t *testing.T) {
	agent := &Agent{}
	if got := agent.progressFor(&httpTurn{}); got != nil {
		t.Fatal("a synchronous transport has nothing to narrate to")
	}
	if got := agent.progressFor(&discordMessageTurn{}); got != nil {
		t.Fatal("a Discord turn with no session has no line to edit")
	}
	if got := agent.progressFor(&narratedTurn{sink: nil}); got != nil {
		t.Fatal("a provider that returns no sink gets no progress line")
	}
	sink := recordedSink{}
	got := agent.progressFor(&narratedTurn{sink: sink})
	if got == nil || got.sink != sink {
		t.Fatal("a provider's sink must be the one the progress line posts through")
	}
}

func TestADiscordTurnNarratesThroughItsOwnMessage(t *testing.T) {
	session, err := discordgo.New("Bot test")
	if err != nil {
		t.Fatalf("discordgo.New: %v", err)
	}
	message := &discordgo.Message{ID: "1390000000000000009", ChannelID: "1390000000000000003"}
	turn := &discordMessageTurn{session: session, message: message}
	progress, ok := turn.ProgressSink().(discordTurnProgress)
	if !ok {
		t.Fatalf("sink = %T, want discordTurnProgress", turn.ProgressSink())
	}
	if progress.channel != message.ChannelID || progress.message != message || progress.session != session {
		t.Fatal("the sink must answer the summoning message in its own channel")
	}
}
