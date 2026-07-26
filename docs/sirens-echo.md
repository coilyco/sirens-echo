# Sirens Echo community agent

Sirens Echo is the independently deployed Discord community agent in this
repository. Agent-compose supplies the canonical `siren community host` seat.
This repository owns the running integration, model-opaque route declaration,
skillpack, feedback loop, tests, and rollback.

## Request flow

1. Discord delivers a guild message to the Sirens Echo Gateway session.
2. The runtime accepts only the configured `#bots` channel and a mention or
   reply directed at Sirens Echo.
3. The runtime rejects DMs, other channels, bot authors, self messages, and
   duplicate deliveries.
4. The runtime loads at most 12 earlier `#bots` messages and marks the
   transcript as untrusted.
5. Agent Proxy runs the externally selected route model with the repo-local
   skillpack and strict JSON response contract.
6. The runtime rejects malformed output, invented channels, and unsupported
   first-person action claims.
7. The runtime optionally creates or reuses an exact-title Forgejo issue.
8. The runtime replies with every Discord mention disabled.

Requests run one at a time. State consists only of a bounded duplicate set and
recent Discord history. Sirens Echo has no automatic memory.

## Source-controlled agent

`agent/sirens-echo.json` pins identity, seat, role, the AOS
`knowledge-retrieval` and `conversation-management` intent routes, `#bots`
boundary, history budget, and skill roots. It pins no backend model.

`.agents/skills/sirens-echo-community/` is the first local skill. Its
`SKILL.md` carries the response method and `references/` carries approved
Sirens knowledge. Future Sirens-specific skills belong here and enter the
agent definition through reviewed commits.

Agent Proxy remains transport. AOSH owns model selection. The deployment reads
the selected model from `/sirens-echo/agent-proxy-model` and passes it through
without a source default.

## Knowledge gaps and corrections

An unanswered question produces a `knowledge-gap` draft. An explicit
correction produces a `correction` draft. These values control the issue title
prefix only. The runtime applies no labels.

Before writing, the runtime removes Discord message links, mention syntax, and
long Discord-style identifiers. The draft must summarize the needed knowledge
or behavior change without member identity, handles, raw quotes, secrets, or
personal details.

The Forgejo client reuses an exact-title open issue. A new issue records the
sanitized gap, the expected skillpack change, and the regression-conversation
requirement. A reviewed commit updates the skill and closes the issue.

## Permission boundary

The Sirens Echo token is separate from the admin bot token. Her application
needs only `#bots` visibility, history, and reply permissions.

Replies use an empty `allowed_mentions.parse` list and set `replied_user` to
false. Text that resembles a mention remains inert.

The Forgejo client can list and create ordinary issues only. It has no code,
pull request, label, milestone, assignment, comment, close, or repository
administration method.

## Evaluation

`agent/evaluation.json` carries an unknown event-time case and an explicit
correction case. `ward exec test` runs offline boundary tests.

`ward exec eval-echo` sends both cases through the production Agent Proxy
prompt. It performs no Discord or Forgejo write. The gate requires the correct
issue type and rejects case-specific inventions and action claims.

Rollout and rollback live in
[sirens-echo-rollout.md](sirens-echo-rollout.md).
