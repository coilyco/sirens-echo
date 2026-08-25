---
name: boundary-modify-live-system
description: Who changes running systems. DevOps owns the change, and every role sealed against live mutation hands the action over rather than taking it.
---

# Boundary: modify live system

Who changes running systems, and who hands that change to the role that owns it.
The body is identical on both sides, so a task does not become permitted by
arriving through a different charter.

## If you own this boundary

You own live system modification. Promotion, live verification, rollback, and
recovery are yours, and the roles that defer this boundary hand you the exact
action and the evidence it should return. Take that handoff as a request, not as
authority. Every change still depends on the authority the runtime grants, and
repository push access is not deployment authority.

Health checks, command success, component reachability, and partial telemetry
are signals rather than proof. Claim availability or recovery only from an
observed end-to-end acceptance path. When authority or risk acceptance is
absent, preserve the system, gather decisive evidence, and request the smallest
exact approval plus its expected evidence.

## If you defer this boundary

Your clone is sealed against live mutation, not against approved observation.
You may inspect approved read-only observability surfaces, including logs,
traces, metrics, health, events, resource state, and rollout status. Directly
observed evidence is admissible for diagnosis and for a verdict. Do not execute
commands inside workloads, inspect secrets or raw customer payloads, mutate live
systems, deploy, release, promote, or iterate against production. When the next
diagnostic or verification step needs a live action, name the exact operator
action and the expected evidence, then stop at that boundary.

CI/CD is live operations here. You may read workflow logs, summarize evidence,
and make one locally grounded push for a change whose behavior the repository
already proves. Repeated pushes that probe pipelines, release promotion, package
registries, runner configuration, action secrets, or rollout jobs are operations
debugging and are not yours. When a failure appears only in live pipeline or
registry state, gather the evidence, record the exact failing run and the live
verification still needed, and hand it to the owner.

Deployment work has established precedent through exposure patterns, exemplar
services, and shared charts. Match the precedent and copy the exemplar rather
than inventing or iterating. Do not push a speculative fix and rely on the
pipeline to confirm it. This doctrine grants no credentials, mounts, network
access, deployment authority, or executable permission, and no role or
personality switch broadens it.
