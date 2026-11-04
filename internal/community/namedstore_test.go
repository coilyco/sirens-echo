package community

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// storeLookupServer publishes find_trade as eco-app does since COI-3279: store and
// item vocabularies, templates ordered store+item, store, item. Calls are recorded.
func storeLookupServer(t *testing.T, calls *[]map[string]any) *MCPProvider {
	t.Helper()
	var mu sync.Mutex
	server := mcp.NewServer(&mcp.Implementation{Name: "eco-test", Version: "1"}, nil)
	vocabulary := func(uri string, entries []VocabEntry) {
		body, err := json.Marshal(map[string]any{"entries": entries})
		if err != nil {
			t.Fatal(err)
		}
		server.AddResource(
			&mcp.Resource{URI: uri, Name: uri, MIMEType: "application/json"},
			func(context.Context, *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
				return &mcp.ReadResourceResult{
					Contents: []*mcp.ResourceContents{{URI: uri, MIMEType: "application/json", Text: string(body)}},
				}, nil
			},
		)
	}
	vocabulary("eco://vocab/stores", []VocabEntry{
		{ID: "Food Coming Soon", Name: "Food Coming Soon"},
		{ID: "wrenhollow's Food Coming Soon", Name: "wrenhollow's Food Coming Soon"},
		{ID: "Recycle Mania", Name: "Recycle Mania"},
	})
	// No bare "Food": the real item vocabulary names Eco items, and "food" alone is no item.
	vocabulary("eco://vocab/items", []VocabEntry{
		{ID: "FoodScrapItem", Name: "Food Scrap"},
		{ID: "LimestoneItem", Name: "Limestone"},
		{ID: "MeatyStewItem", Name: "Meaty Stew"},
	})
	tool := &mcp.Tool{Name: "find_trade", Description: "trade", InputSchema: map[string]any{"type": "object"}}
	tool.Meta = mcp.Meta{
		replyTemplatesMetaKey: []any{
			map[string]any{"when_args": []any{"store", "item"}, "text": "{{reply}}"},
			map[string]any{"when_args": []any{"store"}, "text": "{{reply}}"},
			map[string]any{"when_args": []any{"item"}, "text": "The cheapest {{args.item}} is {{cheapest.0.price}} Credits."},
		},
		toolArgsMetaKey: map[string]any{
			"store": map[string]any{"vocabulary": "eco://vocab/stores", "field": "name", "ignore": []any{"store", "shop", "market"}},
			"item":  map[string]any{"vocabulary": "eco://vocab/items", "field": "name", "ignore": []any{"store", "shop", "market"}},
		},
	}
	server.AddTool(tool, func(_ context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		var args map[string]any
		_ = json.Unmarshal(req.Params.Arguments, &args)
		mu.Lock()
		*calls = append(*calls, args)
		mu.Unlock()
		structured := map[string]any{"cheapest": []any{map[string]any{"price": 3}}}
		if args["store"] != nil {
			structured = map[string]any{"reply": "Food Coming Soon: 265 listings, 150 in stock."}
		}
		return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: "summary"}}, StructuredContent: structured}, nil
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

func TestANamedStoreQuestionCallsTheStoreLookupNeverAnItemSearch(t *testing.T) {
	cases := map[string]map[string]any{
		// The words of the 10-31 and 11-02 misses: "food" is in the store's name.
		"stock check food coming soon":                                    {"store": "Food Coming Soon"},
		"I'm asking about the items in the store called food coming soon": {"store": "Food Coming Soon"},
		"what is in stock at Food Coming Soon?":                           {"store": "Food Coming Soon"},
		"stock check wrenhollow's food coming soon":                       {"store": "wrenhollow's Food Coming Soon"},
		"how much meaty stew at food coming soon":                         {"store": "Food Coming Soon", "item": "Meaty Stew"},
		"what is in stock at recycle mania":                               {"store": "Recycle Mania"},
	}
	for message, want := range cases {
		t.Run(message, func(t *testing.T) {
			var calls []map[string]any
			agent := testJevAgent(t)
			agent.cfg.JevDirectTools = true
			agent.tools = storeLookupServer(t, &calls)

			got, ok := directReplyText(agent, confidentTradePick(), message)

			if !ok || len(calls) != 1 {
				t.Fatalf("directToolReply = %q, %v after %d call(s), want one store lookup", got, ok, len(calls))
			}
			if len(calls[0]) != len(want) {
				t.Fatalf("arguments = %v, want %v", calls[0], want)
			}
			for name, value := range want {
				if calls[0][name] != value {
					t.Errorf("argument %s = %v, want %v (all: %v)", name, calls[0][name], value, calls[0])
				}
			}
			if _, searched := calls[0]["item"]; searched && want["item"] == nil {
				t.Errorf("a store question became an item search: %v", calls[0])
			}
		})
	}
}

func TestAQuestionNamingNoStoreStillTakesTheItemPath(t *testing.T) {
	var calls []map[string]any
	agent := testJevAgent(t)
	agent.cfg.JevDirectTools = true
	agent.tools = storeLookupServer(t, &calls)

	got, ok := directReplyText(agent, confidentTradePick(), "where can I buy limestone")

	if !ok || got != "The cheapest Limestone is 3 Credits." || len(calls) != 1 || calls[0]["store"] != nil {
		t.Fatalf("directToolReply = %q, %v, calls %v, want the item answer with no store", got, ok, calls)
	}
}

func TestAMisspelledStoreNameLeavesTheTurnToTheModelRatherThanGuessing(t *testing.T) {
	var calls []map[string]any
	agent := testJevAgent(t)
	agent.cfg.JevDirectTools = true
	agent.tools = storeLookupServer(t, &calls)

	// One letter off: no whole-word store form matches and "food" is no item, so
	// the direct path declines and the model path reaches find_trade's near match.
	typo := strings.Replace("stock check food coming soon", "min", "mmin", 1)
	got, ok := directReplyText(agent, confidentTradePick(), typo)

	if ok || len(calls) != 0 {
		t.Fatalf("directToolReply = %q, %v after %d call(s), want a decline with no call", got, ok, len(calls))
	}
}
