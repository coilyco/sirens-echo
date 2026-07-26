# Agent instructions

Workspace-level conventions (git workflow, voice, ops boundary) load globally via `~/.claude/CLAUDE.md` -> `agentic-os-kai/AGENTS.md`. This file holds the repo-local specifics.

## Scope

Discord-native integration for the Sirens community. The repository deploys
two independent bot identities:

- `sirens-discord-ops` is a thin admin-button passthrough to
  `coily gaming <game> <verb>` with audit logging.
- `sirens-echo` is the Agent Proxy-backed community host. Her agent
  definition, skillpack, evaluation cases, and Discord boundary live here.

## Project shape

Go module. Admin code lives in `cmd/sirens-discord-ops` and `internal/bot`.
Community code lives in `cmd/sirens-echo`, `cmd/sirens-echo-eval`, and
`internal/community`. Agent and evaluation definitions live in `agent/`.
Skills live under `.agents/skills/`. Deploy assets live in `scripts/`,
`systemd/`, and `sudoers/`.

## Repo boundaries

The admin bot never implements game operations itself. Coily is its API and
safety net.
The community agent owns her deployed integration and Sirens skillpack.
Agent-compose owns the upstream seat and personality. Agent Proxy owns
transport. The runtime may reply in `#bots` and create ordinary issues here
for unanswered questions or corrections. It has no moderation, DM, account,
or general Forgejo authority.

## Commands

Route every dev verb through ward, which reads `.ward/ward.yaml`: `ward exec build`, `vet`, `test`, `tidy`, `run`. Do not invoke bare `make`, `go`, or `uv`.

## Validation

Run `ward exec vet` and `ward exec test` before committing. The full pre-commit gate (`pre-commit run --all-files`) must pass. Never use `--no-verify`.

## Safety

Required secrets live in SSM. Admin secrets stay under
`/sirens-discord-ops/`. Sirens Echo uses
`/sirens-echo/discord-bot-token`,
`/sirens-echo/forgejo-issue-token`, and `/discord/channel/bots`. No env files
live on disk.

Every admin button press checks the actor's role against `ADMIN_ROLE_ID`.
Every community message passes exact channel, summon, author, duplicate,
response, grounding, and empty-mention checks. Forgejo issues contain
sanitized summaries and no labels.

## Cross-repo contracts

Depends on `coilyco-flight-deck/infrastructure` for kai-server, on the
`coily` binary for admin operations, on Agent Proxy for community inference,
and on agent-compose for the canonical community seat. Catalog metadata lives
in `.ward/ward.yaml`.

## Release

Production uses independent native systemd units on kai-server, both running
as `kai`. `sirens-discord-ops.service` remains the admin control panel.
`sirens-echo.service` is an explicit opt-in through
`scripts/install-echo.sh` and can roll back through
`scripts/disable-echo.sh`. Both start paths fetch main, rebuild, and pull SSM
values at exec time.

## Agent rules

Commit to main directly and push after each commit. No PRs unless asked. She/her in all artifacts. No em-dashes or semicolons in prose.

## See also

- [README.md](README.md) - human-facing intro.
- [docs/FEATURES.md](docs/FEATURES.md) - inventory of what ships today.
- [.ward/ward.yaml](.ward/ward.yaml) - allowlisted commands.

Cross-reference convention from [coilysiren/agentic-os#59](https://github.com/coilyco-flight-deck/agentic-os/issues/59).
