# Deployment

## Sirens Echo application image

Sirens Echo has no host-local command dependency. The root `Dockerfile` builds
`cmd/sirens-echo` on the full AOS release image. During the build,
agent-compose clones the image's seeded public AOS provider, composes and
verifies the Community bundle, and runs Echo's consumer check. The final
non-root image contains the full AOS runtime substrate, the compiled Community
bundle, the source-controlled definition, and the repo-local Sirens skill. It
does not add lore.

The Forgejo workflow runs repository validation first. A main push then
publishes the private image
`forgejo.coilysiren.me/coilyco-gaming/sirens-discord-ops:<full-source-sha>`.
The trusted deploy runner supplies `REGISTRY_TOKEN` for package writes. The
publisher uses an isolated Docker config, pushes one immutable tag, and proves
the remote manifest exists.

This application repository publishes an artifact only. It holds no cluster
credential and never reaches into the deployment layer.

The image derives its logical Agent Proxy route from
`agent/sirens-echo.json`. Deployment supplies the Agent Proxy endpoint but no
physical model selection. AOSH and Agent Proxy keep backend choice below the
harness boundary.

## Sirens Echo k3s lifecycle

`coilyco-bridge/deploy/services/sirens-echo/` owns:

- The namespace and SSM-backed ExternalSecret
- A one-replica `Recreate` Deployment
- Resource and container security bounds
- OTLP/HTTP access to the existing SigNoZ collector
- The shared rollout values and path-scoped Forgejo workflow
- Rollback and live-verification instructions

`Recreate` prevents two Gateway sessions from overlapping during a rollout.
The worker exposes no listener, so it needs no Service, Ingress, certificate,
public DNS record, or Tailscale sidecar.

The deploy repository's scheduled reconciler detects a newly published source
SHA, pulls it with the separate read-only `forgejo-registry` credential, and
rolls it through the in-cluster deploy runner. A deploy-repo config change can
also trigger the same shared rollout path.

## Echo rollback

The deploy layer can return Echo to zero replicas without changing the source
repository, skillpack, or SSM parameters. A later rollout restores the
reviewed image and definition.

See [sirens-echo-rollout.md](sirens-echo-rollout.md) for the operator gate.
