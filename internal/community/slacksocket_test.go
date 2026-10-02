package community

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/slack-go/slack"
)

// fakeSocketSlack serves apps.connections.open and one websocket that says
// hello, sends one envelope, and reports the acknowledgement it gets back.
type fakeSocketSlack struct {
	server   *httptest.Server
	envelope string
	acked    chan string
	openErr  string
}

func newFakeSocketSlack(t *testing.T, envelope, openErr string) *fakeSocketSlack {
	t.Helper()
	fake := &fakeSocketSlack{envelope: envelope, acked: make(chan string, 4), openErr: openErr}
	// slack-go dials with Origin https://api.slack.com, which the default check refuses.
	upgrader := websocket.Upgrader{CheckOrigin: func(*http.Request) bool { return true }}
	mux := http.NewServeMux()
	mux.HandleFunc("/api/apps.connections.open", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if fake.openErr != "" {
			_ = json.NewEncoder(w).Encode(map[string]any{"ok": false, "error": fake.openErr})
			return
		}
		url := "ws" + strings.TrimPrefix(fake.server.URL, "http") + "/ws"
		_ = json.NewEncoder(w).Encode(map[string]any{"ok": true, "url": url})
	})
	mux.HandleFunc("/ws", func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer func() { _ = conn.Close() }()
		_ = conn.WriteMessage(websocket.TextMessage, []byte(
			`{"type":"hello","num_connections":1,"connection_info":{"app_id":"A0TEST"},"debug_info":{}}`))
		if fake.envelope != "" {
			_ = conn.WriteMessage(websocket.TextMessage, []byte(fake.envelope))
		}
		for {
			_, message, err := conn.ReadMessage()
			if err != nil {
				return
			}
			fake.acked <- string(message)
		}
	})
	fake.server = httptest.NewServer(mux)
	t.Cleanup(fake.server.Close)
	return fake
}

func (f *fakeSocketSlack) client() *slack.Client {
	return slack.New("xoxb-test", slack.OptionAppLevelToken("xapp-test"),
		slack.OptionAPIURL(f.server.URL+"/api/"))
}

const mentionEnvelope = `{"type":"events_api","envelope_id":"env-1","accepts_response_payload":false,
"payload":{"token":"x","team_id":"T0COILYCO","type":"event_callback","event_id":"Ev1","event_time":1700000000,
"event":{"type":"app_mention","user":"U0KAI0001","text":"<@U0BOT0001> hi","ts":"1700000000.000100",
"channel":"C0GENERAL","event_ts":"1700000000.000100"}}}`

func TestSocketModeAcknowledgesTheEnvelopeBeforeTheTurnAndMapsTheMention(t *testing.T) {
	t.Parallel()
	fake := newFakeSocketSlack(t, mentionEnvelope, "")
	delivered := make(chan slackEvent, 1)
	release := make(chan struct{})
	stop, err := slackSocketSource{client: fake.client()}.Start(context.Background(), func(event slackEvent) {
		delivered <- event
		<-release // the turn is still running when the ack must already have gone out
	})
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	defer func() { close(release); stop() }()

	select {
	case event := <-delivered:
		want := slackEvent{
			Team: "T0COILYCO", Channel: "C0GENERAL", User: "U0KAI0001",
			TS: "1700000000.000100", Text: "<@U0BOT0001> hi",
		}
		if event != want {
			t.Fatalf("event = %+v, want %+v", event, want)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("the mention never arrived")
	}
	select {
	case ack := <-fake.acked:
		if !strings.Contains(ack, `"envelope_id":"env-1"`) {
			t.Fatalf("ack = %s", ack)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("the envelope was not acknowledged while the turn was still running")
	}
}

func TestAnInvalidAppTokenStopsStartInsteadOfRetryingForever(t *testing.T) {
	t.Parallel()
	fake := newFakeSocketSlack(t, "", "invalid_auth")
	started := time.Now()
	_, err := slackSocketSource{client: fake.client()}.Start(context.Background(), func(slackEvent) {})
	if err == nil {
		t.Fatal("an app token Slack rejects must stop the process")
	}
	if elapsed := time.Since(started); elapsed > slackConnectWait/2 {
		t.Fatalf("took %s to fail, want it well inside the connect wait", elapsed)
	}
}
