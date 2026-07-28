# Features

Living inventory of the Sirens Discord deployment.

## Discord Community harness

- Separate `sirens-echo` Discord identity, binary, and container
- Full AOS release substrate with no lore source
- Build-time verified compiled AOS Community role, shared skills, canonical
  seat, and nurturing, diplomatic, playful personality meld
- Source-controlled `#bots` boundary, history budget, local skill root, Eco
  MCP roster, and evaluation cases
- Mention-or-reply invocation with channel, guild, author, and duplicate gates
- Serialized turns with bounded Discord-history continuity
- Agent Proxy loop for MCP schemas, tool calls, results, continuation, and
  compatible content shapes
- Public no-auth Eco MCP as the first tool server
- Decoder-enforced JSON object responses with one bounded repair, strict
  normalization, and grounding checks
- Soft-reference replies with every Discord mention disabled
- Repo-local Sirens knowledge with no automatic memory or autonomous edits
- Sanitized ordinary Forgejo issues with exact-title reuse and no labels
- Joined OpenTelemetry turn traces and complete trace-correlated content logs
- Turn, latency, model-call, tool-call, and failure metrics
- Offline harness and real MCP fixtures plus a non-mutating live gate
- Singleton k3s worker with independent rollout and scale-to-zero rollback

See [the harness](sirens-echo.md), [MCP tools](sirens-echo-tools.md),
[observability](sirens-echo-observability.md), and [rollout](sirens-echo-rollout.md).

## Configuration and deployment

- Echo Discord, channel, Forgejo, and model values injected from SSM
- ExternalSecret injection with no pod AWS permission
- Existing SigNoZ collector for OTLP/HTTP traces and metrics
- Singleton k3s Echo Deployment
- Private full-source-SHA Echo images published to Forgejo OCI
- Separate read-only pull credential owned by deploy

See [deploy.md](deploy.md).

## Development gates

- Ward verbs for build, Community bundle verification, format, vet, test,
  tidy, Echo execution, Echo evaluation, and full pre-commit
- Forgejo CI composes AOS, builds, vets, tests, and runs pre-commit
- Structure, skills, links, Go modules, comments, and secrets validated

## Deliberate exclusions

- Echo has no moderation, DM, account, role, announcement, edit, delete,
  reaction, or ambient-channel surface
- Echo owns no web or mobile UI

## See also

- [README.md](../README.md) - human-facing introduction
- [AGENTS.md](../AGENTS.md) - agent-facing rules
- [.ward/ward.yaml](../.ward/ward.yaml) - allowlisted commands

Cross-reference convention from [features-release-tooling.md](features-release-tooling.md),
tracked by [coilysiren/agentic-os#59](https://github.com/coilyco-flight-deck/agentic-os/issues/59).
