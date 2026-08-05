# Sirens Echo MCP tools

The source-controlled MCP roster is the Sirens Echo service's tool boundary. It
contains public Eco MCP and an environment-backed private Forgejo MCP URL. Eco
needs no client credential or tailnet identity. Echo also sends no credential
to the private MCP. The separate MCP workload holds its Forgejo token.

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

Deploy fixes the Forgejo MCP guardfile to `coilyco-gaming/sirens-echo`. The
published surface includes issue and label reads, issue creation and closing,
comment creation, and label add, replace, and remove. There is no owner or
repository argument to redirect. There is no issue-body edit, comment edit,
delete, reopen, pin, release, pull-request, repository, organization, or
account tool.

The automatic knowledge-gap reporter calls `list_issue` and `create_issue`
through the MCP's matching HTTP tool API. That is the same guarded handler the
model reaches over MCP. Echo keeps exact-title reuse and sanitized unlabeled
creation without holding a Forgejo credential. It requests at most three exact
title matches so the continuation stays within the model context budget.

For unanswered questions and explicit corrections, the model returns an issue
draft instead of calling a mutation tool. The runtime sanitizes and performs
that follow-up. Direct Forgejo tools are for explicit repository requests and
may report success only from the returned tool result.

## Acceptance coverage

The official in-process MCP fixture proves:

- Schema discovery from a real MCP server
- A tool-call-only first model response
- Eco status invocation
- Complete tool result continuation
- A grounded member-facing reply
- Alternate compatible content forms

The live evaluation selects only static MCP URLs, then requires an
`eco__get_eco_server_status` call without sending Discord or Forgejo writes.

See [the service](sirens-echo.md) and
[observability](sirens-echo-observability.md).
