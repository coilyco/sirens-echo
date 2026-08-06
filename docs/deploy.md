# Deployment

## Sirens Echo application image

Sirens Echo has no host-local command dependency. The root `Dockerfile` builds
`cmd/sirens-echo` on the full AOS release image. During the build, the tracked
definitions and repo-local policy roots are loaded and checked for their
selected contracts. The final non-root image contains the full AOS runtime
substrate, both source-controlled definitions, the Sirens policy roots, and the
general CoilyCo policy root. It loads no AOS or Agent Compose context and does
not add lore.

The Forgejo workflow runs repository validation first. A main push then
publishes the private image
`forgejo.coilysiren.me/coilyco-gaming/sirens-echo:<full-source-sha>`.
The trusted deploy runner supplies `REGISTRY_TOKEN` for package writes. The
publisher uses an isolated Docker config, pushes one immutable tag, and proves
the remote manifest exists.

This application repository publishes an artifact only. It holds no cluster
credential and never reaches into the deployment layer.

## Sirens Echo k3s lifecycle

`coilyco-bridge/deploy/services/sirens-echo/` owns:

- The namespace and Echo SSM-backed ExternalSecret
- A private, repository-fixed Ward MCP workload and its separate ExternalSecret
- A one-replica `Recreate` Deployment
- A private ClusterIP Service and Tailscale sidecar
- Resource and container security bounds
- OTLP/HTTP access to the existing SigNoZ collector
- The shared rollout values and path-scoped Forgejo workflow
- Rollback and live-verification instructions

The deploy layer may run multiple instances from the same immutable image. It
owns each instance's definition path, ingress switch, Agent Proxy route,
namespace, tailnet identity, and telemetry name. Sirens Echo stays neutral on
Discord. The existing Sirens Deep deployment is HTTP-only, selects DeepSeek,
and loads the general-purpose CoilyCo definition. The deployment name is a
stable routing identity, not the model's domain or policy.

`Recreate` prevents two Gateway sessions from overlapping during a rollout.
The private Forgejo MCP holds the only Forgejo token and exposes only a
ClusterIP Service. Echo reaches its `/mcp` and automatic HTTP tool surfaces by
service name. No public, tailnet, certificate, DNS, or NodePort route exists
for that service.

The deployment binds Echo's HTTP listener to port 8080. The shared private
ingress chart exposes that listener at `http://sirens-echo:8080` through a
Tailscale sidecar and ClusterIP Service, with no public Ingress, certificate,
DNS record, or NodePort.

The deploy repository pins published full source SHAs and pulls them with the
separate read-only `forgejo-registry` credential. Advancing an immutable pin
or changing either shared chart triggers the in-cluster deploy runner. The
rollout makes the Forgejo MCP ready before it updates Echo.

## Echo rollback

The deploy layer can return Echo to zero replicas without changing the source
repository, skillpack, or SSM parameters. A later rollout restores the
reviewed image and definition.

See [sirens-echo-rollout.md](sirens-echo-rollout.md) for the operator gate.
