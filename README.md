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
- The tool roster is public Eco MCP plus a private Forgejo MCP fixed to this
  repository. No Forgejo token in the Echo pod.
- Every issue a turn observed or filed gets a canonical link appended, built
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
[tuning a deployment](docs/sirens-echo-tuning.md),
[response profiles](docs/sirens-echo.md),
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

`just eval-echo` exercises the production prompt, Agent Proxy, and static
Eco MCP roster without sending Discord messages or creating issues. Set
`OTEL_EXPORTER_OTLP_ENDPOINT` to name the evaluation target you run against.

## Deployment

Main pushes publish a full-source-SHA Echo image to Forgejo OCI.
`coilyco-bridge/deploy` owns the k3s Deployment, secrets, rollout, and
rollback. See [the rollout checks](docs/sirens-echo-deploy.md).

## See also

See [AGENTS.md](AGENTS.md), [docs/FEATURES.md](docs/FEATURES.md), [justfile](justfile), [.ward/ward.yaml](.ward/ward.yaml),
[admission](docs/sirens-echo-admission.md), [access](docs/sirens-echo-access.md),
[HTTP](docs/sirens-echo-http.md), [health](docs/sirens-echo-observability.md),
[notices](docs/sirens-echo-delivery.md),
[identity](docs/sirens-echo-identity.md),
[role record](docs/sirens-echo-compose.md),
[jobs](docs/sirens-echo-jobs.md),
[job lifecycle](docs/sirens-echo-jobs.md),
[job telemetry](docs/sirens-echo-telemetry.md),
[commands](docs/sirens-echo-commands.md),
[execution](docs/sirens-echo-execution.md),
[guardfile knowledge](docs/sirens-echo-config.md),
[reply progress](docs/sirens-echo-progress.md),
[the identity eval](docs/sirens-echo-identity.md),
[counterparts](docs/sirens-echo-compose.md),
[attribution](docs/sirens-echo-worklog.md),
[grants](docs/sirens-echo-access.md), and
[docs/sirens-echo-deploy.md](docs/sirens-echo-deploy.md).
Cross-reference convention from [FEATURES.md](docs/FEATURES.md), tracked by [coilysiren/agentic-os#59](https://github.com/coilyco-flight-deck/agentic-os/issues/59).
