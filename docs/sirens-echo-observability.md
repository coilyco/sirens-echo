# Observability

Accepted `#bots` and private HTTP turns are observable **without copying content into telemetry**. Logs,
traces, and metrics all reach SigNoz over OTLP/HTTP, and logs stay on stdout too. With `SENTRY_DSN`
set, a failed start, a `Run` error, or a main-goroutine panic also goes to Sentry, and a handled error
never does (teable:coilyco/deploy#8347). **A panic on another goroutine dies unreported.**

## Metadata logs

The runtime logs transport and request correlation identifiers, closed-set failure types, body byte
counts, model round, HTTP status, tool identity, outcome, and stage. Every metadata log inside a trace
carries `trace_id` and `span_id`. **Tokens, authorization headers, and SSM values stay out. Member and
history input, prompts, schemas, tool payloads, model bodies, and replies are never logged**, but for
the bounded token in a refusal reason. Every handled runtime or traced `/v1/turn` failure records a
closed-set OpenTelemetry `exception` event **without dynamic content**. DMs, other channels, bot
messages, self messages, duplicates, and messages without a summon are rejected before turn logging.

## Joined traces

Discord delivery starts with `discord.receive`; private HTTP delivery starts with a `POST /v1/turn`
server span that **extracts an incoming W3C `traceparent`**. Both parent the same `community.turn` span,
which joins `community.input` and `community.history`, `context.assemble`, `mcp.tools.list` and one
`mcp.tool.call` per call, one `model.chat` per Agent Proxy round, `response.validate`, and
`community.reply` plus `discord.reply`. The Agent Proxy and MCP HTTP transports inherit the active trace
context, and **Agent Proxy installs its FastAPI server instrumentation before startup, so its request
span and the LiteLLM spans remain in this same trace**.

The Discord boundary spans carry permalink-safe correlation attributes **without message content**:
`messaging.system`, operation name and type, and string-valued `discord.guild.id` and
`discord.channel.id`, with `discord.receive` recording the inbound snowflake as `messaging.message.id`
and `discord.reply` the outbound one. **These identifiers are trace attributes only, not metric labels
or new log fields.** The turn instruments are `sirens_echo.turns`, `sirens_echo.turn.duration`,
`sirens_echo.model.calls`, `sirens_echo.tool.calls`, and `sirens_echo.failures`, with low-cardinality
outcome and stage attributes distinguishing failures. **The OTLP exporter uses delta temporality so
SigNoz counts the first sparse event**, while gauges stay non-delta.

## Health signals

Echo separates process liveness, structural route readiness, and evidence of successful inference,
**each answering a different operational question**. `GET /healthz` returns `{"ok":true}`, inspecting no
dependency, **so a downstream outage cannot make Kubernetes restart a healthy Echo process**. `GET
/readyz` checks the exact `<namespace>/<alias>` route configured in `AGENT_PROXY_MODEL` by calling Agent
Proxy's corresponding endpoint with an uninstrumented client and a five-second ceiling, returning only
`{"status":"ready"}` or `{"status":"not_ready"}` with `503` and **relaying no downstream body, error,
URL, host, credential, check name, or physical model**. Unknown routes, timeouts, malformed replies,
dependency failures, and route-not-ready replies **all fail closed**, and the request never calls chat,
completions, generation, embeddings, or MCP.

**Both health handlers bypass the traced `/v1/turn` wrapper** and readiness uses an uninstrumented
transport, so health requests create no log, span, turn trace, model-call metric, retained body, or
model request. Their metrics are `sirens_echo.health.requests` with fixed `endpoint` and `outcome`,
`sirens_echo.readiness.duration`, `sirens_echo.readiness.state`, and
`sirens_echo.readiness.last_success`. **Labels never contain raw URLs, error text, request identifiers,
member data, physical models, credentials, or other unbounded values.** **Liveness proves the process
can answer locally; route readiness proves the configured inference path is structurally available
without consuming a model turn or extending VRAM residency**, and passive successful turn metrics remain
the evidence that model loading, GPU execution, and a valid completion recently worked.

## A positive signal, so silence means something

Echo emitted nothing between 06:00 and 07:36 one night, and **three explanations fit that evidence
equally**: nobody messaged it, Discord ingress stopped, or something upstream of the first log line
failed. All three produce identical telemetry, which is none. **Absence of logs is not evidence. It is
indistinguishable from absence of work.**

So one record a minute while the gateway session is open, **with counts since the last beat rather than
totals**, so it reads as a rate and needs no baseline. The record existing separates a live process from
a stopped one; `messages_observed` separates ingress arriving from ingress stopped; `turns_admitted`
separates messages that summoned from messages that did not; and `replies_sent` separates turns that
answered from turns that failed. **Observed is counted before eligibility**, because every message being
ineligible and no message arriving are different failures with the same downstream shape.

## Settling an anomaly

**Human recollection is the lead. Traces and configuration are the settlement. Model narration is
neither, and it is the trail most likely to be followed.** Treat a report as the strongest available
signal that something is wrong, **and as no evidence at all about what**. Collect the recollection while
it is fresh, without correcting it against a log yet. Settle it from traces, logs, and configuration -
**values a process emitted or was started with, not summaries of them**. Write the boundary last.

**A person notices the event that does not fit**: an anomaly is by definition out of distribution, and
noticing out-of-distribution things is the thing people do better than a model. A model summarising an
event it took part in produces a fluent, plausible account with the anomaly smoothed away, **because
smoothing is what next-token prediction is good at**. **The narration reads most confident exactly where
it is least reliable**, so it is not weaker evidence than a trace: in this one case it is actively
misleading, and it arrives already shaped like a conclusion. **Use it as a symptom to explain, never as
an account to check the traces against.** **Your own account is an account** - an investigating agent
produces one throughout, and three from one night were each a real command run correctly against a
question adjacent to the one asked. **Name what you could not check**, because saying so stops a reader
treating an unchecked thing as a checked one.

**The worked example**, 2026-08-13 (#258). Kai reported that Sirens Deep was down: correct that
something was wrong, and wrong about what, since the service was answering. The service's own account
was `model backend unavailable, retry shortly`, which **taken as the explanation sends an operator to
the inference tier to debug a backend that was healthy**. That is the expensive shape: the account was
not a lie and not a guess, it was **the service reporting what it had been told to report, and the
report was written by someone who assumed that failure class**. Ops reconstructed nine model request and
response pairs, every one `status: 200`, then a turn failure emitted 74 to 93 microseconds after the
last good response. **A sub-100 microsecond gap is in-process handling**: it cannot be a network call, a
timeout, or an upstream outage, **and that single number settles the question without needing to know
what the internal cause was**. The turn had spent its tool-round budget and the notice was reporting an
internal limit as an external failure. **"Down" was the correct alarm and the wrong diagnosis.**
