package community

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/bwmarrin/discordgo"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// A name a tool returned is the tool's, so the harness must not turn it into a
// ping the model only quoted. Names and ids are invented. See COI-3283.

const (
	ownerID  = "500000000000000001"
	bystand  = "500000000000000002"
	tradeLog = `{"stores":[{"name":"Zephyrine Iron Works","owner":"zephyrine","item":"Iron Bar","price":3}]}`
	tradeSay = "Zephyrine sells Iron Bar at 3 coins."
)

// tradeRoster is the conversation: the store's owner, who is also a member of
// it, and a second member who only talked.
func tradeRoster() mentionRoster {
	roster := mentionRoster{}
	roster.add("zephyrine", ownerID)
	roster.add("zeph99", ownerID)
	roster.add("bravo", bystand)
	return roster
}

func TestAStoreOwnerNamedByAToolIsNotMentioned(t *testing.T) {
	t.Parallel()
	ctx, _ := withToolSourced(context.Background())
	noteToolSourced(ctx, tradeLog)

	reply, mentioned := resolveReplyMentions(ctx, tradeRoster(), tradeSay)

	if strings.Contains(reply, "<@") || len(mentioned) != 0 {
		t.Fatalf("a tool-sourced name resolved: reply %q, mentioned %v", reply, mentioned)
	}
	if reply != tradeSay {
		t.Errorf("the reply was altered: %q", reply)
	}
}

// One spelling in the tool result stops the member's other spellings, because
// the model can write whichever it likes.
func TestTheWholeAccountIsDroppedNotJustTheMatchingName(t *testing.T) {
	t.Parallel()
	ctx, _ := withToolSourced(context.Background())
	noteToolSourced(ctx, `{"owner":"ZEPH99"}`)

	reply, mentioned := resolveReplyMentions(ctx, tradeRoster(), "zephyrine sells it.")

	if strings.Contains(reply, "<@") || len(mentioned) != 0 {
		t.Fatalf("an alias of a tool-sourced account resolved: %q, %v", reply, mentioned)
	}
}

// The documented behavior stays for names that only the conversation supplied.
func TestAConversationOnlyNameStillResolves(t *testing.T) {
	t.Parallel()
	ctx, _ := withToolSourced(context.Background())
	noteToolSourced(ctx, tradeLog)

	reply, mentioned := resolveReplyMentions(ctx, tradeRoster(), "Zephyrine sells it, and bravo asked.")

	if !strings.Contains(reply, "<@"+bystand+">") {
		t.Errorf("a conversation-only name no longer resolves: %q", reply)
	}
	if len(mentioned) != 1 || mentioned[0] != bystand {
		t.Errorf("mentioned = %v, want only the bystander", mentioned)
	}
}

type sentMessage struct {
	Content         string `json:"content"`
	AllowedMentions struct {
		Parse []string `json:"parse"`
		Users []string `json:"users"`
	} `json:"allowed_mentions"`
}

// sentThroughDiscord runs the real send and returns the body Discord would get.
func sentThroughDiscord(t *testing.T, ctx context.Context, content string) sentMessage {
	t.Helper()
	session, err := discordgo.New("Bot test-token")
	if err != nil {
		t.Fatalf("discordgo.New: %v", err)
	}
	capture := &capturingTransport{}
	session.Client = &http.Client{Transport: capture}
	turn := &discordMessageTurn{
		session:  session,
		message:  &discordgo.Message{ID: "m-1", ChannelID: "c-1"},
		mentions: tradeRoster(),
	}
	_ = turn.Reply(ctx, content)
	var sent sentMessage
	if err := json.Unmarshal([]byte(capture.body), &sent); err != nil {
		t.Fatalf("the request was not JSON: %v: %q", err, capture.body)
	}
	return sent
}

func TestTheSentMessageCarriesNoMentionForAToolSourcedName(t *testing.T) {
	t.Parallel()
	ctx, _ := withToolSourced(context.Background())
	noteToolSourced(ctx, tradeLog)

	sent := sentThroughDiscord(t, ctx, tradeSay)

	if strings.Contains(sent.Content, "<@") {
		t.Errorf("content = %q, want no mention", sent.Content)
	}
	if len(sent.AllowedMentions.Parse) != 0 || len(sent.AllowedMentions.Users) != 0 {
		t.Errorf("allowed_mentions = %+v, want an empty parse and no users", sent.AllowedMentions)
	}
}

// The control: with no tool text the same send mentions the owner, so the test
// above is catching the fix and not an unrelated absence.
func TestTheSameSendWithNoToolTextStillMentionsTheOwner(t *testing.T) {
	t.Parallel()
	sent := sentThroughDiscord(t, context.Background(), tradeSay)

	if !strings.Contains(sent.Content, "<@"+ownerID+">") {
		t.Fatalf("content = %q, the control no longer mentions the owner", sent.Content)
	}
	if len(sent.AllowedMentions.Users) != 1 || sent.AllowedMentions.Users[0] != ownerID {
		t.Errorf("users = %v, want the owner", sent.AllowedMentions.Users)
	}
}

// mentionTurn resolves through the same function send calls, so a turn that
// runs end to end shows what would have been delivered.
type mentionTurn struct {
	markableTurn
	roster    mentionRoster
	content   string
	mentioned []string
}

func (t *mentionTurn) Reply(ctx context.Context, content string) error {
	t.content, t.mentioned = resolveReplyMentions(ctx, t.roster, content)
	return nil
}

type toolThenAnswerClient struct{ tools []ExecutedTool }

func (c toolThenAnswerClient) Complete(context.Context, TurnPrompt, string) (CompletionResult, error) {
	return CompletionResult{Content: tradeSay, ToolCalls: c.tools}, nil
}

func TestAModelTurnWhoseToolNamedTheOwnerDoesNotMentionThem(t *testing.T) {
	t.Parallel()
	agent := silentTurnAgent(toolThenAnswerClient{tools: []ExecutedTool{{
		Name: "eco__find_trade", Result: tradeLog, Outcome: ToolOutcomeOK, ReadOnly: true,
	}}})
	turn := &mentionTurn{roster: tradeRoster()}

	if err := agent.runTurn(context.Background(), turn, nil); err != nil {
		t.Fatalf("runTurn: %v", err)
	}
	if turn.content == "" {
		t.Fatal("the turn sent nothing")
	}
	if strings.Contains(turn.content, "<@") || len(turn.mentioned) != 0 {
		t.Errorf("content %q, mentioned %v, want no mention", turn.content, turn.mentioned)
	}
}

func TestAModelTurnWithNoToolStillMentionsAConversationName(t *testing.T) {
	t.Parallel()
	agent := silentTurnAgent(toolThenAnswerClient{})
	turn := &mentionTurn{roster: tradeRoster()}

	if err := agent.runTurn(context.Background(), turn, nil); err != nil {
		t.Fatalf("runTurn: %v", err)
	}
	if !strings.Contains(turn.content, "<@"+ownerID+">") {
		t.Errorf("content = %q, the roster feature no longer resolves a conversation name", turn.content)
	}
}

// ownerServer serves a find_trade-shaped result whose structured content names
// the store's owner, which a reply template then renders verbatim.
func ownerServer(t *testing.T, templates []any) *MCPProvider {
	t.Helper()
	server := mcp.NewServer(&mcp.Implementation{Name: "eco-test", Version: "1"}, nil)
	tool := &mcp.Tool{Name: "find_trade", Description: "trade", InputSchema: map[string]any{"type": "object"}}
	tool.Meta = mcp.Meta{replyTemplatesMetaKey: templates}
	server.AddTool(tool, func(context.Context, *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: "summary"}},
			StructuredContent: map[string]any{
				"store": map[string]any{"owner": "zephyrine", "item": "Iron Bar", "price": 3},
			},
		}, nil
	})
	httpServer := httptest.NewServer(mcp.NewStreamableHTTPHandler(
		func(*http.Request) *mcp.Server { return server },
		&mcp.StreamableHTTPOptions{JSONResponse: true},
	))
	t.Cleanup(httpServer.Close)
	provider := &MCPProvider{Servers: []MCPServerDefinition{{Name: "eco", URL: httpServer.URL}}}
	t.Cleanup(func() { _ = provider.Close() })
	session, err := provider.Open(context.Background())
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	_ = session.Close()
	return provider
}

// The direct-tool route reaches the send with no model and no CompletionResult,
// so a fix on the model path alone would leave it pinging.
func TestADirectToolReplyNamingTheOwnerDoesNotMentionThem(t *testing.T) {
	agent := testJevAgent(t)
	agent.cfg.JevDirectTools = true
	agent.tools = ownerServer(t, []any{map[string]any{"text": "{{store.owner}} sells {{store.item}} at {{store.price}} coins."}})
	route := RouteDecision{Ran: true, ToolServer: "eco", Tool: "find_trade", ToolProb: 0.95}
	ctx, _ := withToolSourced(context.Background())

	replies, ok := agent.directToolReply(ctx, route, "who sells iron")
	if !ok || len(replies) != 1 || !strings.Contains(replies[0], "zephyrine") {
		t.Fatalf("directToolReply = %q, %v, want a reply naming the owner", replies, ok)
	}
	reply, mentioned := resolveReplyMentions(ctx, tradeRoster(), replies[0])

	if strings.Contains(reply, "<@") || len(mentioned) != 0 {
		t.Errorf("the direct route mentioned a tool-sourced owner: %q, %v", reply, mentioned)
	}
	// The same reply with the route's text unrecorded would have pinged them.
	bare, _ := resolveReplyMentions(context.Background(), tradeRoster(), replies[0])
	if !strings.Contains(bare, "<@"+ownerID+">") {
		t.Errorf("control: %q, the fixture does not reach the bug", bare)
	}
}
