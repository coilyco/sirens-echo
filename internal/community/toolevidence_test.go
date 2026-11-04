package community

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/coilyco/sirens-echo/internal/community/systemone"
)

// askedJev runs routeJev against a fake Jev and returns the request it received.
func askedJev(t *testing.T, agent *Agent, message string) systemone.Request {
	t.Helper()
	var got systemone.Request
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		decodeJSON(t, r, &got)
		encodeJSON(t, w, systemone.Response{})
	}))
	t.Cleanup(server.Close)
	agent.cfg.JevModel = "jev-latest"
	agent.cfg.AgentProxyURL = server.URL
	agent.routeJev(context.Background(), TranscriptEntry{Content: message}, "req-evidence", false, false)
	return got
}

func ecoPickPrompt(t *testing.T, req systemone.Request) string {
	t.Helper()
	for _, q := range req.Questions {
		if q.Key == toolPickKeyPrefix+"eco" {
			return q.Prompt
		}
	}
	t.Fatalf("no eco tool pick among %v", questionKeys(req.Questions))
	return ""
}

func TestAnEvidenceArgumentIsReadFromTheToolsPublishedMeta(t *testing.T) {
	var calls []map[string]any
	provider := storeLookupServer(t, &calls)

	specs := toolArgSpecs(provider.CachedTools()[0].Tools[0])

	if !specs["store"].Evidence || specs["item"].Evidence {
		t.Fatalf("evidence = store %v, item %v, want only the store opted in", specs["store"].Evidence, specs["item"].Evidence)
	}
}

func TestAStoreNameInTheMessageReachesJevAsEvidenceForThatServersPick(t *testing.T) {
	var calls []map[string]any
	agent := testJevAgent(t)
	agent.tools = storeLookupServer(t, &calls)

	req := askedJev(t, agent, "stock check food coming soon")

	names, _ := req.State[namesInMessageKey].(map[string]any)
	eco, _ := names["eco"].(map[string]any)
	stores, _ := eco["store"].([]any)
	if len(stores) != 1 || stores[0] != "Food Coming Soon" {
		t.Fatalf("state = %v, want names_in_message eco.store [Food Coming Soon]", req.State)
	}
	if !strings.Contains(ecoPickPrompt(t, req), "names_in_message") {
		t.Errorf("the eco pick prompt does not mention names_in_message: %q", ecoPickPrompt(t, req))
	}
	if req.State["message"] != "stock check food coming soon" {
		t.Errorf("message = %v, want the member's words unchanged", req.State["message"])
	}
}

func TestOnlyAnOptedInArgumentSendsEvidence(t *testing.T) {
	var calls []map[string]any
	agent := testJevAgent(t)
	agent.tools = storeLookupServer(t, &calls)

	req := askedJev(t, agent, "how much meaty stew at food coming soon")

	eco, _ := req.State[namesInMessageKey].(map[string]any)["eco"].(map[string]any)
	if _, leaked := eco["item"]; leaked || len(eco) != 1 {
		t.Fatalf("names_in_message = %v, want the store only, never the item", eco)
	}
}

func TestAMessageNamingNoStoreLeavesJevsInputExactlyAsBefore(t *testing.T) {
	var calls []map[string]any
	agent := testJevAgent(t)
	agent.tools = storeLookupServer(t, &calls)

	req := askedJev(t, agent, "where can I buy limestone")

	if _, present := req.State[namesInMessageKey]; present {
		t.Errorf("state = %v, want no names_in_message when nothing matched", req.State)
	}
	if strings.Contains(ecoPickPrompt(t, req), "names_in_message") {
		t.Errorf("prompt = %q, want no mention of evidence nobody sent", ecoPickPrompt(t, req))
	}
}

func TestAnUnreadableVocabularyAddsNothingAndTheTurnStillRoutes(t *testing.T) {
	tool := &mcp.Tool{Name: "find_trade", Description: "trade", Meta: mcp.Meta{
		toolArgsMetaKey: map[string]any{
			"store": map[string]any{"vocabulary": "eco://vocab/stores", "field": "name", "evidence": true},
		},
	}}
	agent := testJevAgent(t)
	agent.tools = &MCPProvider{entries: []*supervisedServer{
		{definition: MCPServerDefinition{Name: "eco"}, tools: []*mcp.Tool{tool}},
	}}

	req := askedJev(t, agent, "stock check food coming soon")

	if _, present := req.State[namesInMessageKey]; present || len(req.Questions) == 0 {
		t.Fatalf("state = %v, questions %v, want a pick without evidence", req.State, questionKeys(req.Questions))
	}
}
