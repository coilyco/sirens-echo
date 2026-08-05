# Sirens Echo configuration and evaluation

## Agent and model configuration

`agent/sirens-echo.json` pins audit attribution, channel, history budget, local
policy roots, and MCP roster. It pins no backend model. The schema has no
behavioral role or seat. Its audit role is not added to the model prompt.

Agent Proxy is the inference transport. Deployment reads the selected
`<namespace>/<alias>` route from `/sirens-echo/agent-proxy-model` and passes it
through without a source default. Echo uses the same value for inference and
non-generating route readiness.

Every model round requests OpenAI-compatible JSON object mode at temperature
zero. The runtime supplies only its neutral local policy, untrusted bounded
history, and the current request. Echo attempts one same-conversation repair
for invalid JSON or personified language with tools disabled, then fails closed
before Discord or Forgejo.

An MCP entry has either a literal `url` or a deployment-owned `url_env`. The
Forgejo entry uses `SIRENS_ECHO_FORGEJO_MCP_URL`, so the source definition
selects the server while deploy owns its cluster-local endpoint.

## Knowledge gaps and corrections

An unanswered question produces a `knowledge-gap` draft. An explicit
correction produces a `correction` draft. These values affect only the issue
title prefix. The runtime applies no labels.

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

`ward exec policy-check` verifies the real tracked neutral policy. `ward exec test`
exercises the offline harness and an official MCP server fixture.
`ward exec eval-echo` omits environment-backed MCP servers and gives each
unknown-answer, correction, live Eco, and neutral-capability case its own
five-minute deadline, without a Discord or Forgejo write.
