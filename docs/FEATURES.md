# Features

## Discord response service

- AOS release substrate with no composed role, identity card, shared behavioral
  context, or lore source
- Neutral Sirens Echo community and social CoilyCo general-purpose profiles
  with independent local policy roots and build-time verification
- Definition-selected Discord channel, history budget, local skill roots,
  optional MCP roster, and optional automatic issue tracker
- Mention-or-reply invocation with channel, thread, guild, author, and
  duplicate gates
- Serialized turns with bounded Discord-history continuity
- Agent Proxy loop for MCP schemas, tool calls, results, continuation, and
  compatible content shapes
- Impersonal response contract that rejects
  greetings, emojis, first-person voice, banter, sign-offs, and open-ended offers
- Public Eco MCP for current game information
- Private Forgejo MCP fixed to this repository with issue create, close, and
  comment plus issue-label changes and bounded reads
- No Forgejo token in the Echo pod, with the same guarded handler serving
  model tools and deterministic automatic issue follow-up
- Decoder-enforced JSON object responses with one style-aware repair, strict
  normalization, grounding checks, and neutral-style validation where selected
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
- HTTP-only CoilyCo profile with no assumed domain, MCP, automatic issue
  tracking, or default write surface
- Offline harness and real MCP fixtures plus a non-mutating live gate

See [the service](sirens-echo.md), [response profiles](response-profiles.md),
[MCP tools](sirens-echo-tools.md), and [rollout](sirens-echo-rollout.md).

## Configuration and deployment

- Deploy-selected definition, ingress switch, instance, and Agent Proxy route
- Independent community and general-purpose definitions in one immutable image
- Deployment-owned private Forgejo MCP URL injected into Echo and a
  repository-scoped Forgejo token injected only into the MCP pod
- ExternalSecret injection with no pod AWS permission
- Existing SigNoZ collector for OTLP/HTTP traces and metrics
- Singleton k3s Echo Deployment
- Private full-source-SHA Echo images published to Forgejo OCI

See [deploy.md](deploy.md).

## Development gates

- Ward verbs for build, response-policy verification, format, vet, test,
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

Cross-reference convention from [features-release-tooling.md](features-release-tooling.md), tracked by [coilysiren/agentic-os#59](https://github.com/coilyco-flight-deck/agentic-os/issues/59).
