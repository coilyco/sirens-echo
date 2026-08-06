FROM forgejo.coilysiren.me/coilyco-flight-deck/agentic-os:release AS build

USER root
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY cmd ./cmd
COPY internal ./internal
COPY agent ./agent
COPY .agents/skills/sirens-echo-community ./.agents/skills/sirens-echo-community
COPY .agents/skills/sirens-echo-knowledge ./.agents/skills/sirens-echo-knowledge
COPY .agents/skills/coilyco-general ./.agents/skills/coilyco-general
RUN CGO_ENABLED=0 go build -trimpath -o /out/sirens-echo ./cmd/sirens-echo \
    && CGO_ENABLED=0 go build -trimpath -o /out/sirens-echo-policy-check ./cmd/sirens-echo-policy-check \
    && /out/sirens-echo-policy-check

FROM forgejo.coilysiren.me/coilyco-flight-deck/agentic-os:release

USER root
WORKDIR /app
COPY --from=build --chown=1000:1000 /out/sirens-echo /usr/local/bin/sirens-echo
COPY --chown=1000:1000 agent /app/agent
COPY --chown=1000:1000 .agents/skills/sirens-echo-community /app/.agents/skills/sirens-echo-community
COPY --chown=1000:1000 .agents/skills/sirens-echo-knowledge /app/.agents/skills/sirens-echo-knowledge
COPY --chown=1000:1000 .agents/skills/coilyco-general /app/.agents/skills/coilyco-general
USER 1000:1000
ENTRYPOINT ["/usr/local/bin/sirens-echo"]
