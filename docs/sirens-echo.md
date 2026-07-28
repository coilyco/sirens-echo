# Sirens Echo Discord Community harness

Sirens Echo is the first Discord Community harness. She binds the canonical
AOS Community role to a small Discord surface, Agent Proxy, source-controlled
MCP servers, Sirens-local knowledge, Forgejo feedback, and joined telemetry.

This repository owns the harness. AOS and agent-compose own the role, shared
skills, seat, and personality meld. Agent Proxy owns inference transport. MCP
servers own their published tools. Deploy owns the k3s workload.

## Request flow

1. Discord delivers a guild message.
2. Echo accepts only a mention or reply in configured `#bots`.
3. The runtime loads up to 12 earlier messages and marks them untrusted.
4. The harness combines AOS context, the local skill, history, and the summon.
5. It discovers MCP schemas and sends them to Agent Proxy.
6. It executes model-requested tools and continues with their results.
7. It enforces JSON, repairs once, validates, and may create or reuse an issue.
8. It replies with every Discord mention disabled.

Turns run one at a time. A second summon reads history after the first turn
finishes, so it can see the prior member message and Echo reply. Echo keeps
only bounded Discord history and duplicate detection. She has no automatic
memory or trajectory store.

## AOS composition

`agent/community-compose.kdl` requests the `community` role, `frontier` model
class, compiled delivery, and public AOS provider. The image build uses its
seeded AOS mirror, then runs `agent-compose compose`, `agent-compose verify`,
and Echo's consumer check.

Echo rejects a bundle with the wrong format or role, non-compiled delivery,
no personality meld, no `aos-public` source, an unsafe entry point, or empty
context. The verified context supplies role instructions and shared skills to
every system prompt.

The final image retains the full AOS release substrate and adds no lore.
`.agents/skills/sirens-echo-community/` contributes only reviewed
Sirens-specific knowledge and follow-through behavior.

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

See [MCP tool behavior](sirens-echo-tools.md),
[observability](sirens-echo-observability.md), and
[rollout checks](sirens-echo-rollout.md).
