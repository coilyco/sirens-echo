# Agent instructions

Workspace conventions load globally through `agentic-os-kai/AGENTS.md`. This file holds repo-local specifics.

## Scope

This repository deploys `sirens-echo`, the Agent Proxy-backed Discord
Community harness. Her composition request, local skill, MCP roster,
evaluation, telemetry, and Discord boundary live here.

## Project shape

Community code lives in `cmd/sirens-echo`, `cmd/sirens-echo-eval`, and
`internal/community`. Definitions live in `agent/`. The Community role
composes from `agent/community-compose.kdl`. Local skills live under
`.agents/skills/`. The Dockerfile packages Echo on the full AOS release image.

## Repo boundaries

The Community harness owns its integration and Sirens-local skill. AOS and
agent-compose own the role, shared skills, seat, and personality meld. Agent
Proxy owns inference transport. MCP servers own their tool behavior.

The runtime may call its configured MCP roster, reply in `#bots`, and create
ordinary issues here for unanswered questions or corrections. It has no
moderation, DM, account, or general Forgejo authority.

## Commands

Route every dev verb through Ward: `ward exec build`, `bundle-check`, `vet`,
`test`, `tidy`, and `run-echo`. Do not invoke bare `go` or `uv`.

## Validation

Run `ward exec vet` and `ward exec test` before committing. The full
pre-commit gate must pass. Never use `--no-verify`.

## Safety

Required secrets live in SSM. The Echo ExternalSecret maps her Discord token,
Forgejo token, selected model, and `#bots` identifier into the pod.

Every community event passes channel, summon, author, duplicate, response,
grounding, and mention checks. Accepted `#bots` turns retain full content in
trace-correlated logs by design. Rejected events and DMs never enter the turn
logger. Forgejo issues contain sanitized summaries and no labels.

## Cross-repo contracts

Infrastructure owns k3s. Coily owns game operations. Agent Proxy owns
inference. AOS and agent-compose own the Community bundle. Eco MCP owns the
initial tools. `coilyco-bridge/deploy` owns Echo's manifest and rollout.

## Release

Main pushes publish the private Echo image at a full source SHA through the
trusted deploy lane. The application holds no cluster credential. Deploy owns
its read-only pull credential and rollout.

## Agent rules

Commit directly to main and push after each commit. No PRs unless asked.
She/her in all artifacts. No em dashes or semicolons in prose.

## See also

- [README.md](README.md) - human-facing intro.
- [docs/FEATURES.md](docs/FEATURES.md) - shipped inventory.
- [.ward/ward.yaml](.ward/ward.yaml) - allowlisted commands.

Cross-reference convention from [features-release-tooling.md](docs/features-release-tooling.md),
tracked by [coilysiren/agentic-os#59](https://github.com/coilyco-flight-deck/agentic-os/issues/59).
