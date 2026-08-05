# Sirens Echo k3s rollout

Sirens Echo runs as an isolated k3s worker.

## Model and artifact checks

Before raising replicas, an authorized operator:

1. Confirms [inbox#271](https://forgejo.coilysiren.me/coilysiren/inbox/issues/271)
   records the selected Community model.
2. Stores it at `/sirens-echo/agent-proxy-model`.
3. Confirms Agent Proxy advertises it.
4. Confirms `ward exec policy-check` verifies the tracked neutral model policy.
5. Confirms the reviewed full-SHA image exists in Forgejo OCI.

## Credential and access checks

The operator then:

1. Confirms `/forgejo/coilyco-ops-gaming/sirens-echo-issue-token` exists without printing it.
2. Confirms only the private Forgejo MCP ExternalSecret references that token.
3. Confirms the Discord token and `#bots` identifier resolve without printing either value.
4. Enables Message Content and limits Echo to view, read history, and send in
   `#bots`.
5. Confirms deploy uses exact Echo and Ward MCP image SHAs with its read-only pull credential.
6. Confirms access to Agent Proxy, Eco MCP, private Forgejo MCP, and SigNoZ.

Eco MCP needs no client credential or tailnet identity. Echo receives the
private MCP's ClusterIP URL but no Forgejo credential. No secret belongs in a
tracked file, shell history, issue, or chat.

## Enable and verify

The application workflow publishes the reviewed image. Deploy rolls the
private Forgejo MCP to readiness before Echo, then advances the reviewed Echo
image.

The operator runs `ward exec eval-echo`, lands `REPLICAS=1` in deploy, and
summons Echo in `#bots`. Verification requires:

- All four evaluation cases pass, including live Eco status
- The neutral-capability case contains no greeting, emoji, first-person voice,
  self-description, banter, sign-off, or open-ended offer
- Echo responds only to a direct summon and pings nobody
- A second summon can refer to the first message and Echo reply
- SigNoZ shows one joined turn trace through Eco MCP and reply
- Trace-correlated logs retain safe metadata and byte counts without prompt,
  model, tool, or reply bodies
- Turn, latency, model, tool, and failure metrics exist
- The unknown case creates or reuses an ordinary unlabeled issue
- The issue contains no identity, Discord identifier, quote, or private data
- The Forgejo MCP publishes only its reviewed issue and label tools
- Echo's container has the MCP URL and no `FORGEJO_TOKEN`

## Failure behavior

Missing SSM values fail before either workload becomes ready. Agent Proxy,
MCP, exhausted-loop, response-contract, or validation failures return a neutral retry reply. Echo
requests decoder-enforced JSON and attempts one bounded repair. Invalid output
never reaches Discord or Forgejo. ExternalSecret gives the pod no AWS
credentials.

Forgejo failure never makes Echo claim an issue exists. The answer still posts
while logs record the failed follow-through.

## Rollback

The operator restores the prior deploy commit. That returns Echo to the prior
image and credential boundary. Reconciliation can also leave Echo stopped at
zero replicas without changing the MCP. After repair, the operator reruns
evaluation before restoring one replica.

## Ownership

This repository authors and validates the service. An authorized operator
provisions credentials, changes Discord settings, raises replicas, and records
live evidence. Deploy owns the private MCP guardfile, reconciliation, and
rollback.

See [the service](sirens-echo.md), [MCP tools](sirens-echo-tools.md), and [observability](sirens-echo-observability.md).
