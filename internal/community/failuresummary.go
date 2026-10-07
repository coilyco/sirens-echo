package community

import (
	"context"
	"errors"
	"fmt"
	"strings"
)

// Labels for the Complete returns failureCause leaves in stage_failed. They
// add no class, so no notice or alert moves. See COI-2058.
var (
	ErrRepairToolCall = errors.New("tool call during response repair")
	ErrNoToolRoster   = errors.New("tool requested with no MCP roster")
	ErrNoChoices      = errors.New("Agent Proxy response contained no choices")
	ErrModelTransport = errors.New("Agent Proxy request failed in transport")
)

// summaryReasons is the closed set a summary may name. Error text is never
// copied, because it can carry member text, URLs and tokens.
var summaryReasons = []struct {
	sentinel error
	label    string
}{
	{errShuttingDown, "shutting_down"},
	{ErrModelSilent, "model_silent"},
	{context.DeadlineExceeded, "deadline_exceeded"},
	{context.Canceled, "canceled"},
	{ErrToolRoundsExhausted, "tool_rounds_exhausted"},
	{ErrResponseRepairExhausted, "response_repair_exhausted"},
	{ErrBudgetExhausted, "budget_exhausted"},
	{ErrResponseTooLarge, "response_too_large"},
	{ErrResponseUnreadable, "response_unreadable"},
	{ErrRepairToolCall, "repair_tool_call"},
	{ErrNoToolRoster, "no_tool_roster"},
	{ErrNoChoices, "no_choices"},
	{ErrModelTransport, "model_transport"},
	{ErrReplySilent, "reply_silent"},
}

// errorSummary names what failed without quoting it: a closed reason, any HTTP
// status, and the Go type chain. Types are code, so they carry no runtime data.
func errorSummary(err error) string {
	if err == nil {
		return ""
	}
	const maxSummaryBytes = 200
	parts := []string{"reason=" + summaryReason(err)}
	var status modelHTTPError
	if errors.As(err, &status) {
		parts = append(parts, fmt.Sprintf("http_status=%d", status.Status))
	}
	parts = append(parts, "chain="+strings.Join(errorTypeChain(err), ">"))
	summary := strings.Join(parts, " ")
	if len(summary) > maxSummaryBytes {
		summary = summary[:maxSummaryBytes]
	}
	return summary
}

func summaryReason(err error) string {
	if isToolFailure(err) {
		return "tool_failed"
	}
	for _, reason := range summaryReasons {
		if errors.Is(err, reason.sentinel) {
			return reason.label
		}
	}
	if rejectedByModel(err) {
		return "model_rejected"
	}
	return "unlabelled"
}

// errorTypeChain lists each distinct wrapped type once. A join is flattened, so
// a failed notice joined to the cause reads like the cause alone.
func errorTypeChain(err error) []string {
	const (
		maxChainTypes   = 6
		maxTypeNameSize = 48
	)
	var chain []string
	seen := map[string]bool{}
	var walk func(error)
	walk = func(current error) {
		if current == nil || len(chain) >= maxChainTypes {
			return
		}
		name := fmt.Sprintf("%T", current)
		if name != "*errors.joinError" && !seen[name] {
			seen[name] = true
			chain = append(chain, safeTypeName(name, maxTypeNameSize))
		}
		switch wrapped := current.(type) {
		case interface{ Unwrap() error }:
			walk(wrapped.Unwrap())
		case interface{ Unwrap() []error }:
			for _, child := range wrapped.Unwrap() {
				walk(child)
			}
		}
	}
	walk(err)
	return chain
}

// safeTypeName keeps identifier characters and a bound, so safety does not
// depend on every type in the program.
func safeTypeName(name string, limit int) string {
	var kept strings.Builder
	for _, r := range name {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9',
			r == '.', r == '*', r == '_':
			kept.WriteRune(r)
		}
		if kept.Len() >= limit {
			break
		}
	}
	return kept.String()
}
