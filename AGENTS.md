# Agent instructions

Workspace conventions load globally through `agentic-os-kai/AGENTS.md`. This file holds repo-local specifics.

## Scope

This repository builds the Agent Proxy-backed `sirens-echo` runtime for neutral Sirens Echo Discord and general-purpose CoilyCo HTTP profiles.
Their policy, MCP, evaluation, telemetry, and boundaries live here.

## Project shape

Community code lives in `cmd/sirens-echo`, `cmd/sirens-echo-eval`, and
`internal/community`. Definitions and evaluation cases live in `agent/`.
Local skills live under `.agents/skills/`. `sirens-echo-community` and
`sirens-echo-knowledge` are Echo's compiled local policy and knowledge.
`coilyco-general` is the domain-neutral HTTP profile.
`ops-social-discord` guides guarded read-only investigations and is not part of
the runtime skill roots. The Dockerfile packages Echo on the full AOS release
image.

## Repo boundaries

This service owns its model policies, integration, and local skill roots. Its
deployment-owned Agent Proxy alias selects inference tuning only and does not
add model instructions. Agent Proxy owns inference transport. MCP servers own
their tool behavior.

Each definition explicitly selects its channel, MCP roster, and optional issue
tracker. Sirens Echo may call its Eco and repository-fixed Forgejo MCPs and
reply in `#bots`. The CoilyCo profile selects a Steam reader and that same
Forgejo MCP, names no channel, and has no automatic issue tracker.

## Commands

Route every dev verb through Ward: `ward exec build`, `policy-check`, `vet`,
`test`, `tidy`, and `run-echo`. Do not invoke bare `go` or `uv`.

## Validation

Run `ward exec vet` and `ward exec test` before committing. The full
pre-commit gate must pass. Never use `--no-verify`.

## Safety

Required secrets live in SSM. Echo's ExternalSecret maps the Discord token,
selected model, and `#bots` identifier into the pod. The private Forgejo MCP
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

Infrastructure owns k3s. Agent Proxy owns inference. This repository owns model
policy. Each MCP server owns its tools. Deploy owns rollout.

## Release

Main pushes publish the Echo image at a full source SHA through the
trusted deploy lane. The workload holds no cluster credential. Deploy owns rollout.

## Agent rules

Commit directly to main and push after each commit. No PRs unless asked.
Use neutral service nouns for Echo. She/her remains Kai's pronoun. No em dashes
or semicolons in prose.

## See also

- [README.md](README.md) - human-facing intro.
- [docs/FEATURES.md](docs/FEATURES.md) - shipped inventory.
- [.ward/ward.yaml](.ward/ward.yaml) - allowlisted commands.
Cross-reference convention from [features-release-tooling.md](docs/features-release-tooling.md), tracked by [coilysiren/agentic-os#59](https://github.com/coilyco-flight-deck/agentic-os/issues/59).
