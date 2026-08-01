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

## Knowledge gaps and corrections

An unanswered question produces a `knowledge-gap` draft. An explicit
correction produces a `correction` draft. These values affect only the issue
title prefix. The runtime applies no labels.

The runtime removes Discord links, mention syntax, and long identifiers from
drafts. It requires a summary without member identity, handles, raw quotes,
secrets, or personal details. The Forgejo client reuses an exact-title open
issue or creates an ordinary issue. A reviewed change updates the local skill
and regression case.

## Boundaries and evaluation

Echo's Discord token needs only `#bots` visibility, history, and reply
permissions. Her Forgejo client can list and create ordinary issues only.

`ward exec bundle-check` verifies the real AOS bundle. `ward exec test`
exercises the offline harness and an official MCP server fixture.
`ward exec eval-echo` gives each unknown-answer, correction, and live Eco case
its own five-minute deadline, without a Discord or Forgejo write.
