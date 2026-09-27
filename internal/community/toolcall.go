package community

import (
	"encoding/json"
	"fmt"
	"strings"
)

// Why a requested call could not run. Closed set, so the reason is a safe
// telemetry label and never carries argument text.
const (
	toolCallUnnamed     = "unnamed"
	toolCallUnavailable = "unavailable"
	toolCallBadArgs     = "bad_arguments"
)

// resolveToolCall returns the definition and parsed arguments for a call, or
// the reason it cannot run.
func resolveToolCall(
	call chatToolCall,
	definitions map[string]ToolDefinition,
) (ToolDefinition, map[string]any, string) {
	if call.Function.Name == "" {
		return ToolDefinition{}, nil, toolCallUnnamed
	}
	definition, exists := definitions[call.Function.Name]
	if !exists {
		return ToolDefinition{}, nil, toolCallUnavailable
	}
	arguments := make(map[string]any)
	if strings.TrimSpace(call.Function.Arguments) != "" {
		if err := json.Unmarshal([]byte(call.Function.Arguments), &arguments); err != nil {
			return ToolDefinition{}, nil, toolCallBadArgs
		}
	}
	return definition, arguments, ""
}

// invalidToolCallResult is the tool message the model reads in place of a
// result, naming what to do next rather than only what went wrong.
func invalidToolCallResult(reason, name string) string {
	switch reason {
	case toolCallUnnamed:
		return "Error: the tool call named no tool. Call one of the offered tools by its exact name, or answer without a tool."
	case toolCallBadArgs:
		return fmt.Sprintf(
			"Error: the arguments for %s were not a valid JSON object. Call it again with a JSON object matching its schema, or answer without it.",
			name,
		)
	}
	return fmt.Sprintf(
		"Error: no tool named %q is offered this turn. Call one of the offered tools by its exact name, or answer without a tool.",
		name,
	)
}

// repeatedCallResult answers an exact back-to-back repeat without running it,
// so a model looping on one result is told so.
func repeatedCallResult() string {
	return "This exact call was just made and was not run again. Its result is the " +
		"previous tool message. Use a different call or answer with what you have."
}
