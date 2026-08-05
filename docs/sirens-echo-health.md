# Sirens Echo health signals

Echo separates process liveness, structural route readiness, and evidence of
successful inference. Each signal answers a different operational question.

## Local liveness

`GET /healthz` returns `{"ok":true}` with `200`. It inspects no dependency and
does not contact Discord, Agent Proxy, LiteLLM, Ollama, or MCP. A downstream
outage therefore cannot make Kubernetes restart a healthy Echo process.

## Route readiness

`GET /readyz` checks the exact `<namespace>/<alias>` route configured in
`AGENT_PROXY_MODEL`. Echo calls Agent Proxy's corresponding
`GET /readyz/{namespace}/{alias}` endpoint with an uninstrumented HTTP client
and a five-second ceiling.

Echo returns only `{"status":"ready"}` with `200` or
`{"status":"not_ready"}` with `503`. It does not relay a downstream body,
error, URL, host, credential, check name, or physical model. Unknown routes,
timeouts, malformed replies, dependency failures, and route-not-ready replies
all fail closed.

The request never calls chat, completions, generation, embeddings, or MCP.
Agent Proxy performs only its bounded LiteLLM and Ollama control-surface checks.

## Metrics-only policy

Both health handlers bypass the traced `/v1/turn` wrapper. Readiness also uses
a transport without OpenTelemetry instrumentation. Health requests create no
application or access log, server or client span, turn trace, model-call
metric, retained body, or model request.

Their OTLP metrics are:

* `sirens_echo.health.requests` with fixed `endpoint` and `outcome` values
* `sirens_echo.readiness.duration` in milliseconds with a fixed outcome
* `sirens_echo.readiness.state`, where one is ready and zero is not ready
* `sirens_echo.readiness.last_success`, a Unix timestamp in seconds

Labels never contain raw URLs, error text, request identifiers, member data,
physical models, credentials, or other unbounded values.

## Operational meaning

Liveness proves the process can answer locally. Route readiness proves the
configured inference path is structurally available without consuming a model
turn or extending VRAM residency. Passive successful turn metrics remain the
evidence that model loading, GPU execution, and a valid completion recently
worked. Deployment probe and external-monitor selection belong to the Deploy
repository.

See [the service](sirens-echo.md), [observability](sirens-echo-observability.md),
and [configuration](sirens-echo-config.md).
