# sirens-discord-ops

Source-controlled Discord deployment for the Sirens community. This repository
ships the game-server admin panel and the independent Sirens Echo Community
harness.

## Admin control panel

The admin bot pins one panel per game in `#admin-control`. A button runs
`coily gaming <game> <verb>` on kai-server and posts start and completion
output to `#admin-audit`.

Eco exposes restart, status, stop, and start. Restart, stop, and start require
confirmation. Every interaction checks `ADMIN_ROLE_ID`. Non-admins receive an
ephemeral refusal and produce no audit write.

## Sirens Echo

Sirens Echo is the automated she/her host selected from the agent-compose
`siren community host` seat and `community` role. Her image retains the full
AOS runtime substrate. Its build composes and verifies the real Community
role, personality meld, and shared skills, then adds the local Sirens skill.

- Only a mention or reply in the exact `#bots` channel invokes her.
- DMs, other channels, bots, self messages, and duplicates are ignored.
- Replies disable all mentions, including the replied member.
- Twelve earlier messages provide continuity across consecutive summons.
- The source-controlled tool roster initially exposes public Eco MCP.
- Agent Proxy can call tools, receive results, and continue to a final reply.
- Unanswered questions and corrections become sanitized ordinary Forgejo
  issues with no labels.
- One OpenTelemetry trace joins the complete turn. Trace-correlated JSON logs
  retain accepted inputs, prompts, responses, tool activity, and final replies.
- No moderation, DM, reaction, edit, delete, announcement, role, or account
  action exists.

Sirens-local knowledge lives at
`.agents/skills/sirens-echo-community/`. Sirens Echo keeps no automatic
memory. See [the harness walkthrough](docs/sirens-echo.md).

## Configuration

The admin service reads its token, admin and audit channel identifiers, and
admin role identifier from `/sirens-discord-ops/`.

Echo reads her Discord token, `#bots` identifier, issue token, and selected
Agent Proxy model from SSM-backed deployment secrets. `AGENT_PROXY_URL`,
`SIRENS_ECHO_AOS_BUNDLE`, and `OTEL_EXPORTER_OTLP_ENDPOINT` override their
runtime defaults. The pod receives no AWS permission.

## Development

Run commands through Ward:

```sh
ward exec build
ward exec bundle-check
ward exec test
ward exec vet
ward exec pre-commit-all
```

`ward exec eval-echo` exercises the production prompt, Agent Proxy, and Eco
MCP without sending Discord messages or creating issues.

## Deployment

The admin bot remains a native systemd service because it executes host-local
Coily commands. Main pushes publish a private full-source-SHA Echo image.
`coilyco-bridge/deploy` owns the independent k3s Deployment, secrets, rollout,
and rollback. See [the rollout checks](docs/sirens-echo-rollout.md).

## See also

See [AGENTS.md](AGENTS.md), [docs/FEATURES.md](docs/FEATURES.md),
[docs/deploy.md](docs/deploy.md), and [.ward/ward.yaml](.ward/ward.yaml).

Cross-reference convention from [features-release-tooling.md](docs/features-release-tooling.md),
tracked by [coilysiren/agentic-os#59](https://github.com/coilyco-flight-deck/agentic-os/issues/59).
