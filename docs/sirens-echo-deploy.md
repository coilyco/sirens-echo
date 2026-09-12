# Deployment

The image this repository publishes, the k3s lifecycle deploy owns, and what an operator proves first.

## The application image

The root `Dockerfile` builds `cmd/sirens-echo` on the full AOS release image, and during the build the
tracked definitions, repo-local policy roots, and the reference access policy are loaded and checked for
their selected contracts. **`just test` proves that check passes with only the files the build context
carries.** The final non-root image contains both definitions and the policy roots; **it loads no AOS or
Agent Compose context and does not add lore.** A main push publishes
`forgejo.coilysiren.me/coilyco-gaming/sirens-echo:<full-source-sha>`, the publisher using an isolated
Docker config, pushing one immutable tag, and proving the remote manifest exists. **This application
repository publishes an artifact only. It holds no cluster credential and never reaches into the
deployment layer.**

## The k3s lifecycle

`coilyco-bridge/deploy/services/sirens-echo/` owns the namespace and Echo's SSM-backed ExternalSecret, a
private repository-fixed Ward MCP workload with its own ExternalSecret, a one-replica `Recreate`
Deployment, a private ClusterIP Service and Tailscale sidecar, resource and container security bounds,
OTLP/HTTP access to the SigNoz collector, the shared rollout values and path-scoped workflow, and the
rollback instructions.

**The deploy layer may run multiple instances from the same immutable image**, owning each instance's
definition path, ingress switch, Agent Proxy route, namespace, tailnet identity, and telemetry name.
**The deployment name is a stable routing identity, not the model's domain or policy.** `Recreate`
prevents two Gateway sessions from overlapping during a rollout. **The private tracker MCP holds the
only tracker token** and exposes only a ClusterIP Service, with no public, tailnet, certificate, DNS, or
NodePort route. The deploy repository pins published full source SHAs and pulls them with the separate
read-only `forgejo-registry` credential, and **the rollout makes the tracker MCP ready before it updates
Echo**. The deploy layer can return Echo to zero replicas **without changing the source repository,
skillpack, or SSM parameters**.

## The operator gate

Before raising replicas, an authorized operator confirms the private intake tracker records the selected
Community model, stores it at `/sirens-echo/agent-proxy-model`, confirms Agent Proxy advertises it, that
`just policy-check` verifies both response policies, and that the reviewed full-SHA image exists in
Forgejo OCI. Then: **the tracker token exists without printing it**, only the private tracker MCP
ExternalSecret references it, the Discord token and `#bots` identifier resolve **without printing either
value**, Message Content is enabled with Echo limited to view, read history, and send in `#bots`, deploy
uses exact image SHAs with its read-only pull credential, and Agent Proxy, the roster's MCP servers,
and SigNoz are reachable. **Echo receives the private MCP's ClusterIP URL but no tracker
credential, and no secret belongs in a tracked file, shell history, issue, or chat.**

**Missing SSM values fail before either workload becomes ready.** Agent Proxy, MCP, loop,
response-contract, or validation failures return a neutral retry reply, **invalid output never reaches
Discord or the tracker**, and the pod gets no AWS credentials. **Tracker failure never makes Echo claim an
issue exists**: the answer still posts while logs record the failed follow-through. To roll back, the
operator restores the prior deploy commit, or leaves Echo at zero replicas, and **reruns evaluation
before restoring one replica**.

## The CoilyCo gate

The Sirens Deep workload selects the hosted DeepSeek route and loads the CoilyCo definition, receiving
its own instance, namespace, tailnet hostname, and non-reusable Tailscale key. Its Discord ingress
refuses every guild, channel, and account its access policy does not name, and **it holds no tracker
secret, because that credential lives only in the MCP pod**. **Separate instance and separate namespace
is the point rather than an implementation detail**: the two profiles differ in what they will talk
about, **and a shared workload would make that difference a configuration value instead of a boundary**.

Before live rollout an operator verifies the immutable image and route registries, applies the
Terraform-managed Tailscale service entry and namespace RBAC, then deploys the reviewed values. **Tailnet
HTTP verification must prove the general profile answers unrelated topics without assuming a game or
community domain and without weakening truth, privacy, or action grounding. Both halves matter**: a
general profile that refuses unrelated topics has not generalised, **and one that answers them by
dropping a grounding rule has traded the wrong thing for the breadth**.

## Checking a file deploy owns

Two of Echo's configuration files live in the deploy repository as Kubernetes ConfigMaps, **and anything
here that validates one has to know that, because the pod never sees the shape the repository stores**.
Kubernetes mounts the value under `data:` as a file, so **the runtime loader receives a bare document
and is right to expect one**, while an offline checker reads the wrapper. **The two inputs are different
and both are real.**

**Read the inner document, and let deploy extract it.** Unwrapping `data:` inside the checker **is wrong
twice**: it puts Kubernetes knowledge in the repository that does not own k3s, and it makes the checker
accept a shape the runtime never sees, **so the checker and the pod stop agreeing about what a valid
file is**. **Refuse the wrapper with a message that names it**, and **test against a real file**,
because a fixture written from the format's documentation describes the mounted shape, **the one input
the checker will never be handed**. **Do not iron out how the loaders differ**: `LoadAccessPolicy`
narrows a schema this repository owns, so strict decoding is free, while `LoadMCPRoster` cannot, the
`mcpServers` shape being shared with mcporter, Claude Code, and Codex. **Same wrong file, two refusals,
both correct for their format.**

## Which commit is answering

The publish script already derives the full source sha to tag the image, and now passes that same value
as a build argument stamped into a package variable with a linker assignment: **one source of truth,
used twice**. **`.git` is deliberately not copied into the build stage**, because letting the toolchain
stamp `vcs.revision` would add the whole repository to the build context to recover one string, **and it
would make the revision depend on how the tree was checked out rather than on what the publisher
tagged**. A local build passes no argument, so the revision is empty and `BuildRevision` returns an
empty string - **the honest answer for a build that carried no revision, and why readiness records the
value rather than a placeholder: an empty field is visibly different from a wrong one**. **Naming a
commit does not let the process read that commit**: a source link is still current source rather than
the code that answered, unless the revision is named alongside it, and that claim is bound to the
Dockerfile by a test.
