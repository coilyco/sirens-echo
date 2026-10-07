package community

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"sync/atomic"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func itemVocab() []VocabEntry {
	return []VocabEntry{
		{ID: "LimestoneItem", Name: "Limestone"},
		{ID: "CrushedLimestoneItem", Name: "Crushed Limestone"},
		{ID: "HewnLogItem", Name: "Hewn Log", Aliases: []string{"hewn logs"}},
		{ID: "IronBarItem", Name: "Iron Bar"},
		{ID: "IronOreItem", Name: "Iron Ore"},
	}
}

func TestMatchVocabPicksTheLongestWholeWordForm(t *testing.T) {
	cases := map[string]string{
		"where can I buy limestone":                      "LimestoneItem",
		"who sells crushed limestone cheapest":           "CrushedLimestoneItem",
		"what are the prices of hewn logs?":              "HewnLogItem",
		"who is selling iron bars cheapest right now":    "IronBarItem",
		"Where can I buy LIMESTONE, asking for a friend": "LimestoneItem",
	}
	for message, want := range cases {
		got, ok := matchVocab(message, itemVocab(), nil)
		if !ok || got.ID != want {
			t.Errorf("%q: got %q/%v, want %q", message, got.ID, ok, want)
		}
	}
}

func TestMatchVocabDeclinesOnNoMatchPartialWordsOrATie(t *testing.T) {
	cases := []string{
		"is the server up",           // nothing named
		"limestones are overrated??", // "limestones" matches, so this one is a positive control below
		"ironbar prices",             // not whole words of "iron bar"
		"iron bar or iron ore",       // two entries tie at two words
	}
	if _, ok := matchVocab(cases[0], itemVocab(), nil); ok {
		t.Errorf("%q matched, want nothing named", cases[0])
	}
	if got, ok := matchVocab(cases[1], itemVocab(), nil); !ok || got.ID != "LimestoneItem" {
		t.Errorf("%q: got %q/%v, want the plural to match", cases[1], got.ID, ok)
	}
	for _, message := range cases[2:] {
		if got, ok := matchVocab(message, itemVocab(), nil); ok {
			t.Errorf("%q matched %q, want a decline", message, got.ID)
		}
	}
}

func TestMatchVocabSkipsFormsTheToolDeclaresAsDomainWords(t *testing.T) {
	vocab := append(itemVocab(), VocabEntry{ID: "StoreItem", Name: "Store"}, VocabEntry{ID: "BrickItem", Name: "Brick"})
	ignore := map[string]bool{"store": true}

	if got, ok := matchVocab("can I buy bricks in one store and resell them in another", vocab, ignore); !ok || got.ID != "BrickItem" {
		t.Errorf("bricks in one store: got %q/%v, want Brick once store is a domain word", got.ID, ok)
	}
	if got, ok := matchVocab("what should I stock in my store to make money", vocab, ignore); ok {
		t.Errorf("my store: matched %q, want a decline", got.ID)
	}
	if got, ok := matchVocab("what should I stock in my store to make money", vocab, nil); !ok || got.ID != "StoreItem" {
		t.Errorf("control without ignore: got %q/%v, want Store, the measured wrong fill", got.ID, ok)
	}
}

func TestToolArgSpecsSkipsMalformedDeclarations(t *testing.T) {
	tool := &mcp.Tool{Name: "find_trade"}
	tool.Meta = mcp.Meta{toolArgsMetaKey: map[string]any{
		"item":     map[string]any{"vocabulary": "eco://vocab/items", "field": "name", "ignore": []any{"Store", 3}},
		"product":  map[string]any{"vocabulary": "eco://vocab/items", "field": "slug"},
		"currency": "not an object",
	}}

	specs := toolArgSpecs(tool)

	if len(specs) != 1 || specs["item"].Vocabulary != "eco://vocab/items" || specs["item"].Field != "name" {
		t.Fatalf("specs = %+v, want only the well-formed item declaration", specs)
	}
	if !specs["item"].Ignore["store"] || len(specs["item"].Ignore) != 1 {
		t.Errorf("ignore = %v, want the string form normalised and the number skipped", specs["item"].Ignore)
	}
}

// tradeServer publishes find_trade with an item-gated template, an args
// declaration, and the vocabulary resource it points at, as eco-app does.
func tradeServer(t *testing.T, reads *atomic.Int32) *MCPProvider {
	t.Helper()
	server := mcp.NewServer(&mcp.Implementation{Name: "eco-test", Version: "1"}, nil)
	body, err := json.Marshal(map[string]any{"entries": itemVocab()})
	if err != nil {
		t.Fatal(err)
	}
	server.AddResource(
		&mcp.Resource{URI: "eco://vocab/items", Name: "items", MIMEType: "application/json"},
		func(context.Context, *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
			reads.Add(1)
			return &mcp.ReadResourceResult{
				Contents: []*mcp.ResourceContents{{URI: "eco://vocab/items", MIMEType: "application/json", Text: string(body)}},
			}, nil
		},
	)
	tool := &mcp.Tool{Name: "find_trade", Description: "trade", InputSchema: map[string]any{"type": "object"}}
	tool.Meta = mcp.Meta{
		replyTemplatesMetaKey: []any{map[string]any{
			"when_args": []any{"item"},
			"text":      "The cheapest {{args.item}} is {{cheapest.0.price}} Credits.",
		}},
		toolArgsMetaKey: map[string]any{"item": map[string]any{"vocabulary": "eco://vocab/items", "field": "name"}},
	}
	server.AddTool(tool, func(_ context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		var args map[string]any
		_ = json.Unmarshal(req.Params.Arguments, &args)
		if args["item"] != "Limestone" {
			return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: "no match"}}, StructuredContent: map[string]any{"cheapest": []any{}}}, nil
		}
		return &mcp.CallToolResult{
			Content:           []mcp.Content{&mcp.TextContent{Text: "summary"}},
			StructuredContent: map[string]any{"cheapest": []any{map[string]any{"price": 3}}},
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

func confidentTradePick() RouteDecision {
	return RouteDecision{Ran: true, ToolServer: "eco", Tool: "find_trade", ToolProb: 0.96}
}

func TestDirectToolReplyFillsTheItemFromTheServersVocabulary(t *testing.T) {
	var reads atomic.Int32
	agent := testJevAgent(t)
	agent.cfg.JevDirectTools = true
	agent.tools = tradeServer(t, &reads)

	got, ok := directReplyText(agent, confidentTradePick(), "where can I buy limestone")
	if !ok || got != "The cheapest Limestone is 3 Credits." {
		t.Fatalf("directToolReply = %q, %v, want the item filled from the vocabulary", got, ok)
	}
	if _, ok := agent.directToolReply(context.Background(), confidentTradePick(), "where can I buy limestone"); !ok {
		t.Fatal("second identical turn declined, want the cached vocabulary to answer again")
	}
	if reads.Load() != 1 {
		t.Errorf("vocabulary reads = %d over two turns, want 1 from the cache", reads.Load())
	}
}

func TestDirectToolReplyDeclinesWhenNoItemIsNamed(t *testing.T) {
	var reads atomic.Int32
	agent := testJevAgent(t)
	agent.cfg.JevDirectTools = true
	agent.tools = tradeServer(t, &reads)

	if got, ok := agent.directToolReply(context.Background(), confidentTradePick(), "where can I buy stuff"); ok {
		t.Fatalf("directToolReply = %q, want a decline so the model path runs", got)
	}
}

// pricedServer is tradeServer's shape for price_by_stage: an item template, a
// literal miss template, and a vocabulary holding one priced item.
func pricedServer(t *testing.T, calls *atomic.Int32, miss map[string]any) *MCPProvider {
	t.Helper()
	server := mcp.NewServer(&mcp.Implementation{Name: "eco-test", Version: "1"}, nil)
	body, err := json.Marshal(map[string]any{"entries": []VocabEntry{{ID: "IronBarItem", Name: "Iron Bar", Aliases: []string{"Iron"}}}})
	if err != nil {
		t.Fatal(err)
	}
	server.AddResource(
		&mcp.Resource{URI: "eco://vocab/priced-items", Name: "priced", MIMEType: "application/json"},
		func(context.Context, *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
			return &mcp.ReadResourceResult{
				Contents: []*mcp.ResourceContents{{URI: "eco://vocab/priced-items", MIMEType: "application/json", Text: string(body)}},
			}, nil
		},
	)
	tool := &mcp.Tool{Name: "price_by_stage", Description: "price", InputSchema: map[string]any{"type": "object"}}
	tool.Meta = mcp.Meta{
		replyTemplatesMetaKey: []any{
			map[string]any{"when_args": []any{"item"}, "text": "{{reply}}"},
			miss,
		},
		toolArgsMetaKey: map[string]any{"item": map[string]any{"vocabulary": "eco://vocab/priced-items", "field": "name"}},
	}
	server.AddTool(tool, func(context.Context, *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		calls.Add(1)
		return &mcp.CallToolResult{
			Content:           []mcp.Content{&mcp.TextContent{Text: "summary"}},
			StructuredContent: map[string]any{"reply": "Iron Bar median Spectres by stage: Modern 4 0.58 (181)."},
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

func confidentPricePick() RouteDecision {
	return RouteDecision{Ran: true, ToolServer: "eco", Tool: "price_by_stage", ToolProb: 0.99}
}

var missTemplate = map[string]any{
	"when_unmatched": []any{"item"},
	"text":           "Couldn't match that to one Eco item with recorded trades.",
}

// sirens-echo#8424: an unknown word answered as Factorio, Minecraft and
// Satisfactory on the model path. The miss is now literal and calls nothing.
func TestDirectToolReplyAnswersAnUnmatchedWordFromTheMissTemplate(t *testing.T) {
	var calls atomic.Int32
	agent := testJevAgent(t)
	agent.cfg.JevDirectTools = true
	agent.tools = pricedServer(t, &calls, missTemplate)

	got, ok := directReplyText(agent, confidentPricePick(), "how much should I sell unobtainium for?")
	if !ok || got != "Couldn't match that to one Eco item with recorded trades." {
		t.Fatalf("directToolReply = %q, %v, want the literal miss", got, ok)
	}
	if calls.Load() != 0 {
		t.Errorf("tool calls = %d, want 0: a miss has nothing to call with", calls.Load())
	}
}

func TestDirectToolReplyPrefersTheItemTemplateWhenTheWordMatches(t *testing.T) {
	var calls atomic.Int32
	agent := testJevAgent(t)
	agent.cfg.JevDirectTools = true
	agent.tools = pricedServer(t, &calls, missTemplate)

	got, ok := directReplyText(agent, confidentPricePick(), "how much should I sell iron for?")
	if !ok || got != "Iron Bar median Spectres by stage: Modern 4 0.58 (181)." {
		t.Fatalf("directToolReply = %q, %v, want the item template", got, ok)
	}
	if calls.Load() != 1 {
		t.Errorf("tool calls = %d, want 1", calls.Load())
	}
}

func TestAMissTemplateWithAPlaceholderIsDropped(t *testing.T) {
	var calls atomic.Int32
	agent := testJevAgent(t)
	agent.cfg.JevDirectTools = true
	agent.tools = pricedServer(t, &calls, map[string]any{"when_unmatched": []any{"item"}, "text": "No {{args.item}}."})

	if got, ok := agent.directToolReply(context.Background(), confidentPricePick(), "sell unobtainium"); ok {
		t.Fatalf("directToolReply = %q, want a decline: a miss template cannot be filled", got)
	}
}

// stagedServer is price_by_stage after eco-app#8425: an item vocabulary where
// `au3` is itself an item, and a stage vocabulary where `au3` is a stage.
func stagedServer(t *testing.T, got *[]map[string]any) *MCPProvider {
	t.Helper()
	server := mcp.NewServer(&mcp.Implementation{Name: "eco-test", Version: "1"}, nil)
	vocabs := map[string][]VocabEntry{
		"eco://vocab/priced-items": {
			{ID: "IronBarItem", Name: "Iron Bar", Aliases: []string{"Iron"}},
			{ID: "CopperBarItem", Name: "Copper Bar", Aliases: []string{"Copper"}},
			{ID: "GoldBarItem", Name: "Gold Bar", Aliases: []string{"Gold"}},
			{ID: "AdvancedUpgradeLvl3Item", Name: "Advanced Upgrade 3", Aliases: []string{"au3", "au 3"}},
		},
		"eco://vocab/stages": {{ID: "Advanced 3", Name: "Advanced 3", Aliases: []string{"au3", "au 3"}}},
	}
	for uri, entries := range vocabs {
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
	tool := &mcp.Tool{Name: "price_by_stage", Description: "price", InputSchema: map[string]any{"type": "object"}}
	tool.Meta = mcp.Meta{
		replyTemplatesMetaKey: []any{
			map[string]any{"when_args": []any{"stage", "item"}, "text": "{{args.item}} at {{args.stage}}."},
			map[string]any{"when_args": []any{"item"}, "text": "{{args.item}}."},
			missTemplate,
		},
		toolArgsMetaKey: map[string]any{
			"item":  map[string]any{"vocabulary": "eco://vocab/priced-items", "field": "name"},
			"stage": map[string]any{"vocabulary": "eco://vocab/stages", "field": "name"},
		},
	}
	server.AddTool(tool, func(_ context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		var args map[string]any
		_ = json.Unmarshal(req.Params.Arguments, &args)
		*got = append(*got, args)
		return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: "ok"}}, StructuredContent: map[string]any{}}, nil
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

// The words one argument used cannot fill the next, so "iron at au3" is
// Iron Bar at Advanced 3 and not a tie between iron and Advanced Upgrade 3.
func TestDirectToolReplyFillsTheStageThenTheItemFromWhatIsLeft(t *testing.T) {
	cases := []struct{ message, want string }{
		{"how much should I sell iron at au3?", "Iron Bar at Advanced 3."},
		{"what does iron go for at AU 3", "Iron Bar at Advanced 3."},
		{"how much for au3?", "Advanced Upgrade 3."},
	}
	for _, tc := range cases {
		t.Run(tc.message, func(t *testing.T) {
			var got []map[string]any
			agent := testJevAgent(t)
			agent.cfg.JevDirectTools = true
			agent.tools = stagedServer(t, &got)
			reply, ok := directReplyText(agent, confidentPricePick(), tc.message)
			if !ok || reply != tc.want {
				t.Fatalf("directToolReply = %q, %v, want %q (calls %v)", reply, ok, tc.want, got)
			}
		})
	}
}

// Two items named is not no item named, so the literal miss must not fire:
// each item is answered on its own (COI-2112).
func TestDirectToolReplyAnswersEachNamedItemOnItsOwn(t *testing.T) {
	cases := []struct {
		message string
		want    []string
		calls   []map[string]any
	}{
		{
			"price check on Iron Bar & Gold Bar?",
			[]string{"Iron Bar.", "Gold Bar."},
			[]map[string]any{{"item": "Iron Bar"}, {"item": "Gold Bar"}},
		},
		{
			"price check on iron and copper?",
			[]string{"Iron Bar.", "Copper Bar."},
			[]map[string]any{{"item": "Iron Bar"}, {"item": "Copper Bar"}},
		},
		{
			"gold, iron and copper at au3",
			[]string{"Gold Bar at Advanced 3.", "Iron Bar at Advanced 3.", "Copper Bar at Advanced 3."},
			[]map[string]any{
				{"item": "Gold Bar", "stage": "Advanced 3"},
				{"item": "Iron Bar", "stage": "Advanced 3"},
				{"item": "Copper Bar", "stage": "Advanced 3"},
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.message, func(t *testing.T) {
			var got []map[string]any
			model := &noModelCompletions{}
			agent := testJevAgent(t)
			agent.cfg.JevDirectTools = true
			agent.completions = model
			agent.tools = stagedServer(t, &got)

			replies, ok := agent.directToolReply(context.Background(), confidentPricePick(), tc.message)

			if !ok || !reflect.DeepEqual(replies, tc.want) {
				t.Fatalf("directToolReply = %q, %v, want %q", replies, ok, tc.want)
			}
			if !reflect.DeepEqual(got, tc.calls) {
				t.Errorf("price_by_stage calls = %v, want one per item %v", got, tc.calls)
			}
			if model.calls.Load() != 0 {
				t.Errorf("model calls = %d, want 0", model.calls.Load())
			}
		})
	}
}

// A tie that is one ambiguous item, or too many items, still takes the model
// path rather than answering a partial.
func TestDirectToolReplyDeclinesWhatItCannotAnswerPerItem(t *testing.T) {
	vocab := []VocabEntry{
		{ID: "IronBarItem", Name: "Iron Bar", Aliases: []string{"Bar"}},
		{ID: "GoldBarItem", Name: "Gold Bar", Aliases: []string{"Bar"}},
	}
	if hits, ok := matchVocabAll(vocabWords("price of a bar"), vocab, nil); ok {
		t.Errorf("one word claimed by two items matched %v, want a decline", hits)
	}
	cases := map[string]string{"three items past a cap of two": "price check on iron, copper and gold"}
	for name, message := range cases {
		t.Run(name, func(t *testing.T) {
			var got []map[string]any
			agent := testJevAgent(t)
			agent.cfg.JevDirectTools = true
			agent.tools = stagedServer(t, &got)
			defer func(was int) { maxDirectItems = was }(maxDirectItems)
			maxDirectItems = 2

			if replies, ok := agent.directToolReply(context.Background(), confidentPricePick(), message); ok {
				t.Fatalf("directToolReply = %q, want a decline (calls %v)", replies, got)
			}
			if len(got) != 0 {
				t.Errorf("price_by_stage calls = %v, want none before the decline", got)
			}
		})
	}
}
