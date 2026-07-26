# Sirens Echo rollout

Sirens Echo has a separate service so enabling, observing, and disabling her
never changes the admin control bot.

## Operator prerequisites

Before enabling the unit, an authorized operator:

1. Confirms Agent Proxy advertises `ornith:35b`.
2. Creates a Forgejo token limited to ordinary issue reads and creates in
   `coilyco-gaming/sirens-discord-ops`.
3. Stores that token at `/sirens-echo/forgejo-issue-token`.
4. Confirms `/sirens-echo/discord-bot-token` and `/discord/channel/bots`
   resolve without printing either value.
5. Enables the Discord application Message Content intent.
6. Limits the Discord application to view, read history, and send in `#bots`.

No production secret belongs in a tracked file, shell history, issue, or chat.

## Enable and verify

The operator runs the committed `scripts/install-echo.sh`. That script installs
the shared units and explicitly enables `sirens-echo.service`.

The operator then runs the committed `ward exec eval-echo` gate and summons
Sirens Echo once in `#bots`.

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
model response never reaches Discord or Forgejo.

Forgejo failure does not cause the model to claim an issue was created. The
member-facing answer still posts, while the service log records the failed
follow-through.

## Rollback

The committed `scripts/disable-echo.sh` disables and stops only
`sirens-echo.service`.

Rollback leaves the admin bot, checkout, skillpack, and credentials intact.
After the cause is repaired, the operator reruns the evaluation gate and
explicit installer.

## Deployment ownership

Engineering authors and validates this repository. An authorized operator
provisions credentials, changes Discord application intents, enables the unit,
runs live acceptance, and records evidence on issue #31.

See [sirens-echo.md](sirens-echo.md) for the request and feedback contracts.
