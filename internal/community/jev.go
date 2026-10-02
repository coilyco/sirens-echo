package community

import (
	"context"
	"fmt"
	"log/slog"
	"path/filepath"
	"sort"
	"strings"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"

	"github.com/coilyco/sirens-echo/internal/community/systemone"
)

// route.jev runs between context.assemble and the content gate (sirens-echo#8050).
// PR 1 computes and traces the decision; consuming it is a follow-up.

// RouteFamily is the closed set of questions route.jev asks. Every family
// name doubles as the SIRENS_ECHO_JEV_DISABLE token for it.
type RouteFamily string

const (
	RouteFamilyContent   RouteFamily = "content"
	RouteFamilyDrawer    RouteFamily = "drawer"
	RouteFamilyRoot      RouteFamily = "root"
	RouteFamilyFocus     RouteFamily = "focus"
	RouteFamilyServer    RouteFamily = "server"
	RouteFamilyRoute     RouteFamily = "route"
	RouteFamilyDepth     RouteFamily = "depth"
	RouteFamilyShape     RouteFamily = "shape"
	RouteFamilyCoalesce  RouteFamily = "coalesce"
	RouteFamilyAddressed RouteFamily = "addressed"
	RouteFamilyRequest   RouteFamily = "request"
	RouteFamilyTool      RouteFamily = "tool"
)

// routeFamilies is the spec's own table order, so a trace reads the same way.
var routeFamilies = []RouteFamily{
	RouteFamilyContent, RouteFamilyDrawer, RouteFamilyRoot, RouteFamilyFocus,
	RouteFamilyServer, RouteFamilyRoute, RouteFamilyDepth, RouteFamilyShape,
	RouteFamilyCoalesce, RouteFamilyAddressed, RouteFamilyRequest, RouteFamilyTool,
}

// jevContentThreshold, jevDrawerThreshold, jevShapeCutoff are the spec's
// proposed cutoffs (algorithm constants, not knobs - see elsewhereByDesign).
const (
	jevContentThreshold = 0.30
	jevDrawerThreshold  = 0.50
	jevShapeCutoff      = 0.70
)

// jevFallbackReason is the closed vocabulary a fallback carries onto the
// span. Never model or member text.
type jevFallbackReason string

const (
	jevFallbackNone           jevFallbackReason = ""
	jevFallbackDisabled       jevFallbackReason = "disabled"
	jevFallbackCallFailed     jevFallbackReason = "call_failed"
	jevFallbackMissingAnswer  jevFallbackReason = "missing_answer"
	jevFallbackNoDrawerPassed jevFallbackReason = "no_drawer_at_threshold"
)

// RouteDrawerPick is one drawer route.jev approved for inline shipping.
type RouteDrawerPick struct {
	Path        string
	Probability float64
}

// RouteDecision is what route.jev decided for one turn. A family present in
// Fallbacks fell back for the named reason; its field holds today's value.
type RouteDecision struct {
	// Ran is false when the stage was skipped entirely: no network call.
	Ran bool
	// QuestionCount is what was built and sent, post kill-switch, pre-split.
	QuestionCount int
	// Split reports whether SIRENS_ECHO_JEV_SPLIT_AT forced two requests.
	Split bool

	ContentClasses []string // ids at/above jevContentThreshold
	Drawers        []RouteDrawerPick
	GatedRoots     []string // roots to ship, from the noul-per-root family
	Focus          string   // a game-focus root, "neither", or "" on fallback
	Servers        []string // servers to keep, from the noul-per-server family
	DroppedServers []string // servers Jev answered below 0.5, the only ones pruned
	Route          string   // "default" or "deepseek"
	Depth          int      // 1-5, 0 on fallback
	Shape          string   // an option id, or "" (full) on fallback
	ShapeProb      float64
	Coalesce       bool
	Addressed      bool
	Requested      string // the reply key the member asked Echo to react with, or ""
	RequestedProb  float64
	ToolServer     string  // the server whose tool pick won, "" when every server declined
	ToolRival      string  // the strongest rival server, whose pick contests the win
	ToolRivalProb  float64 // that rival's pick
	Tool           string  // that server's tool, "" on no_tool or fallback
	ToolProb       float64

	// Fallbacks names, per family, why its field is the fallback value.
	Fallbacks map[RouteFamily]jevFallbackReason
}

func (d *RouteDecision) fellBackTo(family RouteFamily, reason jevFallbackReason) {
	if d.Fallbacks == nil {
		d.Fallbacks = make(map[RouteFamily]jevFallbackReason)
	}
	d.Fallbacks[family] = reason
}

func (d *RouteDecision) hasFallback(family RouteFamily) bool {
	_, ok := d.Fallbacks[family]
	return ok
}

// jevQuestionMeta maps one answer key back to its family and, for
// drawer/root/server, the specific path or name it named.
type jevQuestionMeta struct {
	family RouteFamily
	target string
}

// jevDisabledSet parses SIRENS_ECHO_JEV_DISABLE into a lookup set.
func jevDisabledSet(raw []string) map[RouteFamily]bool {
	disabled := make(map[RouteFamily]bool, len(raw))
	for _, name := range raw {
		disabled[RouteFamily(strings.TrimSpace(strings.ToLower(name)))] = true
	}
	return disabled
}

// jevGameFocusPrefix mirrors gameFocusPrefix in gamefocus_test.go, duplicated
// because the test constant is not part of the compiled package.
const jevGameFocusPrefix = "sirens-game-"

// jevGameName mirrors gameNameOf in gamefocus_test.go, same reason.
func jevGameName(root string) string {
	words := strings.Split(strings.TrimPrefix(root, jevGameFocusPrefix), "-")
	for i, word := range words {
		if word == "" {
			continue
		}
		words[i] = strings.ToUpper(word[:1]) + word[1:]
	}
	return strings.Join(words, " ")
}

// gatedSkillRoots names roots whose body ships only when the root family
// picks them, read from LocalSkillRoots rather than a literal list.
func gatedSkillRoots(localSkillRoots []string) []string {
	gated := make([]string, 0, 2)
	for _, root := range localSkillRoots {
		base := filepath.Base(strings.TrimRight(root, "/"))
		if strings.HasPrefix(base, jevGameFocusPrefix) || strings.Contains(base, "-science") {
			gated = append(gated, base)
		}
	}
	sort.Strings(gated)
	return gated
}

// gameFociOnDisk maps every game-focus root configured for this definition
// to the game name its slug declares.
func gameFociOnDisk(localSkillRoots []string) map[string]string {
	foci := make(map[string]string)
	for _, root := range localSkillRoots {
		base := filepath.Base(strings.TrimRight(root, "/"))
		if !strings.HasPrefix(base, jevGameFocusPrefix) {
			continue
		}
		foci[base] = jevGameName(base)
	}
	return foci
}

// shapeOptions reads the live reaction and phrase registries rather than a
// hardcoded list, since both are still changing (sirens-echo#8050).
func (a *Agent) shapeOptions() []systemone.Criterion {
	options := []systemone.Criterion{
		{Name: "full", Description: "An ordinary answer: the main model runs as it does today."},
	}
	for _, key := range reactKeys() {
		if !snapReactions[key] {
			continue
		}
		options = append(options, systemone.Criterion{
			Name: "react:" + key,
			Description: "The message is purely social and needs no lookup, so the whole answer " +
				"is the " + key + " mark (" + reactionMeanings[key] + "), no words, no model call.",
		})
	}
	if a.phrases.Configured() {
		for _, key := range a.phrases.Keys() {
			options = append(options, systemone.Criterion{
				Name:        "phrase:" + key,
				Description: "Deliver only the canonical " + key + " phrase, no model call.",
			})
		}
	}
	return options
}

// buildRouteQuestions assembles every enabled family's questions for one
// turn. coalesce and addressed ask only when their trigger signal is true.
func (a *Agent) buildRouteQuestions(
	current TranscriptEntry,
	disabled map[RouteFamily]bool,
	hasEarlierInWindow bool,
	threadNoMention bool,
) ([]systemone.Question, map[string]jevQuestionMeta) {
	questions := make([]systemone.Question, 0, 96)
	meta := make(map[string]jevQuestionMeta)

	add := func(q systemone.Question, m jevQuestionMeta) {
		questions = append(questions, q)
		meta[q.Key] = m
	}

	if !disabled[RouteFamilyContent] {
		for _, class := range a.taxonomy.Classes {
			key := "content:" + class.ID
			add(systemone.Question{
				Key:    key,
				Type:   systemone.TypeNoul,
				Prompt: "Does the member's message belong to the content class \"" + class.ID + "\" (" + class.Summary + ")?",
			}, jevQuestionMeta{family: RouteFamilyContent, target: class.ID})
		}
	}

	if !disabled[RouteFamilyDrawer] {
		if references, err := LoadSkillReferences(a.cfg.Definition.LocalSkillRoots); err == nil {
			for _, reference := range references {
				key := "drawer:" + reference.Path
				add(systemone.Question{
					Key:    key,
					Type:   systemone.TypeNoul,
					Prompt: "Does answering this turn need the reference \"" + reference.Path + "\" (" + reference.Title + ")?",
				}, jevQuestionMeta{family: RouteFamilyDrawer, target: reference.Path})
			}
		}
	}

	if !disabled[RouteFamilyRoot] {
		for _, root := range gatedSkillRoots(a.cfg.Definition.LocalSkillRoots) {
			key := "root:" + root
			add(systemone.Question{
				Key:    key,
				Type:   systemone.TypeNoul,
				Prompt: "Does this turn need the gated skill root \"" + root + "\"?",
			}, jevQuestionMeta{family: RouteFamilyRoot, target: root})
		}
	}

	if !disabled[RouteFamilyFocus] {
		foci := gameFociOnDisk(a.cfg.Definition.LocalSkillRoots)
		criteria := []systemone.Criterion{{Name: "neither", Description: "The turn is not about either game."}}
		names := make([]string, 0, len(foci))
		for root, game := range foci {
			names = append(names, root)
			criteria = append(criteria, systemone.Criterion{Name: root, Description: "The turn is about " + game + "."})
		}
		sort.Strings(names)
		if len(criteria) > 1 {
			add(systemone.Question{
				Key:      "focus",
				Type:     systemone.TypeChoice,
				Prompt:   "Which game, if any, is this turn about?",
				Criteria: criteria,
			}, jevQuestionMeta{family: RouteFamilyFocus})
		}
	}

	if !disabled[RouteFamilyServer] && a.tools != nil {
		for _, server := range a.tools.Servers {
			key := "server:" + server.Name
			add(systemone.Question{
				Key:    key,
				Type:   systemone.TypeNoul,
				Prompt: "Might answering this turn need a tool from the MCP server \"" + server.Name + "\"?",
			}, jevQuestionMeta{family: RouteFamilyServer, target: server.Name})
		}
	}

	if !disabled[RouteFamilyRoute] {
		add(systemone.Question{
			Key:  "route",
			Type: systemone.TypeChoice,
			Prompt: "Should this turn's answer come from the deployment's default route, or " +
				"from the deeper-reasoning route?",
			Criteria: []systemone.Criterion{
				{Name: "default", Description: "A quick, factual turn."},
				{Name: "deepseek", Description: "A turn that needs longer reasoning."},
			},
		}, jevQuestionMeta{family: RouteFamilyRoute})
	}

	if !disabled[RouteFamilyDepth] {
		add(systemone.Question{
			Key:    "depth",
			Type:   systemone.TypeScore,
			Prompt: "How many tool rounds and how large an answer does this turn need, from 1 (minimal) to 5 (extensive)?",
			Levels: 5,
		}, jevQuestionMeta{family: RouteFamilyDepth})
	}

	if !disabled[RouteFamilyShape] {
		add(systemone.Question{
			Key:      "shape",
			Type:     systemone.TypeChoice,
			Prompt:   "What shape should the reply take?",
			Criteria: a.shapeOptions(),
		}, jevQuestionMeta{family: RouteFamilyShape})
	}

	if !disabled[RouteFamilyCoalesce] && hasEarlierInWindow {
		add(systemone.Question{
			Key:    "coalesce",
			Type:   systemone.TypeNoul,
			Prompt: "Is this message part of the same request as the member's earlier message in this window?",
		}, jevQuestionMeta{family: RouteFamilyCoalesce})
	}

	if !disabled[RouteFamilyAddressed] && threadNoMention {
		add(systemone.Question{
			Key:    "addressed",
			Type:   systemone.TypeNoul,
			Prompt: "Is this thread message addressed to this service, though it carries no mention?",
		}, jevQuestionMeta{family: RouteFamilyAddressed})
	}

	if !disabled[RouteFamilyRequest] {
		criteria := []systemone.Criterion{
			{Name: "none", Description: "The message does not ask this service to react with an emoji."},
		}
		for _, key := range reactKeys() {
			criteria = append(criteria, systemone.Criterion{
				Name: "react:" + key,
				Description: "The member asks this service to react with " + replyReactions[key] +
					" or a skin-tone or look-alike variant of it (" + reactionMeanings[key] + ").",
			})
		}
		add(systemone.Question{
			Key:      "request",
			Type:     systemone.TypeChoice,
			Prompt:   "Does the member explicitly ask this service to react with a particular emoji, and which?",
			Criteria: criteria,
		}, jevQuestionMeta{family: RouteFamilyRequest})
	}

	if !disabled[RouteFamilyTool] && a.tools != nil {
		toolQuestions, toolMeta := toolRouteQuestions(a.tools.CachedTools(), a.cfg.JevSkipTools)
		for _, q := range toolQuestions {
			add(q, toolMeta[q.Key])
		}
	}

	return questions, meta
}

// jevState renders the turn facts route.jev's questions refer to. Never
// member-identifying: the spec requires state to carry message text only.
func jevState(current TranscriptEntry) map[string]any {
	return map[string]any{
		"message": current.Content,
	}
}

// splitQuestions halves a question slice for the two-parallel-requests
// contingency the spec names if a single request balks.
func splitQuestions(questions []systemone.Question) ([]systemone.Question, []systemone.Question) {
	mid := (len(questions) + 1) / 2
	return questions[:mid], questions[mid:]
}

// routeJev runs the pre-answer Jev stage. It never returns an error: every
// failure resolves to the documented per-family fallback instead.
func (a *Agent) routeJev(
	ctx context.Context,
	current TranscriptEntry,
	requestID string,
	hasEarlierInWindow bool,
	threadNoMention bool,
) RouteDecision {
	decision := RouteDecision{Fallbacks: make(map[RouteFamily]jevFallbackReason)}

	jevCtx, span := a.telemetry.StartSpan(ctx, "route.jev")
	defer span.End()

	// Undeployed or intentionally off: every family falls back, no call runs.
	if strings.TrimSpace(a.cfg.JevModel) == "" {
		for _, family := range routeFamilies {
			decision.fellBackTo(family, jevFallbackDisabled)
		}
		span.SetAttributes(attribute.Bool("jev.ran", false))
		return decision
	}

	disabled := jevDisabledSet(a.cfg.JevDisable)
	for family := range disabled {
		decision.fellBackTo(family, jevFallbackDisabled)
	}

	questions, meta := a.buildRouteQuestions(current, disabled, hasEarlierInWindow, threadNoMention)
	decision.QuestionCount = len(questions)
	span.SetAttributes(attribute.Int("jev.questions", len(questions)))
	if len(questions) == 0 {
		decision.Ran = true
		return decision
	}

	client := systemone.Client{
		BaseURL: a.cfg.AgentProxyURL,
		Model:   a.cfg.JevModel,
		Timeout: jevTimeout,
	}
	state := jevState(current)

	var answers map[string]systemone.Answer
	if jevSplitAt > 0 && len(questions) > jevSplitAt {
		decision.Split = true
		first, second := splitQuestions(questions)
		merged, err := a.askJevSplit(jevCtx, client, requestID, state, first, second)
		if err != nil {
			a.fallBackEveryFamily(&decision, span, jevCtx, err)
			return decision
		}
		answers = merged
	} else {
		resp, err := client.Ask(jevCtx, systemone.Request{State: state, Questions: questions})
		if err != nil {
			a.fallBackEveryFamily(&decision, span, jevCtx, err)
			return decision
		}
		answers = resp.AnswerByKey()
	}

	decision.Ran = true
	a.applyRouteAnswers(&decision, meta, answers, disabled)
	recordRouteDecision(span, decision)
	return decision
}

// fallBackEveryFamily records a whole-call failure: every family not already
// disabled falls back, and the reason lands on both the span and the log.
func (a *Agent) fallBackEveryFamily(decision *RouteDecision, span trace.Span, ctx context.Context, err error) {
	for _, family := range routeFamilies {
		if !decision.hasFallback(family) {
			decision.fellBackTo(family, jevFallbackCallFailed)
		}
	}
	span.SetAttributes(attribute.Bool("jev.ran", false), attribute.String("jev.error", err.Error()))
	a.telemetry.Info(ctx, "route.jev.failed", slog.String("error", err.Error()))
}

// askJevSplit sends both halves in parallel. Either half failing fails the
// whole call, since a partial routing decision is not one the spec defines.
func (a *Agent) askJevSplit(
	ctx context.Context,
	client systemone.Client,
	requestID string,
	state map[string]any,
	first, second []systemone.Question,
) (map[string]systemone.Answer, error) {
	type result struct {
		resp systemone.Response
		err  error
	}
	results := make(chan result, 2)
	for _, half := range [][]systemone.Question{first, second} {
		half := half
		go func() {
			resp, err := client.Ask(ctx, systemone.Request{State: state, Questions: half})
			results <- result{resp: resp, err: err}
		}()
	}
	merged := make(map[string]systemone.Answer)
	for range 2 {
		r := <-results
		if r.err != nil {
			return nil, fmt.Errorf("split request %s: %w", requestID, r.err)
		}
		for key, answer := range r.resp.AnswerByKey() {
			merged[key] = answer
		}
	}
	return merged, nil
}

// applyRouteAnswers interprets Jev's reply, filling the fallback for any
// family whose answer is missing.
func (a *Agent) applyRouteAnswers(
	decision *RouteDecision,
	meta map[string]jevQuestionMeta,
	answers map[string]systemone.Answer,
	disabled map[RouteFamily]bool,
) {
	if !disabled[RouteFamilyContent] {
		ids := make([]string, 0)
		missing := true
		for _, class := range a.taxonomy.Classes {
			key := "content:" + class.ID
			if _, asked := meta[key]; !asked {
				continue
			}
			missing = false
			if answer, ok := answers[key]; ok && answer.Probability >= jevContentThreshold {
				ids = append(ids, class.ID)
			}
		}
		if missing {
			decision.fellBackTo(RouteFamilyContent, jevFallbackMissingAnswer)
		} else {
			decision.ContentClasses = ids
		}
	}

	if !disabled[RouteFamilyDrawer] {
		picks := make([]RouteDrawerPick, 0)
		asked := false
		for key, m := range meta {
			if m.family != RouteFamilyDrawer {
				continue
			}
			asked = true
			if answer, ok := answers[key]; ok && answer.Probability >= jevDrawerThreshold {
				picks = append(picks, RouteDrawerPick{Path: m.target, Probability: answer.Probability})
			}
		}
		sort.Slice(picks, func(i, j int) bool { return picks[i].Path < picks[j].Path })
		if asked && len(picks) == 0 {
			decision.fellBackTo(RouteFamilyDrawer, jevFallbackNoDrawerPassed)
		}
		decision.Drawers = picks
	}

	if !disabled[RouteFamilyRoot] {
		roots := make([]string, 0)
		for key, m := range meta {
			if m.family != RouteFamilyRoot {
				continue
			}
			if answer, ok := answers[key]; ok && answer.Probability >= 0.5 {
				roots = append(roots, m.target)
			}
		}
		sort.Strings(roots)
		decision.GatedRoots = roots
	}

	if !disabled[RouteFamilyFocus] {
		if answer, ok := answers["focus"]; ok {
			decision.Focus = answer.Option
		} else if _, asked := metaHasFamily(meta, RouteFamilyFocus); asked {
			decision.fellBackTo(RouteFamilyFocus, jevFallbackMissingAnswer)
		}
	}

	if !disabled[RouteFamilyServer] {
		servers := make([]string, 0)
		dropped := make([]string, 0)
		asked, answered := false, false
		for key, m := range meta {
			if m.family != RouteFamilyServer {
				continue
			}
			asked = true
			answer, ok := answers[key]
			if !ok {
				continue
			}
			answered = true
			if answer.Probability >= 0.5 {
				servers = append(servers, m.target)
			} else {
				dropped = append(dropped, m.target)
			}
		}
		sort.Strings(servers)
		sort.Strings(dropped)
		decision.Servers = servers
		decision.DroppedServers = dropped
		if asked && !answered {
			decision.fellBackTo(RouteFamilyServer, jevFallbackMissingAnswer)
		}
	}

	if !disabled[RouteFamilyRoute] {
		if answer, ok := answers["route"]; ok {
			decision.Route = answer.Option
		} else {
			decision.fellBackTo(RouteFamilyRoute, jevFallbackMissingAnswer)
		}
	}

	if !disabled[RouteFamilyDepth] {
		if answer, ok := answers["depth"]; ok {
			decision.Depth = answer.Level
		} else {
			decision.fellBackTo(RouteFamilyDepth, jevFallbackMissingAnswer)
		}
	}

	if !disabled[RouteFamilyShape] {
		if answer, ok := answers["shape"]; ok {
			decision.Shape = answer.Option
			decision.ShapeProb = answer.Probability
		} else {
			decision.fellBackTo(RouteFamilyShape, jevFallbackMissingAnswer)
		}
	}

	if !disabled[RouteFamilyCoalesce] {
		if answer, ok := answers["coalesce"]; ok {
			decision.Coalesce = answer.Probability >= 0.5
		}
	}

	if !disabled[RouteFamilyAddressed] {
		if answer, ok := answers["addressed"]; ok {
			decision.Addressed = answer.Probability >= 0.5
		}
	}

	if !disabled[RouteFamilyRequest] {
		if answer, ok := answers["request"]; ok {
			decision.Requested = strings.TrimPrefix(answer.Option, "react:")
			if answer.Option == "none" {
				decision.Requested = ""
			}
			decision.RequestedProb = answer.Probability
		} else {
			decision.fellBackTo(RouteFamilyRequest, jevFallbackMissingAnswer)
		}
	}

	if !disabled[RouteFamilyTool] {
		applyToolAnswers(decision, meta, answers, a.cfg.JevGeneralServers)
	}
}

// PrunedServers is what Complete may drop: an explicit no only, never a
// fallback. See docs/sirens-echo-tools.md.
func (d RouteDecision) PrunedServers() []string {
	if !d.Ran || d.hasFallback(RouteFamilyServer) {
		return nil
	}
	return d.DroppedServers
}

// SnapReaction is the mark to place before the model: one the member asked
// for by name, else a social one from shape. A fallback runs the model.
func (d RouteDecision) SnapReaction() (string, bool) {
	if !d.Ran {
		return "", false
	}
	// Any key may be requested, agree included: the member named the answer.
	if _, known := replyReactions[d.Requested]; known &&
		!d.hasFallback(RouteFamilyRequest) && d.RequestedProb >= jevShapeCutoff {
		return d.Requested, true
	}
	if d.hasFallback(RouteFamilyShape) || d.ShapeProb < jevShapeCutoff {
		return "", false
	}
	key, isReact := strings.CutPrefix(d.Shape, "react:")
	if !isReact || !snapReactions[key] {
		return "", false
	}
	return key, true
}

func metaHasFamily(meta map[string]jevQuestionMeta, family RouteFamily) (jevQuestionMeta, bool) {
	for _, m := range meta {
		if m.family == family {
			return m, true
		}
	}
	return jevQuestionMeta{}, false
}

// recordRouteDecision puts every family's pick and fallback reason on the
// span. Ids, keys and reasons only - never member text.
func recordRouteDecision(span trace.Span, decision RouteDecision) {
	span.SetAttributes(
		attribute.Bool("jev.ran", decision.Ran),
		attribute.Int("jev.questions", decision.QuestionCount),
		attribute.Bool("jev.split", decision.Split),
		attribute.StringSlice("jev.content.classes", decision.ContentClasses),
		attribute.Int("jev.drawer.count", len(decision.Drawers)),
		attribute.StringSlice("jev.root.gated", decision.GatedRoots),
		attribute.String("jev.focus", decision.Focus),
		attribute.StringSlice("jev.server.kept", decision.Servers),
		attribute.StringSlice("jev.server.dropped", decision.DroppedServers),
		attribute.String("jev.route", decision.Route),
		attribute.Int("jev.depth", decision.Depth),
		attribute.String("jev.shape", decision.Shape),
		attribute.Float64("jev.shape.probability", decision.ShapeProb),
		attribute.Bool("jev.coalesce", decision.Coalesce),
		attribute.Bool("jev.addressed", decision.Addressed),
		attribute.String("jev.request", decision.Requested),
		attribute.Float64("jev.request.probability", decision.RequestedProb),
		attribute.String("jev.tool.server", decision.ToolServer),
		attribute.String("jev.tool.rival", decision.ToolRival),
		attribute.Float64("jev.tool.rival.probability", decision.ToolRivalProb),
		attribute.String("jev.tool.name", decision.Tool),
		attribute.Float64("jev.tool.probability", decision.ToolProb),
	)
	_, _, direct := decision.DirectTool()
	span.SetAttributes(attribute.Bool("jev.tool.direct", direct))
	for _, family := range routeFamilies {
		if reason, fellBack := decision.Fallbacks[family]; fellBack {
			span.SetAttributes(attribute.String("jev.fallback."+string(family), string(reason)))
		}
	}
}
