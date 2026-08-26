# Configuration

What a definition pins, what the deployment supplies, and what the service will not claim to do.

## Definitions and deployment variables

`agents/echo/definition.yaml` pins the neutral Community policy and
`agents/deep/definition.yaml` the social CoilyCo general-purpose policy. Each pins audit attribution,
response style, optional channel, history budget, local policy roots, optional MCP roster, and optional
automatic issue tracker. **Neither pins a backend model or loads a behavioral role or seat**, and the
audit role never reaches the model prompt.

Agent Proxy is the inference transport: deployment reads the selected `<namespace>/<alias>` route from
`/sirens-echo/agent-proxy-model` and passes it through **without a source default**, and Echo uses that
value for inference and route readiness alike. Every model round runs at **temperature zero** over the
selected policy, bounded history, and the current request, and Echo attempts one same-conversation,
style-aware repair with tools disabled, **then fails closed before Discord delivery**.

* `SIRENS_ECHO_DEFINITION` selects the tracked definition. `SIRENS_ECHO_DISCORD_ENABLED=false` removes
  the token and channel requirements while retaining the HTTP turn path, and `SIRENS_ECHO_INSTANCE` sets
  a distinct lowercase service name for telemetry.
* `SIRENS_ECHO_ROLE` selects the baked bundle a `composed: true` definition loads, from
  `SIRENS_ECHO_BUNDLE_DIR`. Echo names `sysadmin`, Deep names `devrel`, **and there is no default**.
* `SIRENS_ECHO_PRINCIPAL_HANDLE` and `SIRENS_ECHO_PRINCIPAL_USER_ID` name the one speaker the prompt
  trusts. **Set both or neither**: a half-configured principal stops the process, and an unset one
  renders no identity signals at all.
* `SIRENS_ECHO_MCP_ROSTER` names the roster file, **so which surfaces exist and where they live is a
  deploy edit rather than a change here** - a definition selects no MCP server.
* `SIRENS_ECHO_ACCESS_POLICY` names the tracked allowlist file. Without it, `DISCORD_CHANNEL_ID`,
  `DISCORD_GUILD_IDS`, and `SIRENS_ECHO_DISCORD_DM_ENABLED` synthesize the equivalent.

The definition's `channel` is **the prompt's boundary label, not the routing key**, and must be empty or
a `#channel-name` the grounding validator also accepts. An empty channel is transport-neutral and valid
for Discord and HTTP alike, an empty MCP roster is valid, and `issue_tracker` must name a configured
server when present.

## Encoded capability limits

`.agents/skills/sirens-echo-knowledge/references/capability.md` tells the model what the service can do.
**This section is the reviewer's copy and cites the code that sets each bound**, so a change that moves
a number surfaces as a stale document rather than a stale belief. **The model-facing file carries no
citations on purpose.**

* **Tool rounds per turn** - 6, then the turn fails (`proxy.go:21`, `:411`).
* **Tool execution** - sequential, fail-fast (`proxy.go:428`).
* **Reply size** - 1800 characters (`decision.go:47`).
* **History supplied** - 12 messages (`agents/echo/definition.yaml`).
* **Job kinds** - `echo` and `ward-exec` only (`jobsubmit.go:24`).

**Nothing schedules a turn**, so no reply may describe work continuing after it is sent. A reply
carrying the model's own unparsed tool-call markup is **refused rather than stripped**, because
stripping leaves a claim to have used a tool with no call behind it, **the invented-work failure the
grounding gates exist to catch**. It is deliberately not in `ParseReply`, which the evaluation scorer
and the repair loop both call, **so a runtime gate there would reshape what the deployment gate and the
rate pack measure**; the pattern set is still shared, so the two cannot drift.

**The reports behind this were not the model claiming a tool it lacked.** They were the model describing
an aspiration in the grammar of a shipped capability, as in "the system is now processing these requests
sequentially", **and a prohibition on inventing tools does not reach that sentence, because it names no
tool**. **The asynchronous job surface is real in the binary and left out of the model-facing file**,
because nothing in the deployed Echo values enables a job store for this lane. Sirens Deep is not
covered, loading `coilyco-general` and never seeing this policy root, **so the same defect can recur
there against a different file**.

## Knowing its own authority

Sirens Deep can describe what it is allowed to do without being handed prose that drifts from the
deployed boundary. **A hand-written skill describing a guardfile is a description of a document, not a
description of live authority**, and **an agent confidently misdescribing its own boundaries on a
permanent public recording is worse than one that declines to describe them**. So `just guardfile-skill`
parses the deployed guardfile and writes `.agents/skills/coilyco-general/references/guardfile.md`,
inside Deep's `local_skill_roots`.

**Deny by absence is the interesting half.** The agent already sees its granted tools as schemas; what a
tool list cannot show is the shape of the denial: every path is fixed to one repository with no owner or
repository argument to redirect, and edits and deletes are denied **by absence** rather than by a rule
saying no. **Absence is the part worth teaching, because there is no exception to find and nothing to
argue with**, so the skill tells the agent to say "I do not have it" rather than reason about whether
something unlisted would be permitted.

The rendered skill carries a digest of its source and `just guardfile-skill-check` fails when the
tracked skill no longer matches. **But the guardfile lives in the private `coilyco-bridge/deploy` and
this repository is public**, so the check cannot run in this repository's CI, **and vendoring a copy
here would recreate the drift the generator exists to remove**. The verification has to run **where the
guardfile changes**, and the generator takes a `--guardfile` path so it can run from either side.
**Until the deploy-side hook lands, drift is caught by whoever runs the check, not by a gate.** This is
knowledge about authority, not authority. **Four layers hold the boundary** - network controls, the ward
KDL, harness config, and prose instruction - **and the KDL is the layer worth teaching**, because
network controls cannot be shown, harness config is YAML anyone could have written, and prose does not
hold under pressure.

## Exception taxonomy

SigNoz groups exceptions by `service.name` and `exception.type`, so Echo records every failure through
one typed, source-controlled catalog. `exception.type` is one of 38 unique `sirens_echo.*` values;
`exception.message` and the span status description use the same fixed sentence; `error.stage` has ten
values and `error.outcome` is owned by the selected entry; and **`error.fault` is `caller` or `service`,
so a caller mistake does not inflate the service error rate**. The stage cannot stand in for it, because
`prompt_failed` is an MCP failure surfaced on the HTTP path and `rate_limited` is the service refusing a
well-formed request, **and a code declaring neither fails the suite**. **Echo emits no
`exception.stacktrace`**, and an invalid or unclassified code becomes `sirens_echo.telemetry.
unclassified` **without retaining the supplied value**.

**The recording API accepts only the catalog's numeric code type**, so upstream error text, request
identifiers, member data, URLs and paths, credentials, and payloads cannot be passed into the exception
event. Regression coverage exercises every entry, checking the event, span attributes, status
description, stack-trace omission, cardinality, and safe fallback.
