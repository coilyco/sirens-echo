FROM forgejo.coilysiren.me/coilyco-flight-deck/agentic-os:release AS build

USER root
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY cmd ./cmd
COPY internal ./internal
COPY agent ./agent
COPY scripts/build-community-bundle.sh ./scripts/build-community-bundle.sh
RUN CGO_ENABLED=0 go build -trimpath -o /out/sirens-echo ./cmd/sirens-echo \
    && CGO_ENABLED=0 go build -trimpath -o /out/sirens-echo-bundle-check ./cmd/sirens-echo-bundle-check
RUN scripts/build-community-bundle.sh \
      /opt/substrate-seed/coilyco-flight-deck__agentic-os.git \
      /out/community-bundle \
    && /out/sirens-echo-bundle-check /out/community-bundle

FROM forgejo.coilysiren.me/coilyco-flight-deck/agentic-os:release

USER root
WORKDIR /app
COPY --from=build --chown=1000:1000 /out/sirens-echo /usr/local/bin/sirens-echo
COPY --from=build --chown=1000:1000 /out/community-bundle /app/agent/community-bundle
COPY --chown=1000:1000 agent/sirens-echo.json /app/agent/sirens-echo.json
COPY --chown=1000:1000 .agents/skills/sirens-echo-community /app/.agents/skills/sirens-echo-community
USER 1000:1000
ENTRYPOINT ["/usr/local/bin/sirens-echo"]
