# Telemetry beyond the turn

What the harness itself did, what a job did, what an evaluation run costs, and how to count discovery.

## Recording what the harness itself did

Workspace command execution emits a `job.command` span and `sirens_echo.commands`, counting
`command.verb` against an outcome. **The verb is the label**, this repository's own closed set, **and
arguments are not**, because a clone argument is a repository URL. Three outcomes: `ok` exited zero,
`exited` exited non-zero, and `did_not_run`. The exit code goes on the span rather than the metric,
since 0 to 255 is cardinality a closed label does not want. The span carries duration, the truncation
flag, and the job id, **and the output never reaches any of them**.

`exec.CommandContext` kills the direct child on expiry but does not close the pipe, and `CombinedOutput`
waits for every writer to it, a grandchild included, so a shell that forks rather than execs outlives its
own deadline. `command.WaitDelay = commandKillGrace` bounds the wait after the kill and closes the pipes.
**The orphan is not reaped**: this unblocks the harness rather than killing the process tree.

**None of this leaves the process.** The Temporal mirror that used to carry tool-call metadata outward
is gone, so a new `Record*` cannot widen an export path that no longer exists. A command verb is closer
to content than `mcp.tool.name` is, and that is the reason to keep it that way if one ever returns.

## Job-scoped telemetry

Service-scoped telemetry answers "how is the service doing". Once work is durable the question is **what
happened in that run**, and that needs a different key: traces carry `service.name` and logs carry
`k8s.namespace.name`, neither addressable by unit of work.

The job id rides the context, so it reaches both without every call site threading it. Spans carry
`sirens_echo.job.id` **not only on the root**: `StartSpan` adds the attribute whenever the context is
inside a job, so `model.chat`, `mcp.tool.call`, and everything else are queryable by id directly rather
than only by walking down from the root. Logs carry `job_id` as a row field.

**A span or a row outside a job carries no job id at all**, because an attribute that is sometimes
absent is more useful than one that is sometimes wrong. Given a job id and nothing else you get every
span, every log row, and the record itself, with service and namespace needed for none of the three.

## Evaluation telemetry is its own service

An evaluation run exports to the same receiver as the deployment and reports as `sirens-echo-eval`, and
**that separation is load-bearing**. One binary serves both profiles, so without it `eval-echo`,
`eval-deep`, `board-deep`, and `rate-deep` all report as the Echo deployment, and `service.name =
sirens-echo` becomes the production deployment plus every evaluation run of both profiles. Measured over
24 hours that was **898 spans against 169 real turns**, and every number taken from that service was
wrong in the same direction: error rate, latency, token spend, and cache-hit ratio all read as plausible.

An evaluation run opens no `community.turn`, so its `mcp.tools.list` spans are roots whose traces contain
only themselves. **The two evaluated profiles are not distinguished**, there being no lowercase slug on a
definition to derive one from.

## What a turn costs

`promptBudgets` ratchets the system prompt, and `worstCaseTurn` measures the turn context beside it. The
variable half is the conversation window plus the member's message: history author at 80 runes and
content at 1000 runes, both times `max_context_messages`, plus one current message at 2000 runes. At the
tracked window of 12 a worst-case turn assembles **15,248 bytes against an Echo system prompt of roughly
19,900**, so it is about 43% of the request.

**The fixed window should be the cheapest thing that makes ordinary continuity work**, because raising it
buys context for every turn including the self-contained ones that need none. A turn that genuinely needs
more reaches through backfill, where the cost falls only on the turns that ask.

`worstCaseTurn` feeds entries far larger than the caps, so it measures what the caps permit rather than
what a caller happened to send, and the window is read from the tracked definitions. A second check keeps
the truncation honest, because a cap that stopped truncating would not fail a ceiling on its own. It does
not measure token count, bytes being a proxy that is checkable offline with no model and no network, and
it does not measure tool results.

## Histogram boundaries a turn actually fits in

The OpenTelemetry SDK's default explicit boundaries top out at **10,000 ms**, and a turn's p50 sits
above that. So every turn landed in the overflow bucket, and a p50 or p99 over `sirens_echo.turn.duration`
returned the 10,000 boundary rather than a duration, **for every lane, on both sides of any comparison**.
That reads as a working metric holding steady, which is why it survived so long: **the failure of a
histogram whose range is wrong is a plausible number, not an empty one** (#976).

`sirens_echo.turn.duration` and `sirens_echo.coalesce.turn.duration` take boundaries running to 300,000
ms, the request ceiling, so a turn that ran the clock out is distinguishable from one that nearly did.
`sirens_echo.coalesce.batch.size` takes small-integer boundaries, because a batch is bounded by the wide
batch size and the default first bucket held every batch alike, **so a lane batching nothing read exactly
like a lane batching four**. Every other histogram keeps the defaults, its recorded range being one they
fit.

Boundaries are a view on the meter provider rather than a knob. A deployment that changed them would
make its own history unreadable against everyone else's, and a bucket edge is not a thing to tune under
pressure.

## Tool discovery telemetry

**The span is the lookup, not the round trip.** `mcp.tools.list` wraps the call site, so it is emitted
once per turn whether or not anything reached the network. Counting spans reports one listing per turn
even when the roster is served from memory.

So the span says which it was: `mcp.tools.configured`, `mcp.tools.reached` (how many went to the network
**including a failed connect**), `mcp.tools.listed`, `mcp.tools.unavailable`, `mcp.tools.registered`, and
`mcp.tools.cached`, true when the roster is non-empty and none went out. **Count round trips with
`reached`. Do not count spans, and do not read the duration.**

Reaching the network and completing a listing are different, **and the difference is the outage**: a
connect that fails is a round trip that listed nothing, and reporting it as cached would assert the
comfortable answer on the one turn that matters. `cached` is all-or-nothing over the roster, the cached
count being `configured - reached`, and **an empty roster reports `cached=false`**, because nothing was
served from cache when there is nothing to cache. `cached` alone reads the same for a warm roster and one
where every server is backing off, which is what `unavailable` and `registered` separate.

`mcp.server.discovery` wraps one server's round trips and carries `mcp.server.name`, with
`mcp.discovery.stage` moving through `connect`, `tools`, `resources`, and `prompts`, **so a failure's
stage is the span's last value**. Without it a rejection is an `HTTP POST` carrying a URL and nothing
else.

**A tool reporting its own failure carries error status**, or every generic error query misses it. The
type is `sirens_echo.mcp.tool_reported_error`, the class rather than the tool's own words, since a span
carries no bodies, and a deadline splits out as `tool_call_timed_out`. Neither fails the turn.

**`stage_failed` names where, not what**, so `turn.stage.failed` and the Sentry event carry
`error_summary`: a closed `reason`, an `http_status` if sent, and the Go type `chain`. Never error text.
