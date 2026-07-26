# sirens-discord-ops

Source-controlled Discord deployment for the Sirens community. This repository
ships the game-server admin panel and the independently managed Sirens Echo
community agent.

## Admin control panel

The admin bot pins one panel per game in `#admin-control`. A button runs
`coily gaming <game> <verb>` on kai-server and posts start and completion
output to `#admin-audit`.

Eco currently exposes restart, status, stop, and start. Restart, stop, and
start require confirmation. Every interaction checks `ADMIN_ROLE_ID`.
Non-admins receive an ephemeral refusal and produce no audit write.

## Sirens Echo

Sirens Echo is the automated she/her community host selected from the
agent-compose `siren community host` seat and `community` role.

Her deployed definition, model-opaque AOS intent routes, skillpack, and
evaluation cases are versioned here. Agent Proxy supplies inference transport.

- Only an exact `#bots` channel match can invoke her.
- A member must mention her or reply to one of her messages.
- DMs, other channels, bots, self messages, and duplicates are ignored.
- Replies disable every mention, including the replied member.
- The model receives at most 12 earlier messages from `#bots`.
- Unanswered questions and explicit corrections become sanitized ordinary
  Forgejo issues in this repository with no labels.
- No moderation, DMs, reactions, edits, deletes, announcements, role changes,
  or account actions exist.

The initial skillpack lives at `.agents/skills/sirens-echo-community/`.
Reviewed corrections update the skillpack and a regression conversation.
Sirens Echo keeps no automatic memory. See [docs/sirens-echo.md](docs/sirens-echo.md).

## Configuration

The admin service reads its token, admin and audit channel identifiers, and
admin role identifier from `/sirens-discord-ops/`.

The Echo service reads `/sirens-echo/discord-bot-token`,
`/discord/channel/bots`, `/sirens-echo/forgejo-issue-token`, and the
selection-owned `/sirens-echo/agent-proxy-model`.

The Forgejo token should be limited to ordinary issue reads and creates in
this repository. `AGENT_PROXY_URL` can override `http://ser8:8080`. Start
scripts keep resolved values in memory and write no env file.

## Development

Run commands through Ward:

```sh
ward exec build
ward exec test
ward exec vet
ward exec pre-commit-all
```

`ward exec eval-echo` exercises the production prompt and Agent Proxy path
without sending Discord messages or creating issues.

## Deployment

Both bots run as native systemd units on kai-server. The base installer keeps
the admin bot enabled and installs Echo disabled. An operator runs
`scripts/install-echo.sh` after [the rollout checks](docs/sirens-echo-rollout.md).

The update timer restarts Echo only when her unit is enabled.
`scripts/disable-echo.sh` is the independent Echo rollback.

## See also

See [AGENTS.md](AGENTS.md), [docs/FEATURES.md](docs/FEATURES.md),
[docs/deploy.md](docs/deploy.md), and [.ward/ward.yaml](.ward/ward.yaml).

Cross-reference convention from [coilysiren/agentic-os#59](https://github.com/coilyco-flight-deck/agentic-os/issues/59).
