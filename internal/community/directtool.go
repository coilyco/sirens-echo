package community

import (
	"context"
	"log/slog"
	"regexp"
	"strconv"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"go.opentelemetry.io/otel/attribute"
)

// A direct tool reply answers from a server's own reply template with no model
// call (sirens-echo#8229). eco-app owns the template contract.
const (
	replyTemplatesMetaKey = "coilyco/templates"
	// maxTemplateReplyRunes is the contract's cap. Longer is ineligible, not cut.
	maxTemplateReplyRunes = 280
)

// directOutcome is the closed vocabulary tool.direct carries onto its span.
type directOutcome string

const (
	directOff        directOutcome = "off"
	directNoPick     directOutcome = "no_pick"
	directNoTemplate directOutcome = "no_template"
	directNeedsArgs  directOutcome = "needs_args"
	directCallFailed directOutcome = "call_failed"
	directIneligible directOutcome = "ineligible"
	directAnswered   directOutcome = "answered"
	directUnmatched  directOutcome = "unmatched"
)

var templatePlaceholder = regexp.MustCompile(`\{\{([A-Za-z0-9_.]+)\}\}`)

// replyTemplate is one entry of a tool's _meta list, first eligible wins. A
// WhenUnmatched entry is literal text for a word no vocabulary matches (#8424).
type replyTemplate struct {
	WhenArgs      []string
	WhenUnmatched []string
	Text          string
}

// toolReplyTemplates reads a tool's templates. A malformed entry is skipped,
// since a server's metadata is untrusted input.
func toolReplyTemplates(tool *mcp.Tool) []replyTemplate {
	if tool == nil {
		return nil
	}
	raw, _ := tool.Meta[replyTemplatesMetaKey].([]any)
	templates := make([]replyTemplate, 0, len(raw))
	for _, entry := range raw {
		fields, ok := entry.(map[string]any)
		if !ok {
			continue
		}
		text, _ := fields["text"].(string)
		if strings.TrimSpace(text) == "" {
			continue
		}
		template := replyTemplate{
			Text:          text,
			WhenArgs:      metaStrings(fields["when_args"]),
			WhenUnmatched: metaStrings(fields["when_unmatched"]),
		}
		// An unmatched entry is literal: nothing was called, so nothing can fill it.
		if len(template.WhenUnmatched) > 0 && templatePlaceholder.MatchString(text) {
			continue
		}
		templates = append(templates, template)
	}
	return templates
}

func metaStrings(raw any) []string {
	values, _ := raw.([]any)
	var out []string
	for _, value := range values {
		if s, ok := value.(string); ok {
			out = append(out, s)
		}
	}
	return out
}

// renderReplyTemplate returns the first eligible template rendered over payload.
func renderReplyTemplate(templates []replyTemplate, payload any, args map[string]any) (string, bool) {
	for _, template := range templates {
		if len(template.WhenUnmatched) > 0 || !argsPresent(template.WhenArgs, args) {
			continue
		}
		missing := false
		rendered := templatePlaceholder.ReplaceAllStringFunc(template.Text, func(match string) string {
			path := strings.TrimSuffix(strings.TrimPrefix(match, "{{"), "}}")
			value, ok := formatTemplateValue(resolveTemplatePath(path, payload, args))
			if !ok {
				missing = true
			}
			return value
		})
		if !missing && len([]rune(rendered)) <= maxTemplateReplyRunes {
			return rendered, true
		}
	}
	return "", false
}

func argsPresent(names []string, args map[string]any) bool {
	for _, name := range names {
		value, ok := args[name]
		if !ok || value == nil {
			return false
		}
		if s, isString := value.(string); isString && strings.TrimSpace(s) == "" {
			return false
		}
	}
	return true
}

func resolveTemplatePath(path string, payload any, args map[string]any) any {
	segments := strings.Split(path, ".")
	var node any = payload
	if segments[0] == "args" {
		node, segments = args, segments[1:]
	}
	for _, segment := range segments {
		switch typed := node.(type) {
		case map[string]any:
			node = typed[segment]
		case []any:
			index, err := strconv.Atoi(segment)
			if err != nil || index < 0 || index >= len(typed) {
				return nil
			}
			node = typed[index]
		default:
			return nil
		}
	}
	return node
}

// formatTemplateValue matches the reference renderer: strings and numbers only,
// numbers at most two decimals with trailing zeros trimmed.
func formatTemplateValue(value any) (string, bool) {
	switch typed := value.(type) {
	case string:
		return typed, strings.TrimSpace(typed) != ""
	case float64:
		text := strings.TrimRight(strings.TrimRight(strconv.FormatFloat(typed, 'f', 2, 64), "0"), ".")
		if text == "-0" {
			text = "0"
		}
		return text, true
	case int:
		return strconv.Itoa(typed), true
	case int64:
		return strconv.FormatInt(typed, 10), true
	default:
		return "", false
	}
}

// directToolReply calls route.jev's picked tool and renders its template, once
// per item named. Any miss returns false and the turn takes the model path.
func (a *Agent) directToolReply(ctx context.Context, route RouteDecision, message string) ([]string, bool) {
	ctx, span := a.telemetry.StartSpan(ctx, "tool.direct")
	defer span.End()
	outcome := directOff
	defer func() { span.SetAttributes(attribute.String("tool.direct.outcome", string(outcome))) }()

	if !a.cfg.JevDirectTools || a.tools == nil {
		return nil, false
	}
	server, toolName, ok := route.DirectTool()
	if !ok {
		outcome = directNoPick
		return nil, false
	}
	span.SetAttributes(attribute.String("tool.direct.server", server), attribute.String("tool.direct.tool", toolName))
	tool := cachedTool(a.tools.CachedTools(), server, toolName)
	templates := toolReplyTemplates(tool)
	if len(templates) == 0 {
		outcome = directNoTemplate
		return nil, false
	}
	session, err := a.tools.Open(ctx)
	if err != nil {
		outcome = directCallFailed
		return nil, false
	}
	defer func() { _ = session.Close() }()
	// The first template whose arguments all resolve decides the call's arguments.
	specs := toolArgSpecs(tool)
	var argSets []map[string]any
	for _, template := range templates {
		if len(template.WhenUnmatched) > 0 {
			continue
		}
		if resolved, ok := a.resolveToolArgs(ctx, server, specs, template.WhenArgs, message); ok {
			argSets = resolved
			break
		}
	}
	if argSets == nil {
		for _, template := range templates {
			if len(template.WhenUnmatched) > 0 && a.argsUnmatched(ctx, server, specs, template.WhenUnmatched, message) &&
				len([]rune(template.Text)) <= maxTemplateReplyRunes {
				outcome = directUnmatched
				a.telemetry.Info(ctx, "tool.direct.unmatched", slog.String("server", server), slog.String("tool", toolName))
				return []string{template.Text}, true
			}
		}
		outcome = directNeedsArgs
		return nil, false
	}
	span.SetAttributes(attribute.Int("tool.direct.args", len(argSets[0])), attribute.Int("tool.direct.items", len(argSets)))
	name, err := proxyToolName(server, toolName)
	if err != nil {
		outcome = directCallFailed
		return nil, false
	}
	// One call and one reply per item. A miss on any of them declines the whole
	// turn, since a member answered for one item of two has the partial #8431 names.
	replies := make([]string, 0, len(argSets))
	for _, args := range argSets {
		result, err := session.Call(ctx, name, args)
		if err != nil || result.IsError {
			outcome = directCallFailed
			return nil, false
		}
		text, ok := renderReplyTemplate(templates, result.Structured, args)
		if !ok {
			outcome = directIneligible
			return nil, false
		}
		replies = append(replies, text)
	}
	outcome = directAnswered
	a.telemetry.Info(ctx, "tool.direct.answered", slog.String("server", server), slog.String("tool", toolName), slog.Int("items", len(replies)))
	return replies, true
}

func cachedTool(listings []CachedServerTools, server, toolName string) *mcp.Tool {
	for _, listing := range listings {
		if listing.Server != server {
			continue
		}
		for _, tool := range listing.Tools {
			if tool != nil && tool.Name == toolName {
				return tool
			}
		}
	}
	return nil
}

// finishWithDirect delivers template replies, one message each. A thread's
// living answer is overwritten by each send, so it takes them joined.
func (a *Agent) finishWithDirect(ctx context.Context, turn turnIO, texts []string) error {
	if a.summaryTarget(turn) != "" {
		texts = []string{strings.Join(texts, "\n")}
	}
	for _, text := range texts {
		if err := a.deliverOrReport(ctx, turn, text, nothingWithheld); err != nil {
			return err
		}
	}
	a.clearTurnMarks(ctx)
	a.beats.reply()
	return nil
}
