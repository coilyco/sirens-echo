# Sirens Echo configuration and evaluation

## Agent and model configuration

`agent/sirens-echo.json` pins identity, seat, role, channel, history budget,
local skill roots, and MCP roster. It pins no backend model.

Agent Proxy is the inference transport. AOSH owns model selection. Deployment
reads the selected model from `/sirens-echo/agent-proxy-model` and passes it
through without a source default.

Every model round requests OpenAI-compatible JSON object mode at temperature
zero. Echo attempts one same-conversation repair with tools disabled, then
fails closed before Discord or Forgejo.

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
permissions. Her pod receives a cluster-local MCP URL and no Forgejo token.
The separate MCP pod receives an exact-repository token and a guardfile that
publishes only the reviewed issue and label tools.

`ward exec bundle-check` verifies the real AOS bundle. `ward exec test`
exercises the offline harness and an official MCP server fixture.
`ward exec eval-echo` omits environment-backed MCP servers and gives each
unknown-answer, correction, and live Eco case its own five-minute deadline,
without a Discord or Forgejo write.
