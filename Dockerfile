FROM golang:1.25 AS build

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY cmd ./cmd
COPY internal ./internal
RUN CGO_ENABLED=0 go build -trimpath -o /out/sirens-echo ./cmd/sirens-echo

FROM gcr.io/distroless/static-debian12:nonroot

WORKDIR /app
COPY --from=build /out/sirens-echo /usr/local/bin/sirens-echo
COPY agent/sirens-echo.json /app/agent/sirens-echo.json
COPY .agents/skills/sirens-echo-community /app/.agents/skills/sirens-echo-community
ENTRYPOINT ["sirens-echo"]
