# Sirens Echo k3s rollout

Sirens Echo runs as an isolated k3s worker.

## Model and artifact checks

Before raising replicas, an authorized operator:

1. Confirms [inbox#271](https://forgejo.coilysiren.me/coilysiren/inbox/issues/271)
   records the selected Community model.
2. Stores it at `/sirens-echo/agent-proxy-model`.
3. Confirms Agent Proxy advertises it.
4. Confirms `ward exec bundle-check` verifies the AOS Community bundle.
5. Confirms the reviewed full-SHA image exists in Forgejo OCI.

## Credential and access checks

The operator then:

1. Stores an issue-read/create token at
   `/sirens-echo/forgejo-issue-token`.
2. Confirms the Discord token and `#bots` identifier resolve without printing
   either value.
3. Enables Message Content and limits Echo to view, read history, and send in
   `#bots`.
4. Confirms deploy uses the exact image through its read-only pull credential.
5. Confirms access to Agent Proxy, public Eco MCP, and the SigNoZ collector.

Eco MCP needs no client credential or tailnet identity. No secret belongs in a
tracked file, shell history, issue, or chat.

## Enable and verify

The application workflow publishes the reviewed image. Deploy initially keeps
`services/sirens-echo/values.env` at `REPLICAS=0`.

The operator runs `ward exec eval-echo`, lands `REPLICAS=1` in deploy, and
summons Echo in `#bots`. Verification requires:

- All three evaluation cases pass, including live Eco status
- Echo responds only to a direct summon and pings nobody
- A second summon can refer to the first message and Echo reply
- SigNoZ shows one joined turn trace through Eco MCP and reply
- Trace logs contain all accepted prompt, model, tool, and reply content
- Turn, latency, model, tool, and failure metrics exist
- The unknown case creates or reuses an ordinary unlabeled issue
- The issue contains no identity, Discord identifier, quote, or private data

## Failure behavior

Missing SSM values fail before Discord opens. Agent Proxy, MCP, exhausted-loop,
or validation failures return a neutral retry reply. Invalid output never
reaches Discord or Forgejo. ExternalSecret gives the pod no AWS credentials.

Forgejo failure never makes Echo claim an issue exists. Her answer still posts
while logs record the failed follow-through.

## Rollback

The operator lands `REPLICAS=0` in deploy. Reconciliation leaves Echo stopped
without changing the image, skill, or credentials. After repair, the operator
reruns evaluation before restoring one replica.

## Ownership

This repository authors and validates the harness. An authorized operator
provisions credentials, changes Discord settings, raises replicas, and records
live evidence. Deploy owns reconciliation and rollback.

See [the harness](sirens-echo.md),
[MCP tools](sirens-echo-tools.md), and
[observability](sirens-echo-observability.md).
