# Runtime response profiles

The runtime separates identity, local policy, tools, issue tracking, model
route, and ingress so one immutable image can support independently reviewed
deployments.

## Repository profiles

`agent/sirens-echo.yaml` is the neutral Sirens Echo community profile. It
selects the exact `#bots` boundary, approved Sirens policy and knowledge, the
public Eco MCP, the private repository-fixed Forgejo MCP, and that Forgejo
server as its automatic issue tracker.

`agent/sirens-deep.yaml` is the social CoilyCo general-purpose profile. The
filename remains the stable deployment selection used by the existing
`sirens-deep` workload. Its model identity is CoilyCo, its audit role is
`general`, and its channel is empty because the profile is HTTP-only. It
loads only `.agents/skills/coilyco-general/`, has no MCP roster, and has no
automatic issue tracker.

The CoilyCo profile starts from the user's request without assuming a project,
community, product, or technical discipline. It retains a warm,
proportionate voice while keeping truth, uncertainty, privacy, action
grounding, and the deployment boundary above personality.

Neither profile loads an Agent Compose role, seat, personality meld, shared
behavioral context, or lore source.

## Definition capabilities

The `coilyco-harness.agent.v1` definition makes three capability boundaries
explicit:

- `channel` may be empty for HTTP-only profiles. Discord-enabled deployments
  still require the exact `#bots` value.
- `mcp_servers` may be empty. A profile gains tool access only through a
  reviewed tracked entry.
- `issue_tracker` is optional and must name one configured MCP server. A
  profile without it must return `issue: null` and cannot trigger automatic
  issue follow-up.

`ward exec policy-check` renders and verifies both profiles. Agent Proxy gets
one same-conversation, style-aware repair with tools disabled. JSON structure,
reply bounds, privacy, action grounding, and invented-channel checks remain
shared.

## Deployment controls

`SIRENS_ECHO_DEFINITION` selects a tracked definition. Discord ingress
defaults on. `SIRENS_ECHO_DISCORD_ENABLED=false` removes the Discord token
and channel requirements while keeping the private HTTP turn path.
`SIRENS_ECHO_INSTANCE` sets a lowercase telemetry service name.
`AGENT_PROXY_MODEL` selects the logical route independently of response
style and profile identity.

This is the intended variance boundary. Shared runtime guarantees stay in the
binary. Identity, local policy, tools, automatic issue tracking, model choice,
ingress, secrets, namespace, and tailnet exposure remain explicit inputs.

## CoilyCo deployment gate

The existing Sirens Deep workload disables Discord, selects the hosted
DeepSeek route, and loads the CoilyCo definition. It receives its own instance,
namespace, tailnet hostname, and non-reusable Tailscale key. It receives no
Discord or Forgejo secret and does not require either MCP endpoint.

Before live rollout, an authorized operator verifies the immutable image and
route registries, applies the Terraform-managed Tailscale service entry and
namespace RBAC, then deploys the reviewed values. Tailnet HTTP verification
must prove the general profile answers unrelated topics without assuming a
game or community domain and without weakening truth, privacy, or action
grounding. The neutral Sirens Echo deployment is verified separately and
remains unchanged.
