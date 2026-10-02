package community

import (
	"context"
	"log/slog"
	"sort"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/coilyco/sirens-echo/internal/community/systemone"
)

// The tool family asks one Jev choice per server. The most confident non-no_tool pick
// names the server, since a separate server pick declined data (sirens-echo#8229).
const (
	// jevToolThreshold is Kai's bar, applied to the winning tool pick.
	jevToolThreshold = 0.9
	// jevToolContested is where a second server's tool pick blocks a direct call.
	jevToolContested = 0.5
	// jevToolMaxOptions is the choice size Jev is reliable at, per the
	// tooling-jev-decisions skill. A larger server is not asked about.
	jevToolMaxOptions = 240

	toolPickKeyPrefix   = "tool.pick:"
	toolNoToolOption    = "no_tool"
	toolNoToolText      = "No tool from this server answers this. Game mechanics and how-to, client crashes and bug reports, wipe or patch schedules, requests addressed to a specific person, and anything outside this server's data."
	jevFallbackNoListed = jevFallbackReason("no_tool_listing")
)

// CachedServerTools is one server's last listing, read without dialing.
type CachedServerTools struct {
	Server   string
	Guidance string
	Tools    []*mcp.Tool
}

// CachedTools snapshots every listed server. A server not yet listed is absent,
// so the first turn after boot asks nothing rather than waiting on a dial.
func (p *MCPProvider) CachedTools() []CachedServerTools {
	p.mu.Lock()
	defer p.mu.Unlock()
	out := make([]CachedServerTools, 0, len(p.entries))
	for _, entry := range p.entries {
		if len(entry.tools) == 0 {
			continue
		}
		listing := CachedServerTools{
			Server: entry.definition.Name,
			Tools:  append([]*mcp.Tool(nil), entry.tools...),
		}
		if guidance, ok := serverGuidance(entry.definition.Name, entry.session); ok {
			listing.Guidance = guidance.Text
		}
		out = append(out, listing)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Server < out[j].Server })
	return out
}

// warmTools lists the roster once at startup. A failure is logged and left to
// the first turn, which retries the listing as it always has.
func (a *Agent) warmTools(ctx context.Context) {
	session, err := a.tools.Open(ctx)
	if err != nil {
		a.telemetry.Info(ctx, "mcp.warm.failed", slog.String("error", err.Error()))
		return
	}
	_ = session.Close()
}

// toolRouteQuestions builds one tool pick per listed server.
func toolRouteQuestions(listings []CachedServerTools, skipTools []string) ([]systemone.Question, map[string]jevQuestionMeta) {
	skip := make(map[string]bool, len(skipTools))
	for _, name := range skipTools {
		skip[strings.TrimSpace(name)] = true
	}
	questions := make([]systemone.Question, 0, len(listings))
	meta := make(map[string]jevQuestionMeta)
	for _, listing := range listings {
		if len(listing.Tools)+1 > jevToolMaxOptions {
			continue
		}
		criteria := make([]systemone.Criterion, 0, len(listing.Tools)+1)
		for _, tool := range listing.Tools {
			if tool == nil || tool.Name == "" || tool.Name == toolNoToolOption ||
				skip[tool.Name] || skip[listing.Server+"__"+tool.Name] {
				continue
			}
			criteria = append(criteria, systemone.Criterion{Name: tool.Name, Description: tool.Description})
		}
		if len(criteria) == 0 {
			continue
		}
		criteria = append(criteria, systemone.Criterion{Name: toolNoToolOption, Description: toolNoToolText})
		prompt := "Which single tool from the MCP server \"" + listing.Server +
			"\" best answers the member's message? Pick " + toolNoToolOption +
			" when no tool's data can answer it."
		if listing.Guidance != "" {
			prompt += " The server describes itself: " + listing.Guidance
		}
		key := toolPickKeyPrefix + listing.Server
		questions = append(questions, systemone.Question{
			Key:      key,
			Type:     systemone.TypeChoice,
			Prompt:   prompt,
			Criteria: criteria,
		})
		meta[key] = jevQuestionMeta{family: RouteFamilyTool, target: listing.Server}
	}
	return questions, meta
}

// toolPick is one server's non-no_tool answer.
type toolPick struct {
	server, tool string
	prob         float64
}

// applyToolAnswers takes the most confident non-no_tool pick. A domain pick at
// jevToolContested outranks every general server, which then cannot contest it.
func applyToolAnswers(
	decision *RouteDecision,
	meta map[string]jevQuestionMeta,
	answers map[string]systemone.Answer,
	generalServers []string,
) {
	general := make(map[string]bool, len(generalServers))
	for _, name := range generalServers {
		general[strings.TrimSpace(name)] = true
	}
	var domain, all []toolPick
	asked, answered := false, false
	for key, m := range meta {
		if m.family != RouteFamilyTool {
			continue
		}
		asked = true
		answer, ok := answers[key]
		if !ok {
			continue
		}
		answered = true
		if answer.Option == toolNoToolOption || answer.Option == "" {
			continue
		}
		pick := toolPick{server: m.target, tool: answer.Option, prob: answer.Probability}
		all = append(all, pick)
		if !general[m.target] {
			domain = append(domain, pick)
		}
	}
	switch {
	case !asked:
		decision.fellBackTo(RouteFamilyTool, jevFallbackNoListed)
		return
	case !answered:
		decision.fellBackTo(RouteFamilyTool, jevFallbackMissingAnswer)
		return
	}
	field := all
	if top, _ := topTwoPicks(domain); top.prob >= jevToolContested {
		field = domain
	}
	top, rival := topTwoPicks(field)
	decision.ToolServer, decision.Tool, decision.ToolProb = top.server, top.tool, top.prob
	decision.ToolRival, decision.ToolRivalProb = rival.server, rival.prob
}

// topTwoPicks orders by probability, then server name, so a tie is stable.
func topTwoPicks(picks []toolPick) (top, rival toolPick) {
	sorted := append([]toolPick(nil), picks...)
	sort.Slice(sorted, func(i, j int) bool {
		if sorted[i].prob != sorted[j].prob {
			return sorted[i].prob > sorted[j].prob
		}
		return sorted[i].server < sorted[j].server
	})
	if len(sorted) > 0 {
		top = sorted[0]
	}
	if len(sorted) > 1 {
		rival = sorted[1]
	}
	return top, rival
}

// DirectTool is the server and tool to call without the model: the winning pick at
// the bar, no rival server's pick at jevToolContested, never on a fallback.
func (d RouteDecision) DirectTool() (server, tool string, ok bool) {
	if !d.Ran || d.hasFallback(RouteFamilyTool) || d.Tool == "" {
		return "", "", false
	}
	if d.ToolProb < jevToolThreshold || d.ToolRivalProb >= jevToolContested {
		return "", "", false
	}
	return d.ToolServer, d.Tool, true
}
