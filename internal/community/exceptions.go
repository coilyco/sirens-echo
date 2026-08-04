package community

import (
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

type exceptionCode uint8

const (
	exceptionUnclassified exceptionCode = iota
	exceptionTurnFailed
	exceptionHistoryFailed
	exceptionResponseValidationFailed
	exceptionForgejoIssueFailed
	exceptionDiscordReplyFailed
	exceptionReplyFailed
	exceptionMCPToolsListFailed
	exceptionMCPSessionCloseFailed
	exceptionMCPToolCallFailed
	exceptionModelRequestMarshalFailed
	exceptionModelRequestBuildFailed
	exceptionModelTransportFailed
	exceptionModelResponseReadFailed
	exceptionModelResponseTooLarge
	exceptionModelResponseHTTPError
	exceptionModelResponseDecodeFailed
	exceptionModelResponseMissingChoice
	exceptionHTTPTurnMethodNotAllowed
	exceptionHTTPTurnInvalidJSON
	exceptionHTTPTurnContentRequired
	exceptionHTTPTurnInputTooLong
	exceptionHTTPTurnHistoryTooLong
	exceptionCodeCount
)

type exceptionSpec struct {
	typeName string
	message  string
	stage    string
	outcome  string
}

var exceptionCatalog = [exceptionCodeCount]exceptionSpec{
	exceptionUnclassified: {
		typeName: "sirens_echo.telemetry.unclassified",
		message:  "An unclassified runtime failure occurred.",
		stage:    "telemetry",
		outcome:  "unclassified",
	},
	exceptionTurnFailed: {
		typeName: "sirens_echo.turn.failed",
		message:  "Turn processing failed.",
		stage:    "turn",
		outcome:  "failed",
	},
	exceptionHistoryFailed: {
		typeName: "sirens_echo.history.read_failed",
		message:  "Message history retrieval failed.",
		stage:    "history",
		outcome:  "read_failed",
	},
	exceptionResponseValidationFailed: {
		typeName: "sirens_echo.response.validation_failed",
		message:  "Response validation failed.",
		stage:    "validation",
		outcome:  "failed",
	},
	exceptionForgejoIssueFailed: {
		typeName: "sirens_echo.forgejo.issue_failed",
		message:  "Forgejo issue follow-up failed.",
		stage:    "forgejo",
		outcome:  "issue_failed",
	},
	exceptionDiscordReplyFailed: {
		typeName: "sirens_echo.discord.reply_failed",
		message:  "Discord reply delivery failed.",
		stage:    "reply",
		outcome:  "discord_failed",
	},
	exceptionReplyFailed: {
		typeName: "sirens_echo.reply.failed",
		message:  "Reply delivery failed.",
		stage:    "reply",
		outcome:  "failed",
	},
	exceptionMCPToolsListFailed: {
		typeName: "sirens_echo.mcp.tools_list_failed",
		message:  "MCP tool discovery failed.",
		stage:    "mcp",
		outcome:  "tools_list_failed",
	},
	exceptionMCPSessionCloseFailed: {
		typeName: "sirens_echo.mcp.session_close_failed",
		message:  "MCP session cleanup failed.",
		stage:    "mcp",
		outcome:  "session_close_failed",
	},
	exceptionMCPToolCallFailed: {
		typeName: "sirens_echo.mcp.tool_call_failed",
		message:  "MCP tool call failed.",
		stage:    "mcp",
		outcome:  "tool_call_failed",
	},
	exceptionModelRequestMarshalFailed: {
		typeName: "sirens_echo.model.request_marshal_failed",
		message:  "Agent Proxy request encoding failed.",
		stage:    "model",
		outcome:  "request_marshal_failed",
	},
	exceptionModelRequestBuildFailed: {
		typeName: "sirens_echo.model.request_build_failed",
		message:  "Agent Proxy request construction failed.",
		stage:    "model",
		outcome:  "request_build_failed",
	},
	exceptionModelTransportFailed: {
		typeName: "sirens_echo.model.transport_failed",
		message:  "Agent Proxy transport failed.",
		stage:    "model",
		outcome:  "transport_failed",
	},
	exceptionModelResponseReadFailed: {
		typeName: "sirens_echo.model.response_read_failed",
		message:  "Agent Proxy response read failed.",
		stage:    "model",
		outcome:  "response_read_failed",
	},
	exceptionModelResponseTooLarge: {
		typeName: "sirens_echo.model.response_too_large",
		message:  "Agent Proxy response exceeded the size limit.",
		stage:    "model",
		outcome:  "response_too_large",
	},
	exceptionModelResponseHTTPError: {
		typeName: "sirens_echo.model.response_http_error",
		message:  "Agent Proxy returned an unsuccessful HTTP status.",
		stage:    "model",
		outcome:  "response_http_error",
	},
	exceptionModelResponseDecodeFailed: {
		typeName: "sirens_echo.model.response_decode_failed",
		message:  "Agent Proxy response decoding failed.",
		stage:    "model",
		outcome:  "response_decode_failed",
	},
	exceptionModelResponseMissingChoice: {
		typeName: "sirens_echo.model.response_missing_choice",
		message:  "Agent Proxy response contained no choice.",
		stage:    "model",
		outcome:  "response_missing_choice",
	},
	exceptionHTTPTurnMethodNotAllowed: {
		typeName: "sirens_echo.http.turn_method_not_allowed",
		message:  "The HTTP turn method is not allowed.",
		stage:    "http",
		outcome:  "method_not_allowed",
	},
	exceptionHTTPTurnInvalidJSON: {
		typeName: "sirens_echo.http.turn_invalid_json",
		message:  "The HTTP turn body is not valid JSON.",
		stage:    "http",
		outcome:  "invalid_json",
	},
	exceptionHTTPTurnContentRequired: {
		typeName: "sirens_echo.http.turn_content_required",
		message:  "The HTTP turn content is missing.",
		stage:    "http",
		outcome:  "content_required",
	},
	exceptionHTTPTurnInputTooLong: {
		typeName: "sirens_echo.http.turn_input_too_long",
		message:  "The HTTP turn input exceeded the size limit.",
		stage:    "http",
		outcome:  "input_too_long",
	},
	exceptionHTTPTurnHistoryTooLong: {
		typeName: "sirens_echo.http.turn_history_too_long",
		message:  "The HTTP turn history exceeded the context limit.",
		stage:    "http",
		outcome:  "history_too_long",
	},
}

func exceptionFor(code exceptionCode) exceptionSpec {
	if code >= exceptionCodeCount {
		return exceptionCatalog[exceptionUnclassified]
	}
	spec := exceptionCatalog[code]
	if spec.typeName == "" || spec.message == "" || spec.stage == "" || spec.outcome == "" {
		return exceptionCatalog[exceptionUnclassified]
	}
	return spec
}

// MarkSpanError records one cataloged exception and marks its span as failed.
// The typed catalog prevents dynamic runtime data from entering exception fields.
func (t *Telemetry) MarkSpanError(span trace.Span, code exceptionCode) {
	spec := exceptionFor(code)
	attributes := []attribute.KeyValue{
		attribute.String("exception.type", spec.typeName),
		attribute.String("exception.message", spec.message),
		attribute.String("error.stage", spec.stage),
		attribute.String("error.outcome", spec.outcome),
	}
	span.AddEvent("exception", trace.WithAttributes(attributes...))
	span.SetAttributes(
		attribute.String("error.type", spec.typeName),
		attribute.String("error.stage", spec.stage),
		attribute.String("error.outcome", spec.outcome),
	)
	span.SetStatus(codes.Error, spec.message)
}
