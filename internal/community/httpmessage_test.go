package community

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/bwmarrin/discordgo"
)

// Caller-visible guarantees of the message surface. See docs/sirens-echo-http.md.

const testMessageChannelID = "123456789012345678"

// sentToDiscord is what a fake Discord saw, so a refusal can be proven to have
// reached nothing rather than merely to have answered 4xx.
type sentToDiscord struct {
	mutex sync.Mutex
	paths []string
	body  []byte
}

func (s *sentToDiscord) count() int {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	return len(s.paths)
}

// messageAgent answers every Discord send with the given status and body.
func messageAgent(t *testing.T, status int, reply string) (*Agent, *sentToDiscord) {
	t.Helper()
	session, err := discordgo.New("Bot test-token")
	if err != nil {
		t.Fatalf("discordgo.New: %v", err)
	}
	seen := &sentToDiscord{}
	session.Client = &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
		body, _ := io.ReadAll(request.Body)
		seen.mutex.Lock()
		seen.paths = append(seen.paths, request.URL.Path)
		seen.body = body
		seen.mutex.Unlock()
		return &http.Response{StatusCode: status, Body: io.NopCloser(strings.NewReader(reply)),
			Header: http.Header{"Content-Type": {"application/json"}}, Request: request}, nil
	})}
	agent := turnAgent(Config{
		RateLimit:       defaultRateLimitPolicy,
		MessageChannels: map[string]string{"GAMING_ARPGS": testMessageChannelID},
	})
	agent.session = session
	return agent, seen
}

func postMessage(agent *Agent, body string) *httptest.ResponseRecorder {
	request := httptest.NewRequest(http.MethodPost, httpMessagePath, strings.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	agent.HTTPHandler().ServeHTTP(recorder, request)
	return recorder
}

func TestMessageIsPostedVerbatimWithEmojiAndNoMentions(t *testing.T) {
	t.Parallel()
	agent, seen := messageAgent(t, http.StatusOK, `{"id":"900000000000000001"}`)
	// The three glyphs the daily restart sends, plus a mention that must stay text.
	content := "⏳ restart primed ❌ down ✅ up @everyone <@123456789012345678>"
	encoded, _ := json.Marshal(map[string]string{"channel": "gaming-arpgs", "content": content})
	recorder := postMessage(agent, string(encoded))
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, body %q", recorder.Code, recorder.Body.String())
	}
	if !strings.Contains(recorder.Body.String(), `"message_id":"900000000000000001"`) {
		t.Errorf("response omits the message id: %q", recorder.Body.String())
	}
	if len(seen.paths) != 1 || !strings.HasSuffix(seen.paths[0], "/channels/"+testMessageChannelID+"/messages") {
		t.Fatalf("Discord saw %v, want one post to the allowlisted channel", seen.paths)
	}
	var sent struct {
		Content         string `json:"content"`
		AllowedMentions *struct {
			Parse *[]string `json:"parse"`
		} `json:"allowed_mentions"`
	}
	if err := json.Unmarshal(seen.body, &sent); err != nil {
		t.Fatalf("Discord request body: %v", err)
	}
	if sent.Content != content {
		t.Errorf("content = %q, want it untouched: %q", sent.Content, content)
	}
	if sent.AllowedMentions == nil || sent.AllowedMentions.Parse == nil || len(*sent.AllowedMentions.Parse) != 0 {
		t.Errorf("allowed_mentions.parse must be present and empty, body %s", seen.body)
	}
}

func TestMessageChannelAcceptsEverySpellingOfTheSlug(t *testing.T) {
	t.Parallel()
	for _, channel := range []string{"gaming-arpgs", "#gaming-arpgs", "  #Gaming-ARPGS ", "gaming_arpgs"} {
		agent, seen := messageAgent(t, http.StatusOK, `{"id":"1"}`)
		body, _ := json.Marshal(map[string]string{"channel": channel, "content": "hi"})
		if recorder := postMessage(agent, string(body)); recorder.Code != http.StatusOK {
			t.Errorf("channel %q: status = %d, body %q", channel, recorder.Code, recorder.Body.String())
		}
		if seen.count() != 1 {
			t.Errorf("channel %q: Discord saw %d posts, want 1", channel, seen.count())
		}
	}
}

func TestMessageRefusesAChannelOutsideTheAllowlistAndNamesTheSlug(t *testing.T) {
	t.Parallel()
	agent, seen := messageAgent(t, http.StatusOK, `{"id":"1"}`)
	recorder := postMessage(agent, `{"channel":"#general","content":"hi"}`)
	if recorder.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403", recorder.Code)
	}
	if !strings.Contains(recorder.Body.String(), `channel "general" is not allowed`) {
		t.Errorf("refusal does not name the slug the caller sent: %q", recorder.Body.String())
	}
	for _, leaked := range []string{"GAMING", "ARPGS", "SIRENS_ECHO", testMessageChannelID} {
		if strings.Contains(recorder.Body.String(), leaked) {
			t.Errorf("refusal leaks the allowlist (%q): %q", leaked, recorder.Body.String())
		}
	}
	if seen.count() != 0 {
		t.Errorf("a refused post reached Discord %d times", seen.count())
	}
}

func TestMessageNeverEchoesAChannelThatIsNotShapedLikeOne(t *testing.T) {
	t.Parallel()
	agent, seen := messageAgent(t, http.StatusOK, `{"id":"1"}`)
	for name, channel := range map[string]string{
		"spaces":    "gaming arpgs",
		"newline":   "gaming-arpgs\nX-Injected: 1",
		"non-ascii": "gaming-arpgsé",
		"too long":  strings.Repeat("a", 101),
		"traversal": "../gaming-arpgs",
	} {
		body, _ := json.Marshal(map[string]string{"channel": channel, "content": "hi"})
		recorder := postMessage(agent, string(body))
		if recorder.Code != http.StatusForbidden {
			t.Errorf("%s: status = %d, want 403", name, recorder.Code)
		}
		if got := strings.TrimSpace(recorder.Body.String()); got != "channel is not allowed" {
			t.Errorf("%s: body = %q, want the unnamed refusal", name, got)
		}
	}
	if seen.count() != 0 {
		t.Errorf("a refused post reached Discord %d times", seen.count())
	}
}

func TestMessageRejectsEveryCallerError(t *testing.T) {
	t.Parallel()
	tooLong := strings.Repeat("a", discordReplyLimit+1)
	atLimit := strings.Repeat("é", discordReplyLimit)
	cases := map[string]struct {
		body   string
		status int
		want   string
	}{
		"not json":        {`not json`, 400, "request body must be a JSON object"},
		"unknown field":   {`{"channel":"gaming-arpgs","content":"x","tts":true}`, 400, "unknown field: tts"},
		"no channel":      {`{"content":"x"}`, 400, "channel is required"},
		"blank channel":   {`{"channel":" ","content":"x"}`, 400, "channel is required"},
		"no content":      {`{"channel":"gaming-arpgs"}`, 400, "content is required"},
		"blank content":   {`{"channel":"gaming-arpgs","content":" \n"}`, 400, "content is required"},
		"content too big": {`{"channel":"gaming-arpgs","content":"` + tooLong + `"}`, 400, "character limit"},
	}
	for name, tc := range cases {
		agent, seen := messageAgent(t, http.StatusOK, `{"id":"1"}`)
		recorder := postMessage(agent, tc.body)
		if recorder.Code != tc.status || !strings.Contains(recorder.Body.String(), tc.want) {
			t.Errorf("%s: got %d %q, want %d containing %q",
				name, recorder.Code, recorder.Body.String(), tc.status, tc.want)
		}
		if seen.count() != 0 {
			t.Errorf("%s: a rejected body reached Discord", name)
		}
	}
	// The cap counts runes, so a message of exactly the limit in two-byte
	// characters is admitted.
	agent, seen := messageAgent(t, http.StatusOK, `{"id":"1"}`)
	body, _ := json.Marshal(map[string]string{"channel": "gaming-arpgs", "content": atLimit})
	if recorder := postMessage(agent, string(body)); recorder.Code != http.StatusOK || seen.count() != 1 {
		t.Errorf("content at the limit: status = %d, posts = %d", recorder.Code, seen.count())
	}
}

func TestMessageRejectsEveryMethodButPOST(t *testing.T) {
	t.Parallel()
	agent, _ := messageAgent(t, http.StatusOK, `{"id":"1"}`)
	for _, method := range []string{http.MethodGet, http.MethodPut, http.MethodDelete} {
		recorder := httptest.NewRecorder()
		agent.HTTPHandler().ServeHTTP(recorder, httptest.NewRequest(method, httpMessagePath, nil))
		if recorder.Code != http.StatusMethodNotAllowed || recorder.Header().Get("Allow") != http.MethodPost {
			t.Errorf("%s: got %d Allow=%q", method, recorder.Code, recorder.Header().Get("Allow"))
		}
	}
}

// Discord's own answer can carry channel and guild detail, so a failed post
// stays generic and the 5xx is the service's to answer for.
func TestMessageReportsADiscordFailureWithoutItsBody(t *testing.T) {
	t.Parallel()
	agent, _ := messageAgent(t, http.StatusForbidden, `{"message":"Missing Access","code":50001}`)
	recorder := postMessage(agent, `{"channel":"gaming-arpgs","content":"hi"}`)
	if recorder.Code != http.StatusBadGateway {
		t.Fatalf("status = %d, want 502", recorder.Code)
	}
	if got := strings.TrimSpace(recorder.Body.String()); got != "message could not be sent" {
		t.Errorf("body = %q, want the generic sentence", got)
	}
}

func TestMessageWithDiscordDisabledIsUnavailableNotACallerError(t *testing.T) {
	t.Parallel()
	agent := turnAgent(Config{MessageChannels: map[string]string{"GAMING_ARPGS": testMessageChannelID}})
	recorder := postMessage(agent, `{"channel":"gaming-arpgs","content":"hi"}`)
	if recorder.Code != http.StatusServiceUnavailable {
		t.Errorf("status = %d, want 503", recorder.Code)
	}
}

func TestMessageWithNoAllowlistReachesNothing(t *testing.T) {
	t.Parallel()
	agent, seen := messageAgent(t, http.StatusOK, `{"id":"1"}`)
	agent.cfg.MessageChannels = nil
	recorder := postMessage(agent, `{"channel":"gaming-arpgs","content":"hi"}`)
	if recorder.Code != http.StatusForbidden || seen.count() != 0 {
		t.Errorf("status = %d, posts = %d, want 403 and none", recorder.Code, seen.count())
	}
}

func TestNormalizeChannelSlug(t *testing.T) {
	t.Parallel()
	for raw, want := range map[string]string{
		"gaming-arpgs":    "GAMING_ARPGS",
		"#gaming-arpgs":   "GAMING_ARPGS",
		" #Gaming-Arpgs ": "GAMING_ARPGS",
		"a_b-c1":          "A_B_C1",
		"gaming--arpgs":   "GAMING__ARPGS",
	} {
		if key, _, ok := normalizeChannelSlug(raw); !ok || key != want {
			t.Errorf("normalizeChannelSlug(%q) = %q, %v; want %q", raw, key, ok, want)
		}
	}
	for _, raw := range []string{"", "#", "##x", "a b", "é", strings.Repeat("a", 101)} {
		if _, _, ok := normalizeChannelSlug(raw); ok {
			t.Errorf("normalizeChannelSlug(%q) accepted a slug that is not a channel name", raw)
		}
	}
}

func TestMessageChannelsFromEnv(t *testing.T) {
	t.Parallel()
	channels, err := messageChannelsFromEnv([]string{
		"SIRENS_ECHO_MESSAGE_CHANNEL_GAMING_ARPGS= 123456789012345678 ",
		"SIRENS_ECHO_MESSAGE_CHANNEL_EMPTY=",
		"SIRENS_ECHO_MESSAGE_CHANNEL_=123456789012345678",
		"SIRENS_ECHO_HTTP_TOKEN=not-a-channel",
		"UNRELATED=1",
	})
	if err != nil {
		t.Fatalf("messageChannelsFromEnv: %v", err)
	}
	if len(channels) != 1 || channels["GAMING_ARPGS"] != "123456789012345678" {
		t.Errorf("channels = %v, want only GAMING_ARPGS", channels)
	}
}

func TestMessageChannelsFromEnvStopsTheBootOnANonChannelValue(t *testing.T) {
	t.Parallel()
	_, err := messageChannelsFromEnv([]string{"SIRENS_ECHO_MESSAGE_CHANNEL_GAMING_ARPGS=#gaming-arpgs"})
	if err == nil || !strings.Contains(err.Error(), "SIRENS_ECHO_MESSAGE_CHANNEL_GAMING_ARPGS") {
		t.Fatalf("err = %v, want one naming the variable", err)
	}
	if strings.Contains(err.Error(), "#gaming-arpgs") {
		t.Errorf("the error carries the value: %v", err)
	}
}
