# Sirens Echo response profiles

Sirens Echo separates facts, response style, model route, and ingress so one
immutable image can support independently reviewed deployments.

## Repository profiles

`agent/sirens-echo.json` selects the neutral Sirens Echo Community policy.
`agent/sirens-deep.json` selects the warm Sirens Deep social-host policy. Both load
`.agents/skills/sirens-echo-knowledge/`, the same MCP roster, history bound,
channel boundary, and action rules.

The neutral profile rejects greetings, first-person voice, emoji, banter, and
other personified language. The social profile permits a proportional greeting,
first-person voice, and light humor. It still requires truth, uncertainty,
privacy, grounding, and tool-result evidence. It does not load an Agent Compose
role, seat, personality meld, or shared behavioral context.

`ward exec policy-check` renders and verifies both profiles. Agent Proxy gets
one same-conversation, style-aware repair with tools disabled. JSON structure,
reply bounds, issue sanitization, action grounding, and invented-channel checks
remain identical for both profiles.

## Deployment controls

`SIRENS_ECHO_DEFINITION` selects a tracked definition. Discord ingress defaults
on. `SIRENS_ECHO_DISCORD_ENABLED=false` removes the Discord token and channel
requirements while keeping the private HTTP turn path. `SIRENS_ECHO_INSTANCE`
sets a lowercase telemetry service name. `AGENT_PROXY_MODEL` selects the
service-owned logical route independently of response style.

This is the intended variance boundary. Shared behavior stays in the binary and
repository definitions. Model choice, response-profile selection, ingress,
identity, secrets, namespace, and tailnet exposure stay in deploy.

## Sirens Deep deployment gate

Sirens Deep uses the social definition, disables Discord, selects the hosted
DeepSeek route, and receives its own instance, namespace, tailnet hostname, and
non-reusable Tailscale key. It receives no Discord secret.

Before live rollout, an authorized operator verifies the immutable image and
route registries, applies the Terraform-managed Tailscale service entry and
namespace RBAC, then deploys the reviewed values. Tailnet HTTP verification
must prove the social voice works without weakening grounding, privacy, issue,
or tool-action checks. The existing neutral Community deployment is verified
separately and remains unchanged.
