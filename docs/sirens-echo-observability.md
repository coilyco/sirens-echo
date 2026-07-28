# Sirens Echo observability

Accepted `#bots` turns are intentionally fully observable. Kubernetes stdout
collection carries JSON logs to the same SigNoZ stack that receives OTLP/HTTP
traces and metrics.

## Content logs

The runtime logs:

- Current member input and rendered history
- Full system and user prompts
- MCP schemas, tool arguments, and tool results
- Full Agent Proxy requests and responses for every round
- Final validated reply
- Stage and follow-through failures

Every content log inside a trace carries `trace_id` and `span_id`. Discord and
Forgejo tokens, authorization headers, and SSM values are never fields in
these structures.

DMs, other channels, bot messages, self messages, duplicates, and messages
without a summon are rejected before turn logging.

## Joined traces

One root `discord.turn` span joins:

- `discord.receive`
- `discord.history`
- `context.assemble`
- `aos.bundle.load`
- `mcp.tools.list`
- One `model.chat` per Agent Proxy round
- One `mcp.tool.call` per tool call
- `response.validate`
- Optional `forgejo.issue.ensure`
- `discord.reply`

The Agent Proxy and MCP HTTP transports also inherit the active trace context.
The root records outcome and duration before it ends.

## Metrics

The baseline instruments are:

- `sirens_echo.turns`
- `sirens_echo.turn.duration`
- `sirens_echo.model.calls`
- `sirens_echo.tool.calls`
- `sirens_echo.failures`

Low-cardinality outcome and stage attributes distinguish success and failure.
MCP call metrics also identify the configured server and published tool.

The application default targets an in-cluster SigNoZ collector. Production
sets `OTEL_EXPORTER_OTLP_ENDPOINT` to the established cross-cluster ser8
receiver because Echo runs on kai-server.

## Acceptance coverage

The joined-turn test runs history, two Agent Proxy rounds, an Eco tool result,
validation, and reply through one trace. It asserts every required span shares
one trace identifier and that logs retain all prompt, model, tool, and reply
content with trace correlation.

See [the harness](sirens-echo.md),
[MCP tools](sirens-echo-tools.md), and
[rollout checks](sirens-echo-rollout.md).
