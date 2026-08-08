# Sirens Echo response service

Sirens Echo binds a deployment-selected response profile to Discord or private
HTTP ingress, Agent Proxy, MCP servers, Sirens knowledge, Forgejo, and
telemetry.

This repository owns the service and its model-facing policy. Agent
Proxy owns inference transport. MCP servers own their published tools. Deploy
owns the k3s workload.

## Request flow

1. Discord delivers a guild message.
2. Echo accepts only a mention or reply in configured `#bots` or in a thread
   whose parent is that channel.
3. The runtime loads up to 12 earlier messages and marks them untrusted. Inside
   a thread that history and the reply stay in the thread.
4. The harness combines the selected local policy, history, and the summon.
5. It discovers the selected definition's MCP schemas and sends them to Agent
   Proxy. Sirens Echo selects public Eco and private Forgejo.
6. It executes model-requested tools and continues with their results.
7. It enforces JSON, repairs once, validates, and may create or reuse an issue.
8. It applies the selected style contract and replies with every Discord
   mention disabled.

Turns run one at a time. A second summon reads history after the first turn
finishes, so it can see the prior member message and Echo reply. Echo keeps
only bounded Discord history and duplicate detection. It has no automatic
memory or trajectory store.

## Tailnet HTTP test entrypoint

The default deployment uses Discord as its production boundary. The same
`runTurn` path is available through `POST /v1/turn` on the process's
private HTTP listener. The endpoint accepts a JSON object with `author`,
`content`, an optional `request_id`, and optional bounded `history` entries. It
returns the validated reply without sending a Discord message. This bypasses
only Discord's channel, mention, and duplicate gates. Agent Proxy, MCP tool
calls, response validation, grounding, and guarded Forgejo issue handling
remain the same. A deployment can disable Discord entirely and use this as its
only ingress.

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

## Deployment-selected response policy

Deploy selects the neutral Sirens Echo community profile or the social CoilyCo
general-purpose profile. The profiles do not share knowledge or tools. Sirens
Echo retains its approved community knowledge, Eco MCP, private Forgejo MCP,
and automatic issue workflow. CoilyCo starts HTTP-only with one domain-neutral
policy root, no MCP roster, and no automatic issue writes.

Neither profile loads an Agent Compose role bundle. Build-time verification
checks both profiles. See [response profiles](response-profiles.md).

See [MCP tool behavior](sirens-echo-tools.md), [observability](sirens-echo-observability.md), and [rollout checks](sirens-echo-rollout.md).

For runtime configuration, issue handling, and evaluation boundaries, see
[configuration and evaluation](sirens-echo-config.md).
