# Sirens Echo Discord response service

Sirens Echo binds a neutral policy to Discord, Agent Proxy, MCP servers, Sirens
knowledge, Forgejo, and telemetry.

This repository owns the service and its model-facing policy. Agent
Proxy owns inference transport. MCP servers own their published tools. Deploy
owns the k3s workload.

## Request flow

1. Discord delivers a guild message.
2. Echo accepts only a mention or reply in configured `#bots`.
3. The runtime loads up to 12 earlier messages and marks them untrusted.
4. The harness combines the neutral local policy, history, and the summon.
5. It discovers public Eco and private Forgejo MCP schemas and sends them to
   Agent Proxy.
6. It executes model-requested tools and continues with their results.
7. It enforces JSON, repairs once, validates, and may create or reuse an issue.
8. It rejects personified language and replies with every Discord mention disabled.

Turns run one at a time. A second summon reads history after the first turn
finishes, so it can see the prior member message and Echo reply. Echo keeps
only bounded Discord history and duplicate detection. It has no automatic
memory or trajectory store.

## Tailnet HTTP test entrypoint

The Discord gateway is the production boundary. For an operator test, the same
`runTurn` path is available through `POST /v1/turn` on the process's
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

## Neutral response policy

The model receives no Agent Compose role, seat, identity card, personality, or
shared behavioral context. `agent/sirens-echo.json` keeps `audit_role` only for
request attribution. That value is not rendered into the model prompt or used
to select the Agent Proxy route.

`.agents/skills/sirens-echo-community/` owns the complete model-facing response
policy, reviewed Sirens knowledge, and follow-through behavior. The system
prompt requires direct, factual, impersonal language. The runtime rejects
greetings, first-person or collective voice, emojis, exclamation marks, banter,
sign-offs, and open-ended offers. Agent Proxy gets one same-conversation repair
with tools disabled before the turn fails closed.

`ward exec policy-check` loads the real tracked definition and skill root, then
verifies that the rendered prompt contains the neutral policy and no composed
persona surface. The image build runs the same check. The final image retains
the full AOS release substrate for its runtime environment but loads no AOS or
Agent Compose context and adds no lore.

See [MCP tool behavior](sirens-echo-tools.md), [observability](sirens-echo-observability.md), and [rollout checks](sirens-echo-rollout.md).

For runtime configuration, issue handling, and evaluation boundaries, see
[configuration and evaluation](sirens-echo-config.md).
