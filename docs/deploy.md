# Deployment

The two bots use different deployment layers because they have different
dependencies.

## Admin bot native lifecycle

The admin bot executes host-local `coily gaming` commands, so it remains on
kai-server as a native service.

- `sirens-discord-ops.service` runs `scripts/start.sh`.
- `sirens-discord-ops-update.service` runs `scripts/auto-update.sh`.
- `sirens-discord-ops-update.timer` checks for a new main commit every five
  minutes.

`scripts/install.sh` installs those units and the admin-only sudoers drop-in.
The updater restarts only `sirens-discord-ops`.

## Sirens Echo application image

Sirens Echo has no host-local command dependency. The root `Dockerfile` builds
only `cmd/sirens-echo` and copies the source-controlled definition and
repo-local skillpack into a non-root runtime image.

The Forgejo workflow runs repository validation first. A main push then
publishes `sirens-echo:<git-sha>` to the in-cluster registry once the operator
has configured its push endpoint. That endpoint comes from the
`IN_CLUSTER_REGISTRY_PUSH` repository variable, so no opaque address lands in
source. Until the variable exists, the publish job stays skipped while the
repository validation job remains authoritative.

This application repository publishes an artifact only. It holds no cluster
credential and never reaches into the deployment layer.

## Sirens Echo k3s lifecycle

`coilyco-bridge/deploy/services/sirens-echo/` owns:

- The namespace and SSM-backed ExternalSecret
- A one-replica `Recreate` Deployment
- Resource and container security bounds
- The shared rollout values and path-scoped Forgejo workflow
- Rollback and live-verification instructions

`Recreate` prevents two Gateway sessions from overlapping during a rollout.
The worker exposes no listener, so it needs no Service, Ingress, certificate,
public DNS record, or Tailscale sidecar.

The deploy repository's scheduled reconciler detects a newly published source
SHA and rolls it through the in-cluster deploy runner. A deploy-repo config
change can also trigger the same shared rollout path.

## Echo rollback

The deploy layer can return Echo to zero replicas without changing the native
admin bot, source repository, skillpack, or SSM parameters. A later rollout
restores the reviewed image and definition.

See [sirens-echo-rollout.md](sirens-echo-rollout.md) for the operator gate.

## Admin SSM failure behavior

The native `scripts/start.sh` keeps two guards from the 2026-05-29 reboot
incident:

- The SSM command substitution is separate from the local declaration, so a
  failed fetch preserves its non-zero exit.
- An explicit empty-value check catches a present but blank parameter.

A broken admin credential therefore fails at its source instead of opening a
Discord session with an empty token.
