# Sirens Echo MCP tools

The source-controlled MCP roster is the Community harness's tool boundary.
The first roster contains only `https://eco-app.coilysiren.me/mcp`. It needs
no client credential or tailnet identity.

## Tool loop

The harness uses the official Go MCP SDK and streamable HTTP transport. At
the start of each accepted turn, it:

1. Opens every configured MCP session.
2. Lists each published tool, description, and input schema.
3. Exposes each model name as `<server>__<tool>`.
4. Sends those schemas with the Agent Proxy request.
5. Executes valid model-requested tools.
6. Appends assistant calls and tool results, then continues Agent Proxy.

Agent Proxy can return a response containing only tool calls. Final content
can use an OpenAI-compatible string, text-part array, or text object.
Continuation stops after six tool rounds.

## Validation and failure

The harness rejects:

- Empty, colliding, or overlong model-facing tool names
- Tool calls missing an identifier or function name
- Calls to tools absent from the discovered roster
- Tool arguments that are not JSON objects
- A response with neither tool calls nor usable content
- More than six tool rounds

Connection, discovery, and invocation failures stop the turn and produce the
neutral retry reply. MCP error results returned as data remain available to
Agent Proxy for a grounded explanation.

## Ownership

The roster lives in `agent/sirens-echo.json`, not in the inference client.
Adding another MCP server is a reviewed definition change. Each server owns
its tool descriptions and behavior.

Forgejo MCP, a Sirens tailnet identity, reactions, and general Discord actions
are not part of this slice. The existing narrow Forgejo issue client remains
separate from the model tool roster.

## Acceptance coverage

The official in-process MCP fixture proves:

- Schema discovery from a real MCP server
- A tool-call-only first model response
- Eco status invocation
- Complete tool result continuation
- A grounded member-facing reply
- Alternate compatible content forms

The live evaluation adds a required
`eco__get_eco_server_status` call without sending Discord or Forgejo writes.

See [the harness](sirens-echo.md) and
[observability](sirens-echo-observability.md).
