# sirens-echo

A discord community agent harness - home of sirens echo and sirens deep

![sirens-echo and sirens-deep, a discord community agent harness](assets/banner.jpg)

Sirens Echo is an automated community response service. Its model context is a
repository-owned neutral profile plus approved Sirens knowledge. The Discord
deployment remains Sirens Echo.

**It is silent unless summoned.** Only a mention or a reply in a configured
channel, or a thread under one, invokes the service, and everything else is
ignored, direct messages included. A bot sitting in a channel is not a bot
reading it.

## Behavior

- A git-tracked access policy stacks guild, channel, user, and role grants with
  a deny list, per-guild rate overrides, and CI validation.
- Per-user, per-context, and global rate limits over a pool of eight concurrent
  execution slots, with a bounded queue behind it and one cooldown notice per
  window rather than one per denied summon.
- An impersonal response contract rejects greetings, emotive emoji, banter,
  sign-offs, and open-ended offers, with grounding checks reading first-person
  and passive claims alike.
- The game Echo answers for is one swappable skill root. Eco and Enshrouded both
  ship, Enshrouded is the active focus, and changing games is one line in the
  definition. See the game focus section of [AGENTS.md](AGENTS.md).
- The tool roster is deploy-owned and this repository names no server in it, so
  it is a separate axis from the game focus and a swap does not change it. No
  tracker token in the Echo pod, and the harness offers the model issue verbs
  over the tracker MCP's record verbs, reading every write back before
  reporting it.
- Every issue a turn observed or filed gets its tracker key appended, built
  only from returned tool results rather than from anything the model said.
- Traces and metadata logs carry byte counts and no member or model text. A
  gateway heartbeat counts observed, admitted, and replied, so a quiet guild and
  a stopped ingress differ.

No automatic memory. Guarded investigations use
`.agents/skills/ops-social-discord/`. See [the walkthrough](docs/sirens-echo.md).

## Coilyco harness

The `sirens-deep` deployment selects `agents/deep/definition.yaml`. Its model
identity is Sirens Deep of Coilyco and its scope is general-purpose.

- It loads only the domain-neutral `coilyco-general` policy and names no
  channel, so deployment selects Discord ingress, HTTP ingress, or both.
- It starts with no MCP, repository knowledge, issue tracker, or write surface
  while retaining the shared safety and transport bounds.
- One process serves several channels across several guilds, plus opt-in direct
  messages. See [multiple Discord contexts](docs/sirens-echo-threads.md).

General-purpose means topic-neutral and extensible, not universally authorized.
Future tools or knowledge must be added explicitly to the tracked definition
with their own permission boundary.

## Configuration

Every number has one home, `internal/community/config.go`, and every one of
them takes an environment override through the same helper. There is no tier of
numbers a deployment cannot reach. The list is generated into
[the reference](docs/sirens-echo-tuning.md) from the table itself, and a test
fails if a number drifts out of the file or the reference falls behind it.

Deploy selects the tracked YAML definition, Agent Proxy route, Discord switch,
channel and guild scope, admission limits, and instance name. Reachability of
`POST /v1/turn` is decided at the network layer by the deployment. See
[tuning](docs/sirens-echo-tuning.md),
[admission control](docs/sirens-echo-admission.md), and
[deployment](docs/sirens-echo-deploy.md).

## Development

Run commands through just:

```sh
just setup
just build
just policy-check
just test
just vet
just pre-commit-all
```

`just setup` installs the pre-commit hooks. Run it once per clone.

`just eval-echo` exercises the production prompt, Agent Proxy, and a static
MCP roster without sending Discord messages or creating issues. Set
`OTEL_EXPORTER_OTLP_ENDPOINT` to name the evaluation target you run against.

## Deployment

Main pushes publish a full-source-SHA Echo image to Forgejo OCI.
`coilyco-bridge/deploy` owns the k3s Deployment, secrets, rollout, and
rollback. See [the rollout checks](docs/sirens-echo-deploy.md).

## See also

Start here:

- [the walkthrough](docs/sirens-echo.md) - what a turn does, end to end.
- [AGENTS.md](AGENTS.md) and [docs/FEATURES.md](docs/FEATURES.md) - operating rules, and what ships today.

What bounds it:

- [untrusted input](docs/sirens-echo-untrusted-input.md) - links out, files in, and the values a reply may never echo.
- [the content gate](docs/sirens-echo-content-gate.md) - the taxonomy that makes a `deny: true` mean something at runtime.
- [grounding](docs/sirens-echo-grounding.md) - the four rules a filing claim has to clear.
- [access](docs/sirens-echo-access.md) and [admission](docs/sirens-echo-admission.md) - who may invoke it, and what bounds the spend.

How it runs:

- [deployment](docs/sirens-echo-deploy.md) and [tuning](docs/sirens-echo-tuning.md) - the rollout checks, and every number a deployment can reach.
- [HTTP](docs/sirens-echo-http.md) and [observability](docs/sirens-echo-observability.md) - the turn endpoint, and health.
- [threads](docs/sirens-echo-threads.md) - serving several channels and guilds from one process.
- [identity](docs/sirens-echo-identity.md) and [compose](docs/sirens-echo-compose.md) - who it says it is, and the role record behind that.
- [justfile](justfile) - dev verbs, and catalog metadata.

Every doc is under [`docs/`](docs/), one page per surface.
