# Agent instructions

Workspace-level conventions (git workflow, voice, ops boundary) load globally via `~/.claude/CLAUDE.md` -> `agentic-os-kai/AGENTS.md`. This file holds the repo-local specifics.

## Scope

Discord-native admin control panel for the Sirens game servers. The bot is a thin button passthrough to `coily gaming <game> <verb>` with audit logging. v1 ships eco only.

## Project shape

Go module. Entry point `cmd/sirens-discord-ops`, logic under `internal/bot` (bot wiring, coily runner, config, game registry). Deploy assets in `scripts/`, `systemd/`, `sudoers/`. Docs in `docs/`.

## Repo boundaries

The bot never implements game operations itself. Coily is the API and the safety net. No mod install, backups, log tailing, or web UI. Out-of-scope work belongs in coily, not here.

## Commands

Route every dev verb through ward, which reads `.ward/ward.yaml`: `ward exec build`, `vet`, `test`, `tidy`, `run`. Do not invoke bare `make`, `go`, or `uv`.

## Validation

Run `ward exec vet` and `ward exec test` before committing. The full pre-commit gate (`pre-commit run --all-files`) must pass. Never use `--no-verify`.

## Safety

Required secrets live in SSM under `/sirens-discord-ops/`. No env files on disk. Every button press checks the actor's role against `ADMIN_ROLE_ID`. Non-admins get an ephemeral refusal and no audit write.

## Cross-repo contracts

Depends on `coilysiren/infrastructure` for kai-server and on the `coily` binary at runtime. Catalog metadata lives in `.ward/ward.yaml`.

## Release

Production is a native systemd unit on kai-server running as `kai`. Deploy is manual: `git push`, then `ssh kai@kai-server sudo systemctl restart sirens-discord-ops`. `scripts/start.sh` fetches main, rebuilds, and pulls SSM env at exec time.

## Agent rules

Commit to main directly and push after each commit. No PRs unless asked. She/her in all artifacts. No em-dashes or semicolons in prose.

## See also

- [README.md](README.md) - human-facing intro.
- [docs/FEATURES.md](docs/FEATURES.md) - inventory of what ships today.
- [.ward/ward.yaml](.ward/ward.yaml) - allowlisted commands.

Cross-reference convention from [coilysiren/agentic-os#59](https://github.com/coilyco-flight-deck/agentic-os/issues/59).
