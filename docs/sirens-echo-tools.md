# Runtime MCP tools

Each source-controlled definition owns its own MCP roster and optional
automatic issue tracker.

The Sirens Echo community definition contains public Eco MCP and an
environment-backed private Forgejo MCP URL. Eco needs no client credential or
tailnet identity. Echo sends no credential to the private MCP. The separate
MCP workload holds its Forgejo token.

The CoilyCo general-purpose definition begins with an empty MCP roster and no
automatic issue tracker. This is deliberate. General-purpose describes topic
scope, not universal tool or mutation authority.

## Tool loop

When a definition configures MCP servers, the harness:

1. Opens every configured MCP session.
2. Lists each published tool, description, and input schema.
3. Exposes each model name as `<server>__<tool>`.
4. Sends those schemas with the Agent Proxy request.
5. Executes valid model-requested tools.
6. Appends assistant calls and tool results, then continues Agent Proxy.

An empty roster skips discovery and sends no tools. Agent Proxy can return a
response containing only tool calls when tools exist. Final content can use an
OpenAI-compatible string, text-part array, or text object. Continuation stops
after six tool rounds.

## Validation and failure

The harness rejects:

- Empty, colliding, or overlong model-facing tool names
- Tool calls missing an identifier or function name
- Calls to tools absent from the discovered roster
- Tool arguments that are not JSON objects
- A response with neither tool calls nor usable content
- More than six tool rounds
- An issue draft when the selected definition has no automatic issue tracker

Connection, discovery, and invocation failures stop the turn and produce the
neutral retry reply. MCP error results returned as data remain available to
Agent Proxy for a grounded explanation.

## Sirens Echo issue ownership

Deploy fixes the Sirens Echo Forgejo MCP guardfile to
`coilyco-gaming/sirens-echo`. The published surface includes issue and label
reads, issue creation and closing, comment creation, and label add, replace,
and remove. There is no owner or repository argument to redirect. There is no
issue-body edit, comment edit, delete, reopen, pin, release, pull-request,
repository, organization, or account tool.

The community definition names that server as `issue_tracker`. Its automatic
knowledge-gap reporter calls `list_issue` and `create_issue` through the
MCP's matching HTTP tool API. Echo keeps exact-title reuse and sanitized
unlabeled creation without holding a Forgejo credential.

The CoilyCo definition does not inherit this repository-specific action path.
Future tools belong in its tracked roster only after their scope and authority
are explicitly reviewed.

## Acceptance coverage

The official in-process MCP fixture proves schema discovery, a tool-call-only
first model response, complete tool result continuation, a grounded
user-facing reply, and alternate compatible content forms. Separate profile
tests prove the CoilyCo definition loads without MCP or issue-tracker
dependencies.

The live Echo evaluation selects only static MCP URLs, then requires an
`eco__get_eco_server_status` call without sending Discord or Forgejo writes.

See [the service](sirens-echo.md) and
[observability](sirens-echo-observability.md).
