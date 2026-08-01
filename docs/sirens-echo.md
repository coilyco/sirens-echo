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
5. It discovers public Eco and private Forgejo MCP schemas and sends them to
   Agent Proxy.
6. It executes model-requested tools and continues with their results.
7. It enforces JSON, repairs once, validates, and may create or reuse an issue.
8. It replies with every Discord mention disabled.

Turns run one at a time. A second summon reads history after the first turn
finishes, so it can see the prior member message and Echo reply. Echo keeps
only bounded Discord history and duplicate detection. She has no automatic
memory or trajectory store.

## Tailnet HTTP test entrypoint

The Discord gateway is the production delivery boundary. For an operator test,
the same `runTurn` path is available through `POST /v1/turn` on the process's
private HTTP listener. The endpoint accepts a JSON object with `author`,
`content`, an optional `request_id`, and optional bounded `history` entries. It
returns the validated reply without sending a Discord message. This bypasses
only Discord's channel, mention, and duplicate gates. Agent Proxy, MCP tool
calls, response validation, grounding, and guarded Forgejo issue handling
remain the same.

The process binds to `127.0.0.1:8080` by default. The k3s deployment sets
`SIRENS_ECHO_HTTP_ADDR=0.0.0.0:8080` and exposes it only through Echo's
Tailscale sidecar. From an authorized tailnet client, send a request like:

```sh
curl -sS http://sirens-echo:8080/v1/turn \
  -H 'Content-Type: application/json' \
  -d '{"author":"manual test","content":"What is the current Eco server status?"}'
```

`GET /healthz` checks only local listener liveness. `GET /readyz` separately
checks the configured logical route through Agent Proxy without inference.
Both routes emit bounded metrics and no logs or spans. See the
[health contract](sirens-echo-health.md).

The deploy bundle adds a private ClusterIP Service but no public Ingress,
certificate, DNS record, or NodePort. `POST /v1/turn` remains traced. Its server
span extracts W3C context and parents the shared Community turn.

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

See [MCP tool behavior](sirens-echo-tools.md),
[observability](sirens-echo-observability.md), and
[rollout checks](sirens-echo-rollout.md).

For runtime configuration, issue handling, and evaluation boundaries, see
[configuration and evaluation](sirens-echo-config.md).
