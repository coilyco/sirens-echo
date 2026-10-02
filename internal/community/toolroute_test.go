package community

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/coilyco/sirens-echo/internal/community/systemone"
)

func ecoListing() CachedServerTools {
	return CachedServerTools{
		Server:   "eco",
		Guidance: "Live data for the Eco server.",
		Tools: []*mcp.Tool{
			{Name: "find_trade", Description: "Where to buy or sell an item."},
			{Name: "get_market", Description: "How an item's price has moved."},
		},
	}
}

func wikiListing() CachedServerTools {
	return CachedServerTools{
		Server: "wiki",
		Tools:  []*mcp.Tool{{Name: "search", Description: "Search the wiki."}},
	}
}

func TestToolRouteQuestionsAskOneToolChoicePerServerAndNoServerPick(t *testing.T) {
	questions, meta := toolRouteQuestions([]CachedServerTools{ecoListing(), wikiListing()}, nil)

	if len(questions) != 2 {
		t.Fatalf("questions = %d, want one tool pick per server and no separate server pick", len(questions))
	}
	pick := questions[0]
	if pick.Key != toolPickKeyPrefix+"eco" || pick.Type != systemone.TypeChoice {
		t.Fatalf("first question = %s/%s, want the eco tool choice", pick.Key, pick.Type)
	}
	if names := criterionNames(pick.Criteria); fmt.Sprint(names) != "[find_trade get_market no_tool]" {
		t.Errorf("tool options = %v, want the listed tools plus no_tool", names)
	}
	if !strings.Contains(pick.Prompt, "Live data for the Eco server.") {
		t.Errorf("prompt = %q, want the server's own guidance carried in", pick.Prompt)
	}
	if meta[pick.Key].target != "eco" || meta[pick.Key].family != RouteFamilyTool {
		t.Errorf("meta for %s = %+v, want the tool family targeting eco", pick.Key, meta[pick.Key])
	}
}

func TestToolRouteQuestionsSkipAServerPastJevsChoiceSize(t *testing.T) {
	big := CachedServerTools{Server: "huge"}
	for i := range jevToolMaxOptions {
		big.Tools = append(big.Tools, &mcp.Tool{Name: fmt.Sprintf("t%d", i)})
	}
	questions, _ := toolRouteQuestions([]CachedServerTools{big, ecoListing()}, nil)

	if len(questions) != 1 || questions[0].Key != toolPickKeyPrefix+"eco" {
		t.Fatalf("questions = %v, want only eco once the oversize server is left out", questionKeys(questions))
	}
}

func TestToolRouteQuestionsLeaveOutSkippedTools(t *testing.T) {
	eco := ecoListing()
	eco.Tools = append(eco.Tools, &mcp.Tool{Name: "mcp_beaver_info", Description: "doubles as a liveness probe"})
	wiki := wikiListing()
	wiki.Tools = append(wiki.Tools, &mcp.Tool{Name: "mcp_beaver_info"})

	questions, _ := toolRouteQuestions([]CachedServerTools{eco, wiki}, []string{"mcp_beaver_info", "eco__get_market"})

	if names := criterionNames(questions[0].Criteria); fmt.Sprint(names) != "[find_trade no_tool]" {
		t.Errorf("eco options = %v, want the meta tool and the server-scoped skip left out", names)
	}
	if names := criterionNames(questions[1].Criteria); fmt.Sprint(names) != "[search no_tool]" {
		t.Errorf("wiki options = %v, want the meta tool left out on every server", names)
	}
}

func TestToolRouteQuestionsAskNothingWithoutAListing(t *testing.T) {
	if questions, _ := toolRouteQuestions(nil, nil); len(questions) != 0 {
		t.Fatalf("questions = %d with no cached listing, want none", len(questions))
	}
}

func TestApplyToolAnswersTakesTheMostConfidentServerAndHonoursRivals(t *testing.T) {
	_, meta := toolRouteQuestions([]CachedServerTools{ecoListing(), wikiListing()}, nil)
	cases := []struct {
		name       string
		eco, wiki  systemone.Answer
		wantServer string
		wantDirect bool
	}{
		{"eco clears, wiki declines", systemone.Answer{Option: "find_trade", Probability: 0.94}, systemone.Answer{Option: toolNoToolOption, Probability: 0.9}, "eco", true},
		{"eco under the bar", systemone.Answer{Option: "find_trade", Probability: 0.82}, systemone.Answer{Option: toolNoToolOption, Probability: 0.9}, "eco", false},
		{"wiki contests eco", systemone.Answer{Option: "find_trade", Probability: 0.95}, systemone.Answer{Option: "search", Probability: 0.6}, "eco", false},
		{"weak rival does not contest", systemone.Answer{Option: "find_trade", Probability: 0.95}, systemone.Answer{Option: "search", Probability: 0.3}, "eco", true},
		{"every server declines", systemone.Answer{Option: toolNoToolOption, Probability: 0.97}, systemone.Answer{Option: toolNoToolOption, Probability: 0.97}, "", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			tc.eco.Key, tc.wiki.Key = toolPickKeyPrefix+"eco", toolPickKeyPrefix+"wiki"
			decision := RouteDecision{Ran: true}
			applyToolAnswers(&decision, meta, map[string]systemone.Answer{tc.eco.Key: tc.eco, tc.wiki.Key: tc.wiki}, nil)
			if decision.ToolServer != tc.wantServer {
				t.Errorf("server = %q, want %q", decision.ToolServer, tc.wantServer)
			}
			if _, _, direct := decision.DirectTool(); direct != tc.wantDirect {
				t.Errorf("direct = %v, want %v (tool %.2f, rival %.2f)", direct, tc.wantDirect, decision.ToolProb, decision.ToolRivalProb)
			}
		})
	}
}

func TestGeneralServersNeverContestADomainPick(t *testing.T) {
	_, meta := toolRouteQuestions([]CachedServerTools{ecoListing(), wikiListing()}, nil)
	general := []string{"wiki"}
	cases := []struct {
		name       string
		eco, wiki  systemone.Answer
		wantServer string
		wantDirect bool
	}{
		{"general rival ignored", systemone.Answer{Option: "find_trade", Probability: 0.95}, systemone.Answer{Option: "search", Probability: 0.8}, "eco", true},
		{"general wins when no domain pick", systemone.Answer{Option: toolNoToolOption, Probability: 0.96}, systemone.Answer{Option: "search", Probability: 0.92}, "wiki", true},
		{"weak domain pick yields to general", systemone.Answer{Option: "find_trade", Probability: 0.3}, systemone.Answer{Option: "search", Probability: 0.93}, "wiki", true},
		{"domain under the bar still declines", systemone.Answer{Option: "find_trade", Probability: 0.7}, systemone.Answer{Option: "search", Probability: 0.95}, "eco", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			tc.eco.Key, tc.wiki.Key = toolPickKeyPrefix+"eco", toolPickKeyPrefix+"wiki"
			decision := RouteDecision{Ran: true}
			applyToolAnswers(&decision, meta, map[string]systemone.Answer{tc.eco.Key: tc.eco, tc.wiki.Key: tc.wiki}, general)
			if decision.ToolServer != tc.wantServer {
				t.Errorf("server = %q, want %q", decision.ToolServer, tc.wantServer)
			}
			if _, _, direct := decision.DirectTool(); direct != tc.wantDirect {
				t.Errorf("direct = %v, want %v (tool %.2f, rival %.2f)", direct, tc.wantDirect, decision.ToolProb, decision.ToolRivalProb)
			}
		})
	}
}

func TestApplyToolAnswersNamesTheRivalThatContestsTheWin(t *testing.T) {
	_, meta := toolRouteQuestions([]CachedServerTools{ecoListing(), wikiListing()}, nil)
	decision := RouteDecision{Ran: true}
	applyToolAnswers(&decision, meta, map[string]systemone.Answer{
		toolPickKeyPrefix + "eco":  {Key: toolPickKeyPrefix + "eco", Option: "find_trade", Probability: 1},
		toolPickKeyPrefix + "wiki": {Key: toolPickKeyPrefix + "wiki", Option: "search", Probability: 0.9},
	}, nil)

	if decision.ToolServer != "eco" || decision.ToolRival != "wiki" || decision.ToolRivalProb != 0.9 {
		t.Fatalf("winner %q rival %q at %.2f, want eco contested by wiki at 0.90", decision.ToolServer, decision.ToolRival, decision.ToolRivalProb)
	}
	if _, _, direct := decision.DirectTool(); direct {
		t.Error("direct with a rival at 0.9, want the contest to decline it")
	}
}

func TestApplyToolAnswersFallsBackWhenNothingWasAskedOrAnswered(t *testing.T) {
	_, meta := toolRouteQuestions([]CachedServerTools{ecoListing()}, nil)

	missing := RouteDecision{Ran: true}
	applyToolAnswers(&missing, meta, nil, nil)
	if missing.Fallbacks[RouteFamilyTool] != jevFallbackMissingAnswer {
		t.Errorf("unanswered fallback = %q, want %q", missing.Fallbacks[RouteFamilyTool], jevFallbackMissingAnswer)
	}

	unlisted := RouteDecision{Ran: true}
	applyToolAnswers(&unlisted, nil, nil, nil)
	if unlisted.Fallbacks[RouteFamilyTool] != jevFallbackNoListed {
		t.Errorf("no listing fallback = %q, want %q", unlisted.Fallbacks[RouteFamilyTool], jevFallbackNoListed)
	}
}

func TestCachedToolsReadsListedServersWithoutDialing(t *testing.T) {
	provider := &MCPProvider{entries: []*supervisedServer{
		{definition: MCPServerDefinition{Name: "unlisted"}},
		{definition: MCPServerDefinition{Name: "eco"}, tools: ecoListing().Tools},
	}}

	listings := provider.CachedTools()

	if len(listings) != 1 || listings[0].Server != "eco" || len(listings[0].Tools) != 2 {
		t.Fatalf("listings = %+v, want only eco with its two tools", listings)
	}
	if listings[0].Guidance != "" {
		t.Errorf("guidance = %q with no session, want empty", listings[0].Guidance)
	}
}

func TestWarmToolsListsTheRosterBeforeAnyTurn(t *testing.T) {
	server := mcp.NewServer(&mcp.Implementation{Name: "eco-test", Version: "1"}, nil)
	server.AddTool(
		&mcp.Tool{Name: "get_server_status", Description: "status", InputSchema: map[string]any{"type": "object"}},
		func(context.Context, *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			return &mcp.CallToolResult{}, nil
		},
	)
	httpServer := httptest.NewServer(mcp.NewStreamableHTTPHandler(
		func(*http.Request) *mcp.Server { return server },
		&mcp.StreamableHTTPOptions{JSONResponse: true},
	))
	t.Cleanup(httpServer.Close)
	agent := testJevAgent(t)
	agent.tools = &MCPProvider{Servers: []MCPServerDefinition{{Name: "eco", URL: httpServer.URL}}}
	t.Cleanup(func() { _ = agent.tools.Close() })

	if listings := agent.tools.CachedTools(); len(listings) != 0 {
		t.Fatalf("listings before warm-up = %d, want none", len(listings))
	}
	agent.warmTools(context.Background())

	listings := agent.tools.CachedTools()
	if len(listings) != 1 || listings[0].Server != "eco" || len(listings[0].Tools) != 1 {
		t.Fatalf("listings after warm-up = %+v, want eco with its one tool", listings)
	}
}

func TestRouteJevTracesADirectToolPick(t *testing.T) {
	agent := testJevAgent(t)
	agent.cfg.JevModel = "jev-latest"
	agent.tools = &MCPProvider{entries: []*supervisedServer{
		{definition: MCPServerDefinition{Name: "eco"}, tools: ecoListing().Tools},
	}}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req systemone.Request
		decodeJSON(t, r, &req)
		resp := systemone.Response{}
		for _, q := range req.Questions {
			if q.Key == toolPickKeyPrefix+"eco" {
				resp.Answers = append(resp.Answers, systemone.Answer{Key: q.Key, Option: "find_trade", Probability: 0.94})
				continue
			}
			resp.Answers = append(resp.Answers, systemone.Answer{Key: q.Key, Probability: 0.1})
		}
		encodeJSON(t, w, resp)
	}))
	defer server.Close()
	agent.cfg.AgentProxyURL = server.URL

	decision := agent.routeJev(context.Background(), TranscriptEntry{Content: "where can I buy limestone"}, "req-tool", false, false)

	serverName, tool, direct := decision.DirectTool()
	if !direct || serverName != "eco" || tool != "find_trade" {
		t.Fatalf("DirectTool = %s/%s/%v, want eco/find_trade/true", serverName, tool, direct)
	}
}

func TestJevDisableToolAsksNoToolQuestions(t *testing.T) {
	agent := testJevAgent(t)
	agent.tools = &MCPProvider{entries: []*supervisedServer{
		{definition: MCPServerDefinition{Name: "eco"}, tools: ecoListing().Tools},
	}}

	questions, _ := agent.buildRouteQuestions(TranscriptEntry{Content: "hi"}, jevDisabledSet([]string{"tool"}), false, false)

	for _, q := range questions {
		if strings.HasPrefix(q.Key, toolPickKeyPrefix) {
			t.Fatalf("asked %s with the tool family disabled", q.Key)
		}
	}
}

func criterionNames(criteria []systemone.Criterion) []string {
	names := make([]string, 0, len(criteria))
	for _, c := range criteria {
		names = append(names, c.Name)
	}
	return names
}

func questionKeys(questions []systemone.Question) []string {
	keys := make([]string, 0, len(questions))
	for _, q := range questions {
		keys = append(keys, q.Key)
	}
	return keys
}
