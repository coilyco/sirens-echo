# Sirens Echo configuration and evaluation

## YAML agent and model configuration

`agent/sirens-echo.yaml` pins the neutral Community policy.
`agent/sirens-deep.yaml` selects the social CoilyCo general-purpose policy for
the existing HTTP workload. Each definition pins audit attribution, response
style, optional channel, history budget, local policy roots, optional MCP
roster, and optional automatic issue tracker. Neither pins a backend model or
loads a behavioral role or seat. The audit role is not added to the model
prompt.

Agent Proxy is the inference transport. Deployment reads the selected
`<namespace>/<alias>` route from `/sirens-echo/agent-proxy-model` and passes it
through without a source default. Echo uses the same value for inference and
non-generating route readiness.

Every model round requests OpenAI-compatible JSON object mode at temperature
zero. The runtime supplies the selected repo-local policy, untrusted bounded
history, and the current request. Echo attempts one same-conversation,
style-aware repair with tools disabled, then fails closed before Discord or an
automatic issue write.

`SIRENS_ECHO_DEFINITION` selects the tracked definition. Discord ingress is on
by default. `SIRENS_ECHO_DISCORD_ENABLED=false` removes the token and channel
requirements while retaining the private HTTP turn path. `SIRENS_ECHO_INSTANCE`
sets a distinct lowercase service name for telemetry.

An MCP entry has either a literal `url` or a deployment-owned `url_env`.
The Sirens Echo Forgejo entry uses `SIRENS_ECHO_FORGEJO_MCP_URL`, so the
source definition selects the server while deploy owns its cluster-local
endpoint. The CoilyCo definition intentionally has no MCP entries.

An empty channel is valid only for HTTP-only deployment. Discord-enabled
configuration still requires the tracked `#bots` boundary. An empty MCP
roster is valid. `issue_tracker` is optional and, when present, must name one
configured MCP server.

## Knowledge gaps and corrections

For a definition with `issue_tracker`, an unanswered question produces a
`knowledge-gap` draft and an explicit correction produces a `correction`
draft. These values affect only the issue title prefix. The runtime applies no
labels. A definition without `issue_tracker` must return `issue: null` and
state uncertainty in the reply.

The runtime removes Discord links, mention syntax, and long identifiers from
drafts. It requires a summary without member identity, handles, raw quotes,
secrets, or personal details. The automatic reporter calls the guarded
Forgejo MCP's HTTP tool projection to reuse an exact-title open issue or create
an ordinary issue. A reviewed change updates the local skill and regression
case.

## Boundaries and evaluation

Echo's Discord token needs only `#bots` visibility, history, and reply
permissions. The pod receives a cluster-local MCP URL and no Forgejo token.
The separate MCP pod receives an exact-repository token and a guardfile that
publishes only the reviewed issue and label tools.

`ward exec policy-check` verifies both tracked response policies. `ward exec
test` exercises the offline harness, the zero-tool CoilyCo profile, and an
official MCP server fixture.
`ward exec eval-echo` omits environment-backed MCP servers and gives each
unknown-answer, correction, live Eco, and neutral-capability case its own
five-minute deadline, without a Discord or Forgejo write.
