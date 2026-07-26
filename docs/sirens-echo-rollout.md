# Sirens Echo k3s rollout

Sirens Echo runs as an isolated k3s worker. Her lifecycle never changes the
native admin control bot.

## Operator prerequisites

Before raising the Deployment above zero replicas, an authorized operator:

1. Confirms [inbox#271](https://forgejo.coilysiren.me/coilysiren/inbox/issues/271)
   records the selected community model route.
2. Stores that selected model at `/sirens-echo/agent-proxy-model`.
3. Confirms Agent Proxy advertises the selected model.
4. Creates a Forgejo token limited to ordinary issue reads and creates in
   `coilyco-gaming/sirens-discord-ops`.
5. Stores that token at `/sirens-echo/forgejo-issue-token`.
6. Confirms `/sirens-echo/discord-bot-token` and `/discord/channel/bots`
   resolve without printing either value.
7. Enables the Discord application Message Content intent.
8. Limits the Discord application to view, read history, and send in `#bots`.
9. Sets the application repository variable `IN_CLUSTER_REGISTRY_PUSH`.

No production secret belongs in a tracked file, shell history, issue, or chat.

## Enable and verify

The application main workflow publishes the reviewed `sirens-echo` image. The
deploy repository initially keeps
`services/sirens-echo/values.env` at `REPLICAS=0`.

An authorized operator runs `ward exec eval-echo`, changes the deploy value to
`REPLICAS=1`, and lands that deploy-repo change. The deploy runner applies the
singleton worker with a `Recreate` strategy.

The operator then summons Sirens Echo once in `#bots`.

Verification requires:

- Both accepted Agent Proxy cases pass.
- Sirens Echo responds only to the direct summon in `#bots`.
- The reply pings nobody.
- The unknown-answer case creates or reuses an ordinary unlabeled issue.
- The issue contains no member identity, Discord identifier, raw quotation, or
  private context.
- The admin control bot remains healthy.

## Failure behavior

Missing SSM values fail before the Discord session opens. An unavailable model
or Agent Proxy returns a neutral retry message. A malformed or ungrounded
model response never reaches Discord or Forgejo. ExternalSecret keeps the pod
without AWS credentials.

Forgejo failure does not cause the model to claim an issue was created. The
member-facing answer still posts, while the service log records the failed
follow-through.

## Rollback

The operator changes the deploy repository's `REPLICAS` value to `0` and lands
that change. Reconciliation keeps the worker at zero instead of recreating it.

Rollback leaves the admin bot, application image, skillpack, and credentials
intact. After repair, the operator reruns the evaluation gate before restoring
one replica.

## Deployment ownership

Engineering authors and validates this repository. An authorized operator
provisions credentials, changes Discord application intents, raises the k3s
replica count, runs live acceptance, and records evidence on issue #31.

See [sirens-echo.md](sirens-echo.md) for the request and feedback contracts.
