# Features

What ships today, and where each capability is documented.

## Admission

- Mention, reply, own-thread, and edit invocation with channel, thread, guild, author, and duplicate
  gates, the edit gated on a member edit and the thread on cached state, not a lookup.
- Git-tracked access policy stacking guild, channel, user, and role grants with a deny list, per-guild
  rate overrides, and CI validation.
- Per-user, per-context, and global admission control over a pool of eight concurrent execution
  slots with a bounded queue behind it, one cooldown notice per window, and bounded lookups.
- Coalescing lane folding a member's rapid comments into one turn behind **an acknowledgment per
  comment, one writer per member, selectable on the Discord summon path and off by default**.

## The response service

- Deploy-selected verified role bundle per lane, from the Core Roster plus the community person package.
  Neutral and social profiles with independent policy roots, a Kai-only trust boundary, build-time checks.
- Definition-selected history budget, skill roots, MCP roster, and issue tracker, the roots carrying
  **one swappable `sirens-game-*` focus, so a swap is one line**. Serialized turns
  bounded Discord-history continuity, and whole-thread prefill inside a thread **dropping oldest first
  with the loss stated in the reply**.
- Agent Proxy loop for MCP schemas, tool calls, results, and continuation over a deploy-owned roster
  **this repository names no server in**. **No tracker token in the Echo pod.** A harness adapter offers
  issue verbs over the tracker's record verbs and **reads every write back**. Arithmetic in process.
- Impersonal response contract rejecting greetings, emotive emoji, banter, sign-offs, and open-ended
  offers. Plain-text replies with one style-aware repair, grounding checks reading first-person and
  passive claims alike **over prose with links masked out**, and neutral-style validation where
  selected.
- Appended tracker keys for every issue a turn observed or filed, built only from returned tool
  results. Reply refusal for any identifier the process holds, derived from configuration at boot,
  admitted by shape and matched by value rather than spelling.
- Caller-supplied history marked as asserted rather than observed, on both private ingresses. Harness
  reactions for acceptance, tool rounds, failure, and refusal, plus a keyed reaction as whole answer,
  social ones Jev-snapped pre-model, text where a receipt is due or no mark fits. Flagged: a
  0.9 Jev tool pick answers from its `_meta` template.
- A worklog element on a long Discord turn, one row per tool call resolving in place, degrading to
  stacked notice lines where the embed permission is absent.
- Oversized tool results saved to the requester's scratchpad instead of being truncated away, and
  replies over the send budget attached whole as a file, **with every failure falling back to
  truncation**. One assembly step for every service-authored suffix, **shortening the answer so no
  suffix is budgeted against another**.
- Uploaded text stored in the requester's scratchpad, **from a Discord CDN address only and decided by
  its bytes**. Soft-reference replies with every Discord mention disabled, and an undelivered reply
  reported to the member once and never retried.
- Private HTTP entrypoint over the same turn path, served as JSON and as an MCP tool, with W3C tracing
  and Discord's admission policy. Transport-neutral coilyco profile with **no assumed domain, MCP,
  automatic issue tracking, or default write surface**.
- `/poll`, a free-form native Discord poll, gated like any other summon, **the member initiating rather
  than Echo**.

## Observability

- Transport-aware ingress and joined turn traces end to end, with trace-correlated metadata logs
  carrying byte counts **and no member or model text**. Logs exported over OTLP beside traces and
  metrics sharing one `service.name`, **and kept on stdout so `kubectl logs` survives a SigNoz outage**.
- Turn, latency, model-call, tool-call, admission, and failure metrics, plus a build-time closed
  exception catalog tagged by stage, outcome, and fault, **with caller and service faults split per code
  so a new one cannot be silently unclassified**. Crashes also go to Sentry.
- A Discord turn span carrying the author, guild, channel, thread, and message ids, **and no direct
  message contributing any of them**. A gateway heartbeat counting observed, admitted, and replied, **so
  a quiet guild and a stopped ingress differ**.
- Metrics-only liveness and non-generating route readiness. Workspace command execution and attachment
  ingest recorded by verb and outcome, **with no arguments, output, filename, or content**.

## Configuration and deployment

- Deploy-selected definition, ingress switch, instance, and Agent Proxy route, with independent
  community and general-purpose definitions **in one immutable image**.
- Deployment-owned tracker MCP URL in Echo, base-scoped token only in the MCP pod, and ExternalSecret
  injection **with no pod AWS permission**.
- Deploy-selected job store: in-memory, a mounted directory, or Postgres. One worker Deployment, with an
  optional **replicated gateway intake feeding it via a Postgres queue**, so a rollout drops no message.
  Full-SHA images on Forgejo OCI. **A main push publishing no image fails**, and hourly `image-coverage`
  checks main's tip.

## Development gates

- `just` recipes for build, policy, prompt snapshots, format, vet, test, tidy, run, evals, failure rates, pre-commit.
- Every boundary this deployment holds **declared once** in `eval/attributes.yaml`, and
  `just attributes-check` fails when one no longer resolves.
- Forgejo CI builds, checks policy, vets, tests, and runs pre-commit. Structure, skills, links, modules,
  comments, secrets, and prompt all validated. **Entrypoint failures logged as severity-carrying JSON,
  never bare stderr.**

## Deliberate exclusions

Echo has **no moderation, account, role, announcement, issue-body edit, delete, reaction, schema, or
ambient-channel surface**, sends no unsolicited direct message, and owns no web or mobile UI.

## The pages

Start at [sirens-echo.md](sirens-echo.md). Identity and composition:
[identity](sirens-echo-identity.md), [compose](sirens-echo-compose.md).
Ingress: [admission](sirens-echo-admission.md), [access](sirens-echo-access.md), [http](sirens-echo-http.md),
[mentions](sirens-echo-mentions.md), [threads](sirens-echo-threads.md).
Tools: [commands](sirens-echo-commands.md), [tools](sirens-echo-tools.md), [mcp](sirens-echo-mcp.md),
[tool-markup](sirens-echo-tool-markup.md).
The turn: [turn-stages](sirens-echo-turn-stages.md), [prompt](sirens-echo-prompt.md),
[model-call](sirens-echo-model-call.md), [reply-assembly](sirens-echo-reply-assembly.md),
[progress](sirens-echo-progress.md), [delivery](sirens-echo-delivery.md).
Safety: [content-gate](sirens-echo-content-gate.md), [boundaries](sirens-echo-attributes.md),
[phrases](sirens-echo-phrases.md), [untrusted-input](sirens-echo-untrusted-input.md),
[reasoning](sirens-echo-reasoning.md), [grounding](sirens-echo-grounding.md).
Work: [jobs](sirens-echo-jobs.md), [execution](sirens-echo-execution.md),
[scratchpad](sirens-echo-scratchpad.md), [issues](sirens-echo-issues.md),
[worklog](sirens-echo-worklog.md).
Configuration: [config](sirens-echo-config.md), [tuning](sirens-echo-tuning.md).
Telemetry: [observability](sirens-echo-observability.md), [telemetry](sirens-echo-telemetry.md), [rate](sirens-echo-rate.md).
Evaluation: [evals](sirens-echo-eval.md), [testing](sirens-echo-testing.md).
Shipping: [deploy](sirens-echo-deploy.md) and the `site/` publish.

## See also

- [README.md](../README.md) - human-facing introduction.
- [AGENTS.md](../AGENTS.md) - agent-facing rules.
- [justfile](../justfile) - development recipes.

Cross-reference convention from [coilysiren/agentic-os#59](https://github.com/coilyco-flight-deck/agentic-os/issues/59).
