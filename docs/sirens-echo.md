# Sirens Echo community agent

Sirens Echo is the independently deployed Discord community agent in this
repository. Agent-compose supplies the canonical `siren community host` seat
and community personality. This repository owns the running Discord
integration, source-controlled model selection, skillpack, feedback loop,
tests, and rollback.

## Request flow

1. Discord delivers a guild message to the Sirens Echo Gateway session.
2. The runtime accepts only the configured `#bots` channel. A member must
   mention Sirens Echo or reply to one of her messages.
3. The runtime rejects DMs, other channels, bot authors, self-authored
   messages, and duplicate message deliveries.
4. The runtime loads at most 12 earlier `#bots` messages and marks the whole
   transcript as untrusted member content.
5. Agent Proxy runs the source-controlled `ornith:35b` model with the
   repository skillpack and strict JSON response contract.
6. The runtime rejects malformed output, new channel references absent from
   supplied evidence, and first-person claims about actions it has not
   performed.
7. The runtime optionally creates or reuses an exact-title Forgejo issue.
8. The runtime replies to the triggering message with all Discord mentions
   disabled.

The runtime serializes requests one at a time. It keeps only a bounded
in-process duplicate set and recent Discord history. It has no automatic
memory store.

## Source-controlled agent

`agent/sirens-echo.json` pins the deployed identity, agent-compose seat, role,
accepted model, `#bots` boundary, recent-message budget, and skill roots.

`.agents/skills/sirens-echo-community/` is the first repository-local skill.
Its `SKILL.md` carries the response method and its `references/` directory
carries approved Sirens knowledge. Future Sirens-specific skills belong in
this repository and enter the agent definition through a reviewed commit.

Agent Proxy remains transport. It never becomes the source of persona,
community knowledge, Discord permission, or Forgejo permission.

## Knowledge gaps and corrections

An unanswered question produces a `knowledge-gap` draft. An explicit member
correction to Sirens Echo produces a `correction` draft. Those values control
the issue title prefix only. The runtime applies no labels.

Before writing, the runtime removes Discord message links, mention syntax, and
long Discord-style identifiers. The model must summarize the knowledge or
behavior change without member identity, handles, raw quotes, secrets, or
personal details.

The Forgejo client searches open issues and reuses an exact normalized title.
A new issue records:

- The sanitized gap or possible correction
- The expected repo-local skillpack change
- The requirement for a regression conversation

A reviewed commit updates the skillpack, adds or strengthens the regression
case, and closes the issue through normal repository history.

## Discord and Forgejo boundary

The Sirens Echo token is separate from the admin control bot token. The Echo
runtime can read and reply only through its Discord application permissions.
The application should be granted the smallest guild permissions needed to
view `#bots`, read history, and send replies there.

The runtime sets `allowed_mentions.parse` to an empty list and
`replied_user` to false. Text that resembles a mention remains inert.

The Forgejo token should be dedicated to reading and creating ordinary issues
in `coilyco-gaming/sirens-discord-ops`. The client has no code, pull request,
label, milestone, assignment, comment, close, or repository-administration
method.

## Evaluation

`agent/evaluation.json` carries two deployment cases:

- An unknown event time supplied with an unverified member guess
- An explicit correction to a prior Sirens Echo answer

`ward exec test` runs offline parser, grounding, issue-sanitization,
deduplication, no-label, request-shape, and evaluation tests.

`ward exec eval-echo` sends the two cases through the same Agent Proxy model
and prompt used by Discord. It creates no Discord message and no Forgejo
issue. The gate requires the correct issue draft type and rejects the
case-specific invented channels, sources, staff claims, and unsupported
action claims.

## Operator prerequisites

Before enabling the unit, an authorized operator:

1. Confirms Agent Proxy advertises `ornith:35b`.
2. Creates a repository-scoped Forgejo issue token and stores it at
   `/sirens-echo/forgejo-issue-token`.
3. Confirms `/sirens-echo/discord-bot-token` and
   `/discord/channel/bots` resolve without printing either value.
4. Enables the Discord application Message Content intent and limits its
   guild permissions to `#bots`.
5. Runs the committed `scripts/install-echo.sh`.
6. Runs the committed `ward exec eval-echo` gate and then summons Sirens Echo
   once in `#bots`.

The operator verifies the reply stays in `#bots`, mentions nobody, and creates
an ordinary unlabeled issue only for the knowledge-gap case.

## Rollback

The committed `scripts/disable-echo.sh` disables and stops only
`sirens-echo.service`. The admin bot remains available. Re-enabling the unit
restores the same source-controlled definition and skillpack after the
operator resolves the cause.
