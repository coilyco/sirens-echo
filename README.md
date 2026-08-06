# sirens-echo

Automated Discord response service for the Sirens community.

## Sirens Echo

Sirens Echo is an automated community response service. Its model context is a
repository-owned response profile plus approved Sirens knowledge. Deployments
select a neutral or social profile and a service-owned Agent Proxy alias without
loading Agent Compose behavioral context.

- Only a mention or reply in the exact `#bots` channel invokes the service.
- DMs, other channels, bots, self messages, and duplicates are ignored.
- Replies disable all mentions, including the replied member.
- Twelve earlier messages provide continuity across consecutive summons.
- Responses use direct, factual, impersonal language. A deterministic validator
  rejects greetings, emojis, first-person voice, banter, sign-offs, and offers.
- The source-controlled tool roster exposes public Eco MCP and Echo's private,
  repository-fixed Forgejo MCP.
- Unanswered questions and corrections become sanitized ordinary Forgejo
  issues with no labels.
- One OpenTelemetry trace joins transport ingress, the complete turn, Agent
  Proxy, LiteLLM, and MCP calls. Trace-correlated JSON logs retain metadata and
  byte counts without member, prompt, model, tool, or reply bodies.
- Forgejo tools can read issues and labels, create or close issues, add
  comments, and change labels only in this repository. No issue or comment
  edit or delete tool exists.
- No moderation, DM, reaction, announcement, role, or account action exists.
- A tailnet-only HTTP test entrypoint can exercise the same Agent Proxy turn
  path without sending a Discord message.

Shared knowledge and response policies live in separate repo-local skill roots
so deploy can select style without forking facts. Sirens Echo keeps no automatic
memory. Guarded operator and agent investigations use
`.agents/skills/ops-social-discord/`, which resolves exact SSM identifiers
before falling back to Discord discovery. See
[the service walkthrough](docs/sirens-echo.md).

## Configuration

Deploy selects the tracked definition, Agent Proxy route, Discord switch, and
instance name. Discord secrets are required only when Discord is enabled.
Deployment also supplies the private MCP URL. Echo receives no Forgejo
credential or AWS permission. See [response profiles](docs/response-profiles.md)
and [deployment](docs/deploy.md).

## Development

Run commands through Ward:

```sh
ward exec build
ward exec policy-check
ward exec test
ward exec vet
ward exec pre-commit-all
```

`ward exec eval-echo` exercises the production prompt, Agent Proxy, and static
Eco MCP roster without sending Discord messages or creating issues. It sends
telemetry to the host-reachable ser8 receiver by default. Set
`OTEL_EXPORTER_OTLP_ENDPOINT` to override that evaluation target.

## Deployment

Main pushes publish a private full-source-SHA Echo image.
`coilyco-bridge/deploy` owns the k3s Deployment, secrets, rollout, and
rollback. See [the rollout checks](docs/sirens-echo-rollout.md).

## See also

See [AGENTS.md](AGENTS.md), [docs/FEATURES.md](docs/FEATURES.md),
[health](docs/sirens-echo-health.md),
[harness design fundamentals](docs/harness-design-fundamentals.md),
[docs/deploy.md](docs/deploy.md), and [.ward/ward.yaml](.ward/ward.yaml).
Cross-reference convention from [features-release-tooling.md](docs/features-release-tooling.md), tracked by [coilysiren/agentic-os#59](https://github.com/coilyco-flight-deck/agentic-os/issues/59).
