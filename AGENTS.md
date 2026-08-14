---
ward:
  workflow: pull-request-and-merge
---
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

Route every dev verb through Ward: `ward exec setup`, `build`, `policy-check`,
`vet`, `test`, `tidy`, and `run-echo`. Do not invoke bare `go` or `uv`.

## Validation

Run `ward exec gate` before pushing. It runs build, policy-check, vet, test,
test-skips, and pre-commit last, which is what CI runs and the order it runs
them in. See [the gate](docs/sirens-echo-gate.md).

`vet` and `test` alone pass on a tree CI rejects, so the separate verbs still
leave a coverage gap that only `gate` closes. Never use `--no-verify`.

A fresh clone starts with no hooks. Run `ward exec setup` to install them.
Forgetting is survivable, because every verb routed through
`scripts/ward-command.sh` installs a missing hook on the way past, and the
daily loop of `vet`, `test`, and `tidy` all route through it. The snapshot,
regeneration, and image verbs name their tool directly and install nothing.

### Live evaluation cadence

`ward exec eval-deep` runs 5 times. `ward exec eval-echo` runs once.

Deep's pack is a deterministic battery of scoped checks. Never add a check that
could fire on a correct reply. See [the battery](docs/sirens-echo-battery.md).

`ward exec board-deep` is the human-graded board and gates nothing. It repeats
each case 5 times inside one run, so it is invoked once and its stdout is the
evidence. Never wire it into CI and never derive a pass or fail from its
`structural` field. See [the Deep board](docs/sirens-echo-board.md).

`ward exec rate-deep` measures an intermittent behavior and gates nothing. It
runs each case its own declared number of times, reports passed over attempts,
and excludes substrate errors from the denominator. Never wire it into CI, and
never promote a case into the battery on a small clean sample. Record host
state in `SIRENS_ECHO_SUBSTRATE` before running. See [the rate
pack](docs/sirens-echo-rate.md).

Three instruments, sorted by what a case protects rather than by how reliably
it passes. Security cases gate. Everything else reports.

The split is load on `kai-tower-3026`, not confidence. Echo's route
`sirens-echo/default` resolves to `ornith:35b` on ollama and the AOSH router
puts `default_server` on that tower, so every Echo case pins a 35B model on the
daily driver and a run takes minutes. Deep's route `sirens-echo/deepseek`
carries `direct: null` and resolves upstream to `deepseek-v4-flash`, so it never
touches the tower and repeats are cheap.

Both need `AGENT_PROXY_URL`, `AGENT_PROXY_MODEL` naming the profile's route, and
`OTEL_EXPORTER_OTLP_ENDPOINT` pointed at the deployment's receiver. Supply
`SIRENS_ECHO_MCP_ROSTER` when a case requires a tool; without one the roster is
empty and a tool case fails for a reason that is not the agent's.

Unit tests never leave the machine, so the cadence does not apply to them.

## Safety

Required secrets live in SSM. Echo's ExternalSecret maps the Discord token,
selected model, and `#bots` identifier into the pod. The private Forgejo MCP
maps its repository-scoped token only into its pod. Echo receives only the URL.

Every community event passes channel, summon, author, duplicate, response,
grounding, and mention checks. Accepted `#bots` and private HTTP turns retain
trace-correlated metadata and byte counts without prompt, model, tool, or reply
bodies. A Discord turn span additionally carries the author's account id and
the guild, channel, thread, and message ids, so a trace id a member was handed
can be placed. Nothing member-visible goes with them, and no direct message
contributes any of it. See [turn
identifiers](docs/sirens-echo-turn-identifiers.md). Rejected events and DMs
never enter the turn logger. Forgejo issues contain sanitized summaries and
exactly one label, the sandbox marker the harness injects before dispatch so a
member-influenced filing is never unmarked. See [knowledge gaps and
corrections](docs/sirens-echo-issues.md).

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

**Git workflow** - `pull-request-and-merge`, declared as `ward.workflow` in this file's frontmatter. Agents push a branch and open a Forgejo pull request. Nothing lands straight on `main`, and the merge stays director-gated. Byte-identical across the five PR-lane repos (agentic-os, deploy, infrastructure, sirens-echo, ward) per agentic-os#994. Ward honors it only after ward#1661.

**A pull request body must carry a closing reference**, or the merge verb will
not merge it. `closes #N` and `closes owner/repo#N` are the accepted spellings,
and `fixes` and `resolves` work the same way. A full issue URL does **not**
satisfy it, which matters here because a URL is the house convention everywhere
else. See [the merge lane](docs/sirens-echo-merge-lane.md).

If a pull request does not fully close the issue that motivated it, file the
slice as its own issue and close that one. Do not weaken the reference to
satisfy the verb.

**The `consult` label is a dispatch gate, not decoration.** `cli-guard` reads it
to route work and an unlabelled issue fails closed to `consult`, so it is also
the queue a human reads to find what needs them. Two habits keep it true, and
both attach to a comment you are already writing:

- Recording a decision on an issue **removes** `consult` in the same call.
  An answered question that still advertises itself costs hours, because the
  board says blocked and the thread says done.
- Asking a human a question **adds** it in the same call. Unlabelled is
  correctly fail-closed for dispatch and invisible to the human, which is the
  worst pair.

See [the consult gate](docs/sirens-echo-consult-gate.md).

**A claim reserves an issue number, and the collision surface is files.** Two
issues can point at one function, so claiming correctly does not stop two agents
building the same change. Four habits, each attached to something you are
already doing:

- **List the open pull requests before you start.** `aosguard ops forgejo pr
  list <owner> <repo> --state open`. An unclaimed issue with an open pull
  request against it is the common shape, because the author of a small change
  often opens one before commenting. This is the cheapest of the four.
- **Fetch `origin/main` immediately before the first edit**, not only when
  branching. This repository merges several times an hour, and a claim is
  minutes old by the time work starts.
- **Look for the thing before building it.** Grep for the function, the file,
  the flag. A capability that already exists is the cheapest duplicate to avoid.
- **When you are beaten to it, compare before discarding.** The landed version
  is often better, and where it is not, the delta is a small pull request rather
  than an argument. Do not re-open a settled question to keep your version.

See [duplicate work](docs/sirens-echo-duplicate-work.md).

**Verify a write landed when you discard its output.** `aosguard ops forgejo
issue-comment create` does not exist, and run with `>/dev/null` it lost an
entire session of claims and findings without a symptom. One read after the
write is the whole habit.

The same operation fails in both directions: `issue comment` on a **closed**
issue exits non-zero by design and the comment *is* still posted, so an exit
status read without the note says a successful write failed. See
[sirens-echo#693](https://forgejo.coilysiren.me/coilyco-gaming/sirens-echo/issues/693).

**Name labels, never number them.** `--labels` on `issue-label add` and `set`
is `[]string`, so an ID goes over as `"332"` and Forgejo reads a quoted numeral
as a label name. `add` then applies nothing. **`set` is a PUT, so it replaces
the label set with nothing and silently strips every label the issue had.**
Both exit zero. Use `--labels consult`.

`issue create --labels 332` is a different flag of a different type and does
work, which is why the numeric form is easy to trust. `issue-label remove`
takes the identifier in the path and accepts either spelling. See
[agentic-os#1047](https://forgejo.coilysiren.me/coilyco-flight-deck/agentic-os/issues/1047).

Use neutral service nouns for Echo. She/her remains Kai's pronoun. No em dashes
or semicolons in prose.

## Checkout residency

This repo is not in Agent Compose's `repository-plan.yaml`, so it has no
resident checkout under `~/projects/<owner>/`. That is intentional. Work it
from a task-scoped temporary clone, and remove that clone once the work lands.

A temporary root can be purged at any time, so commit and push before pausing,
switching tasks, or ending a session. The remote is the only durable artifact.

## See also

- [README.md](README.md) - human-facing intro.
- [docs/FEATURES.md](docs/FEATURES.md) - shipped inventory.
- [.ward/ward.yaml](.ward/ward.yaml) - allowlisted commands.
Cross-reference convention from [features-release-tooling.md](docs/features-release-tooling.md), tracked by [coilysiren/agentic-os#59](https://github.com/coilyco-flight-deck/agentic-os/issues/59).
