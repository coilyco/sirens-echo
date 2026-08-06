# sirens-echo

Agent Proxy-backed runtime for Sirens Echo and the general-purpose CoilyCo HTTP harness.

## Sirens Echo

Sirens Echo is an automated community response service. Its model context is a
repository-owned neutral profile plus approved Sirens knowledge. The Discord
deployment remains Sirens Echo.

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
- Traces and metadata-only logs cover transport, model, and MCP work.
- The tool and action surface remains repository-scoped and excludes
  moderation, direct messages, announcements, roles, and account actions.

Sirens Echo keeps no automatic memory. Guarded investigations use
`.agents/skills/ops-social-discord/`. See
[the service walkthrough](docs/sirens-echo.md).

## CoilyCo harness

The `sirens-deep` HTTP deployment selects `agent/sirens-deep.json`. Its
CoilyCo model identity is general-purpose rather than an Eco or Sirens harness.

- It is HTTP-only and loads only the domain-neutral `coilyco-general` policy.
- It starts with no MCP, repository-specific knowledge, issue tracker, or
  default write surface while retaining the shared safety and transport bounds.

This is the initial CoilyCo harness definition. General-purpose means
topic-neutral and extensible, not universally authorized. Future tools or
knowledge must be added explicitly to the tracked definition with their own
permission boundary.

## Configuration

Deploy selects the tracked definition, Agent Proxy route, Discord switch, and
instance name. Discord secrets are required only when Discord is enabled.
Sirens Echo also receives the private MCP URL but no Forgejo credential or AWS
permission. See [response profiles](docs/response-profiles.md) and
[deployment](docs/deploy.md).

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
