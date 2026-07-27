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
Skills live under `.agents/skills/`. The Dockerfile packages Echo. Native
scripts, systemd units, and sudoers belong only to the admin bot.

## Repo boundaries

The admin bot never implements game operations itself. Coily is its API and
safety net.
The community agent owns her deployed integration and Sirens skillpack.
Agent-compose owns the upstream seat and personality. Agent Proxy owns
transport. The runtime may reply in `#bots` and create ordinary issues here
for unanswered questions or corrections. It has no moderation, DM, account,
or general Forgejo authority.

## Commands

Route every dev verb through Ward, which reads `.ward/ward.yaml`: `ward exec build`, `vet`, `test`, `tidy`, `run`. Do not invoke bare `go` or `uv`.

## Validation

Run `ward exec vet` and `ward exec test` before committing. The full pre-commit gate (`pre-commit run --all-files`) must pass. Never use `--no-verify`.

## Safety

Required secrets live in SSM. Admin scripts fetch `/sirens-discord-ops/`
values in memory. The Echo k3s ExternalSecret maps
`/sirens-echo/discord-bot-token`,
`/sirens-echo/forgejo-issue-token`, `/sirens-echo/agent-proxy-model`, and
`/discord/channel/bots` into the pod.

Every admin button press checks the actor's role against `ADMIN_ROLE_ID`.
Every community message passes exact channel, summon, author, duplicate,
response, grounding, and empty-mention checks. Forgejo issues contain
sanitized summaries and no labels.

## Cross-repo contracts

Depends on `coilyco-flight-deck/infrastructure` for k3s, `coily` for admin
operations, Agent Proxy for inference, and agent-compose for the community
seat. `coilyco-bridge/deploy` owns Echo's k3s manifest and rollout. Catalog metadata lives in `.ward/ward.yaml`.

## Release

The admin bot remains native because it executes host-local Coily commands. Main
pushes publish the private Sirens Echo image as
`forgejo.coilysiren.me/coilyco-gaming/sirens-discord-ops:<full-source-sha>`
through the trusted deploy lane. The runner supplies `REGISTRY_TOKEN` for
package writes. `coilyco-bridge/deploy` owns her separate read-only
`forgejo-registry` pull credential, singleton k3s Deployment, ExternalSecret,
rollout, and rollback.

## Agent rules

Commit to main directly and push after each commit. No PRs unless asked. She/her in all artifacts. No em-dashes or semicolons in prose.

## See also

- [README.md](README.md) - human-facing intro.
- [docs/FEATURES.md](docs/FEATURES.md) - inventory of what ships today.
- [.ward/ward.yaml](.ward/ward.yaml) - allowlisted commands.

Cross-reference convention from [features-release-tooling.md](docs/features-release-tooling.md),
tracked by [coilysiren/agentic-os#59](https://github.com/coilyco-flight-deck/agentic-os/issues/59).
