# Agent instructions

Workspace conventions load globally through `agentic-os-kai/AGENTS.md`. This file holds repo-local specifics.

## Scope

This repository deploys the Agent Proxy-backed `sirens-echo` Discord Community
harness. Her composition, skill, MCP, evaluation, telemetry, and boundary live here.

## Project shape

Community code lives in `cmd/sirens-echo`, `cmd/sirens-echo-eval`, and
`internal/community`. Definitions live in `agent/`. The Community role
composes from `agent/community-compose.kdl`. Local skills live under
`.agents/skills/`. `sirens-echo-community` is Echo's compiled local knowledge.
`ops-social-discord` guides guarded read-only investigations and is not part of
her runtime skill roots. The Dockerfile packages Echo on the full AOS release
image.

## Repo boundaries

The Community harness owns its integration and Sirens-local skill. AOS and
agent-compose own the role, shared skills, seat, and personality meld. Agent
Proxy owns inference transport. MCP servers own their tool behavior.

The runtime may call its configured MCP roster and reply in `#bots`. Its Forgejo
MCP is fixed to this repository and can read issues and labels, create or close
issues, add comments, and change labels. It cannot edit or delete issue bodies
or comments, delete or reopen issues, reach another repository, or perform
moderation, DM, or account actions.

## Commands

Route every dev verb through Ward: `ward exec build`, `bundle-check`, `vet`,
`test`, `tidy`, and `run-echo`. Do not invoke bare `go` or `uv`.

## Validation

Run `ward exec vet` and `ward exec test` before committing. The full
pre-commit gate must pass. Never use `--no-verify`.

## Safety

Required secrets live in SSM. Echo's ExternalSecret maps her Discord token,
selected model, and `#bots` identifier into her pod. The private Forgejo MCP
maps its repository-scoped token only into its pod. Echo receives only the URL.

Every community event passes channel, summon, author, duplicate, response,
grounding, and mention checks. Accepted `#bots` and private HTTP turns retain
trace-correlated metadata and byte counts without member, prompt, model, tool,
or reply bodies. Rejected events and DMs never enter the turn logger. Forgejo
issues contain sanitized summaries and no labels.

For live diagnosis, an authorized ops or director session can correlate
bounded history from the deploy-owned read-only Discord MCP with Echo's
telemetry. Follow [the observability guide](docs/sirens-echo-observability.md).

## Cross-repo contracts

Infrastructure owns k3s. Coily owns game operations. Agent Proxy owns inference.
AOS and agent-compose own the Community bundle. Eco MCP owns its public tools.
`coilyco-bridge/deploy` owns Echo, the private Forgejo MCP, and their rollout.

## Release

Main pushes publish the private Echo image at a full source SHA through the
trusted deploy lane. Echo holds no cluster credential. Deploy owns rollout.

## Agent rules

Commit directly to main and push after each commit. No PRs unless asked.
She/her in all artifacts. No em dashes or semicolons in prose.

## See also

- [README.md](README.md) - human-facing intro.
- [docs/FEATURES.md](docs/FEATURES.md) - shipped inventory.
- [.ward/ward.yaml](.ward/ward.yaml) - allowlisted commands.

Cross-reference convention from [features-release-tooling.md](docs/features-release-tooling.md), tracked by [coilysiren/agentic-os#59](https://github.com/coilyco-flight-deck/agentic-os/issues/59).
