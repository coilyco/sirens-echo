FROM forgejo.coilysiren.me/coilyco-flight-deck/agentic-os:release AS build

USER root
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY cmd ./cmd
COPY internal ./internal
COPY agent ./agent
COPY agents ./agents
COPY .agents/skills ./.agents/skills
COPY docs ./docs
ARG SIRENS_ECHO_REVISION=
RUN CGO_ENABLED=0 go build -trimpath \
    -ldflags "-X forgejo.coilysiren.me/coilyco-gaming/sirens-echo/internal/community.buildRevision=${SIRENS_ECHO_REVISION}" \
    -o /out/sirens-echo ./cmd/sirens-echo \
    && CGO_ENABLED=0 go build -trimpath -o /out/sirens-echo-policy-check ./cmd/sirens-echo-policy-check \
    && CGO_ENABLED=0 go build -trimpath -o /out/sirens-echo-compose ./cmd/sirens-echo-compose \
    && CGO_ENABLED=0 go build -trimpath -o /out/sirens-echo-prompt ./cmd/sirens-echo-prompt \
    && CGO_ENABLED=0 go build -trimpath -o /out/sirens-echo-access-check ./cmd/sirens-echo-access-check \
    && CGO_ENABLED=0 go build -trimpath -o /out/sirens-echo-definition-check ./cmd/sirens-echo-definition-check \
    && /out/sirens-echo-policy-check

# The release image ships agent-compose but not the composed catalogue, so this
# stage fetches it. The ref floats on main by design, so a rebuild takes the
# catalogue as it stands; override it to reproduce an older bundle.
# See docs/sirens-echo-compose.md.
FROM forgejo.coilysiren.me/coilyco-flight-deck/agentic-os:release AS compose
ARG AOS_CATALOG_REF=main
# The clone below caches on this instruction's text, which never changes, so the
# floating ref froze. This is the resolved commit, and it is what keys the layer.
ARG AOS_CATALOG_HEAD
USER root
WORKDIR /src
# The resolved commit is fetched rather than the branch re-resolved, because the
# branch moves during the build and re-resolving raced it. See sirens-echo#1118.
RUN set -eu; \
    if [ -z "${AOS_CATALOG_HEAD:-}" ]; then \
      echo "AOS_CATALOG_HEAD is required; resolve it with scripts/lib/catalog-head.sh" >&2; \
      exit 1; \
    fi; \
    catalogue=https://forgejo.coilysiren.me/coilyco-flight-deck/agentic-os.git; \
    git init -q /tmp/aos-catalog; \
    git -C /tmp/aos-catalog remote add origin "${catalogue}"; \
    if ! git -C /tmp/aos-catalog fetch -q --depth 1 origin "${AOS_CATALOG_HEAD}"; then \
      rm -rf /tmp/aos-catalog; \
      git clone -q --branch "${AOS_CATALOG_REF}" "${catalogue}" /tmp/aos-catalog; \
    fi; \
    git -C /tmp/aos-catalog checkout -q --detach "${AOS_CATALOG_HEAD}"; \
    checked=$(git -C /tmp/aos-catalog rev-parse HEAD); \
    if [ "${checked}" != "${AOS_CATALOG_HEAD}" ]; then \
      echo "catalogue checked out ${checked}, caller resolved ${AOS_CATALOG_HEAD}" >&2; \
      exit 1; \
    fi
COPY agent ./agent
COPY agents ./agents
COPY .agents/skills ./.agents/skills
COPY scripts/stage-compose-sources.sh ./scripts/
# The expander comes from the build stage, so this stage needs no Go toolchain
# work and the binary is the one the suite already exercised.
COPY --from=build /out/sirens-echo-compose /usr/local/bin/sirens-echo-compose
COPY --from=build /out/sirens-echo-prompt /usr/local/bin/sirens-echo-prompt
RUN SIRENS_ECHO_COMPOSE_BIN=/usr/local/bin/sirens-echo-compose \
    bash scripts/stage-compose-sources.sh /out/bundles /tmp/aos-catalog
# Every baked role must render a valid composed prompt, so one that failed to
# compose stops the image here. See docs/sirens-echo-compose.md.
RUN sirens-echo-prompt --bundles /out/bundles

FROM forgejo.coilysiren.me/coilyco-flight-deck/agentic-os:release

USER root
WORKDIR /app
COPY --from=build --chown=1000:1000 /out/sirens-echo /usr/local/bin/sirens-echo
# Shipped so another layer can expand the same allowlist with catalogues this
# build cannot see. See docs/sirens-echo-compose.md.
COPY --from=build --chown=1000:1000 /out/sirens-echo-compose /usr/local/bin/sirens-echo-compose
# Deploy's CI invokes this against the ConfigMap before applying it, so it has
# to reach the released image and not only the build stage. See #628.
COPY --from=build --chown=1000:1000 /out/sirens-echo-access-check /usr/local/bin/sirens-echo-access-check
# The same shape, for the field where divergence from the image is a bug rather
# than a preference. Run from the image it compares a deploy-owned definition
# against the tree this image actually carries. See sirens-echo#973.
COPY --from=build --chown=1000:1000 /out/sirens-echo-definition-check /usr/local/bin/sirens-echo-definition-check
COPY --chown=1000:1000 scripts/stage-compose-sources.sh /app/scripts/stage-compose-sources.sh
COPY --chown=1000:1000 agent /app/agent
# Eval material, whose only readers are binaries this stage does not ship. The
# copy cannot be narrowed: the build stage runs policy-check over these paths.
RUN rm -f /app/agent/*fixture*.yaml
# Definitions only. The rest of agents/ is probes, board cases, and graded
# replies, and cwd holding the answers to its own tests makes any evaluation run
# there unfalsifiable. A wildcard would flatten them onto one path, so each is
# named, and TestTheImageShipsEveryDefinitionAndNoEvalMaterial holds the list
# complete. See docs/sirens-echo-eval.md and sirens-echo#1012.
COPY --chown=1000:1000 agents/echo/definition.yaml /app/agents/echo/definition.yaml
COPY --chown=1000:1000 agents/deep/definition.yaml /app/agents/deep/definition.yaml
COPY --chown=1000:1000 .agents/skills /app/.agents/skills
# A wildcard here copies each root's contents rather than the root, flattening
# the tree, and a definition naming a root then crashes at startup. deploy#666.
RUN set -eu; \
    for root in /app/.agents/skills/*/; do \
      [ -f "${root}SKILL.md" ] || { echo "skill root ${root} lost its SKILL.md" >&2; exit 1; }; \
    done; \
    [ ! -e /app/.agents/skills/SKILL.md ] || \
      { echo "a SKILL.md landed at the skills root, so the tree was flattened" >&2; exit 1; }
COPY --from=compose --chown=1000:1000 /out/bundles /app/agent/bundles
USER 1000:1000
ENTRYPOINT ["/usr/local/bin/sirens-echo"]
