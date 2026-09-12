# Sirens Echo response service

Sirens Echo binds a deployment-selected response profile to Discord or private HTTP ingress, Agent
Proxy, MCP servers, Sirens knowledge, Forgejo, and telemetry. **This repository owns the service and its
model-facing policy.** Agent Proxy owns inference transport, MCP servers own their published tools, and
deploy owns the k3s workload.

## Request flow

1. Discord delivers a guild message, or a direct message where enabled.
2. Echo accepts only a mention or reply in a configured channel or a thread under one, inside an
   allowlisted guild when an allowlist is set.
3. Admission charges the per-user, per-context, and global buckets, and sheds the summon if any is
   exhausted or the queue is full.
4. The runtime loads up to 12 earlier messages and **marks them untrusted**. Inside a thread that
   history and the reply stay in the thread.
5. The harness combines the selected local policy, history, and the summon.
6. It discovers the selected definition's MCP schemas and sends them to Agent Proxy. Which servers those
   are is deploy's, and the definition's game focus says which of them carry its game's live state.
7. It executes model-requested tools and continues with their results.
8. It enforces JSON, repairs once, validates, and may create or reuse an issue.
9. It applies the selected style contract and replies **with every Discord mention disabled**.

**Turns run one at a time.** A second summon reads history after the first finishes, so it sees the
prior member message and Echo reply. A summon waits up to the queue timeout for that slot, and its
request budget starts once it holds the slot. Echo keeps only bounded history and duplicate detection,
and no automatic memory. See [admission](sirens-echo-admission.md), [contexts](sirens-echo-threads.md).

The same validated turn path is reachable through `POST /v1/turn` on the process's private listener,
with its own shared-secret authentication and the same admission policy, and a deployment can disable
Discord and use it as the only ingress. See [the HTTP entrypoint](sirens-echo-http.md).

## The two profiles

The runtime separates identity, local policy, tools, issue tracking, model route, and ingress so **one
immutable image can support independently reviewed deployments**.

`agents/echo/definition.yaml` is the neutral Sirens Echo community profile. It selects the exact `#bots`
boundary, approved Sirens policy and knowledge, **one swappable game focus naming the game it answers
for** (the game focus section of [AGENTS.md](../AGENTS.md)), the private repository-fixed Forgejo MCP,
and that Forgejo server as its automatic issue tracker. The neutral voice admits an object emoji
for legibility and still refuses tone and status indicators, bounded at three per reply
([object emoji](sirens-echo-phrases.md)).

`agents/deep/definition.yaml` is the social CoilyCo general-purpose profile. **The filename remains the
stable deployment selection used by the existing `sirens-deep` workload.** Its identity is Sirens Deep
of Coilyco, its audit role is `general`, and its channel is empty because deployment owns routing. It
loads only `.agents/skills/coilyco-general/`, selects a Steam reader and the repository-fixed Forgejo
MCP whose addresses both come from deployment, and has no automatic issue tracker. It starts from the
user's request without assuming a project, community, product, or technical discipline, and **its voice
comes from that policy root rather than a scaffolded paragraph, so a copy edit to voice is a skill
diff.**

**The profiles share no knowledge or tools.** Both load an Agent Compose role bundle and neither loads a
shared behavioral context or lore source. Deep composes `devrel` and takes its voice from it. Echo
composes `sysadmin` for the doctrine and keeps the neutral voice its own policy root defines, which the
prompt states as precedence rather than leaving to section order. See
[composition](sirens-echo-compose.md) for why those are separate axes, and why `ops` is the operator
role rather than the infrastructure one. Both share the framing in [the prompt](sirens-echo-prompt.md).

The `coilyco-harness.agent.v1` definition makes three capability boundaries explicit: `channel` may be
empty for HTTP-only profiles, while Discord-enabled deployments still require the exact `#bots` value; a
profile selects no MCP server, so **tool access is a deploy edit rather than a profile change**; and
`issue_tracker` is optional and must name one server in the roster, so a profile without it must return
`issue: null` and cannot trigger automatic issue follow-up.

`just policy-check` renders and verifies both profiles. Agent Proxy gets one same-conversation,
style-aware repair with tools disabled, and JSON structure, reply bounds, privacy, action grounding, and
invented-channel checks stay shared. Outbound links are a separate axis
([link policy](sirens-echo-untrusted-input.md)). `SIRENS_ECHO_DEFINITION` selects a tracked definition. Discord ingress defaults on, and
`SIRENS_ECHO_DISCORD_ENABLED=false` removes the Discord token and channel requirements while keeping the
private HTTP turn path. `SIRENS_ECHO_INSTANCE` sets a lowercase telemetry service name, and
`AGENT_PROXY_MODEL` selects the logical route independently of response style and profile identity.
**This is the intended variance boundary**: shared runtime guarantees stay in the binary, while
identity, local policy, tools, automatic issue tracking, model choice, ingress, secrets, namespace, and
tailnet exposure remain explicit inputs. The Sirens Deep workload holds its own instance, namespace, and
tailnet identity, and an operator proves the general profile before it goes live
([the deployment gate](sirens-echo-deploy.md)).

## What this process can do

**A capability that was never switched on is indistinguishable, from inside the process, from one that
was never built.** That is a consequence of a property worth keeping, that an unset variable offers no
tools rather than tools that fail. The cost is that establishing "is feature X actually on?" meant
reading the harness and the deployment together, by someone who already suspected the answer. Three
capabilities were found built and inert that way, each by accident (sirens-echo#539).

So `Run` emits `capabilities` once, before the gateway opens, naming the content gate, the job store
kind, jobs, the scratchpad, fetch, the issue tracker, the Discord surfaces, and the roster size. **The
line says whether a capability is on, never its values.** Not where the scratchpad is mounted, which
hosts fetch may reach, or which channel is served: a log line naming those grows into an identifier
surface this service guards against elsewhere, and the operator asking "is it on" does not need them.
The roster is a count for the same reason.

**It reports, it does not judge.** Nothing here fails a startup, because the runtime cannot tell whether
an inert capability is a mistake: a profile that legitimately has no scratchpad and one that was meant
to have it are the same process. Only deployment knows which. The job store is the sharpest case and is
a string rather than a bool, since the durable store and the one that drops every in-flight job on a
roll are both "a job store" to a reader who only learns that one exists. What the service **refuses** to
do, as opposed to what it is configured to do, is [capability limits](sirens-echo-config.md).

## What each surface is for

The model receives every tool's schema on every turn but not **which server to reach for**, the
question members' requests actually pose. MCP's `InitializeResult.Instructions` answers it, and the
harness renders it beside the roster. See [the MCP roster](sirens-echo-mcp.md).
