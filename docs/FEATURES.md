# Features

Sirens Discord deployment inventory.

## Discord Community harness

- Separate `sirens-echo` Discord identity, binary, and container
- AOS release substrate with no composed role, identity card, personality,
  shared behavioral context, or lore source
- Build-time verification of the repository-owned neutral model policy
- Source-controlled `#bots` boundary, history budget, local skill root, Eco
  and private Forgejo MCP roster, and evaluation cases
- Mention-or-reply invocation with channel, guild, author, and duplicate gates
- Serialized turns with bounded Discord-history continuity
- Agent Proxy loop for MCP schemas, tool calls, results, continuation, and
  compatible content shapes
- Impersonal response contract that rejects
  greetings, emojis, first-person voice, banter, sign-offs, and open-ended offers
- Public no-auth Eco MCP for current game information
- Private Forgejo MCP fixed to this repository with issue create, close, and
  comment plus issue-label changes and bounded reads
- No Forgejo token in the Echo pod, with the same guarded handler serving
  model tools and deterministic automatic issue follow-up
- Decoder-enforced JSON object responses with one bounded format-or-neutrality
  repair, strict normalization, grounding checks, and neutral-style validation
- Soft-reference replies with every Discord mention disabled
- Repo-local Sirens knowledge with no automatic memory or autonomous edits
- Repository-owned guarded Discord context skill with exact SSM identifier
  fast paths, bounded MCP reads, and live thread discovery
- Sanitized ordinary Forgejo issues with exact-title reuse and no labels
- Transport-aware OpenTelemetry ingress and joined turn traces through Agent
  Proxy, LiteLLM, and MCP calls
- Trace-correlated metadata logs with body byte counts and no member, prompt,
  model, tool, or reply content
- Turn, latency, model-call, tool-call, and failure metrics, plus 23 bounded
  SigNoZ exception groups with stage and outcome tags
- Metrics-only local liveness and non-generating Agent Proxy route readiness,
  with bounded request, outcome, duration, state, and last-success metrics
- Private HTTP test entrypoint with W3C server tracing that exercises the same
  validated turn path
- Offline harness and real MCP fixtures plus a non-mutating live gate
- Singleton k3s worker with independent rollout and scale-to-zero rollback

See [the harness](sirens-echo.md), [MCP tools](sirens-echo-tools.md),
[observability](sirens-echo-observability.md), and [rollout](sirens-echo-rollout.md).

## Configuration and deployment

- Echo Discord, channel, and model values injected from SSM
- Deployment-owned private Forgejo MCP URL injected into Echo and a
  repository-scoped Forgejo token injected only into the MCP pod
- ExternalSecret injection with no pod AWS permission
- Existing SigNoZ collector for OTLP/HTTP traces and metrics
- Singleton k3s Echo Deployment
- Private full-source-SHA Echo images published to Forgejo OCI
- Separate read-only pull credential owned by deploy

See [deploy.md](deploy.md).

## Development gates

- Ward verbs for build, neutral policy verification, format, vet, test,
  tidy, Echo execution, Echo evaluation, and full pre-commit
- Forgejo CI builds, checks policy, vets, tests, and runs pre-commit
- Structure, skills, links, Go modules, comments, and secrets validated

## Deliberate exclusions

- Echo has no moderation, DM, account, role, announcement, Forgejo body or
  comment edit, delete, reaction, cross-repository, or ambient-channel surface
- Echo owns no web or mobile UI

## See also

- [README.md](../README.md) - human-facing introduction
- [AGENTS.md](../AGENTS.md) - agent-facing rules
- [.ward/ward.yaml](../.ward/ward.yaml) - allowlisted commands

Cross-reference convention from [features-release-tooling.md](features-release-tooling.md),
tracked by [coilysiren/agentic-os#59](https://github.com/coilyco-flight-deck/agentic-os/issues/59).
