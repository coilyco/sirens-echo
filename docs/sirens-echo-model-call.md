# The model call

Reading the stream, retrying it, and the budgets it spends against.

## Two bounds instead of one

A turn that was queued and a turn that was hung produced the same thing from here: silence, then a
deadline. **A total deadline fires on schedule however many bytes arrived, so it cannot tell those apart
by construction.** So the call carries two bounds: the turn context is the ceiling, unchanged, and
`SIRENS_ECHO_MODEL_IDLE_TIMEOUT` bounds **silence** rather than the call, reset by any line, and a
heartbeat is a line. **`http.Client.Timeout` is deliberately unset**, being a total timeout that would
have cut a streaming completion on schedule and reintroduced the defect one layer down.

`stream: true` is required, because heartbeats have nowhere to go on a non-streaming request. Agent
Proxy emits them as SSE comments carrying `state`, `n`, `of`, `backend`, and `regime`, and **a keepalive
repeats the current state, so a state that persists keeps the connection provably alive without a
transition**. Only `attempt` is logged, as `model.attempt`, because a ten-second beat under a
five-minute ceiling would be thirty log lines saying nothing changed; the count reaches `model.response`
as `heartbeats`. **An unparsable comment is skipped rather than fatal.**

Content and reasoning concatenate. Tool calls arrive as fragments keyed by `index`, joined per call and
returned **in the order their indexes first appeared** rather than sorted. **Reasoning stays unnamed
when the stream carried none**, because an absent field and an empty one are echoed back differently on
the next round. A backend answering a streaming request with a whole JSON body is read as one, selected
on `Content-Type`, and no heartbeats reach that path so the turn ceiling is its only bound.

**The two failures are different sentences.** Nothing arriving at all is `ErrModelSilent` and reads
`model backend went quiet, retry shortly`; still working at the ceiling is `context.DeadlineExceeded`
and reads `turn timed out, retry shortly`. **The split is the point**: retrying silence can work, while
retrying a turn that ran the ceiling out while making progress reproduces it (#449). `failureCause`
reports `model_silent` against `timeout`.

## Retrying

**Only failures that arrive fast are retried**, and that bound comes from the turn ceiling rather than
from taste. `modelRetryAttempts = 4` with a 250ms backoff doubling, so 1.75 seconds of waiting at most
against a 180 second turn. `defaultRequestTimeout` bounds the whole turn at 180 seconds and Echo's p99
already sits there, so **a retry ladder built for slow failures would convert a failure the member could
have been told about into a timeout they wait out**. A connection refused or a 503 returns in
milliseconds.

**Availability is retried and nothing else**: transport errors, where no server was reached, and 429,
502, 503, 504. **4xx other than 429 is not**, because a 400 is the request being wrong and retrying
produces the same 400 four times, and that case is real since a prompt-trimming defect upstream returns
400. A cancelled or expired turn is not, because the budget is already gone.

**Agent Proxy retries independently, twice, at 600 seconds each**, so a turn Echo gave up on at 180
seconds leaves a request running for twenty minutes. That is not this ladder, but the two multiply.

**A refusal is not an outage.** A 4xx means the backend answered and refused the request this service
built, and retrying rebuilds the same request, so `rejectedByModel` classifies it and the member is told
the harness built something the model refused. `429` and `408` are excluded, being the two a 4xx can
mean that waiting does fix. This mattered under #875, where a malformed message array drew a DeepSeek
400 on every attempt and the member was told to retry shortly: **the advice could not work, because the
same array was rebuilt each time.** The cause is `model_rejected` rather than `stage_failed`.

## The model group is not the deployment

A rate's `model` field names a proxy model **group**, and a group routes to a backend that can change or
fail without the group name changing. **So two runs sharing a `model` value are comparable to each
other, and neither is automatically a statement about a lane.** Echo's deployed group
`sirens-echo/default` routed to a backend that failed for the whole of 2026-08-13, confirmed by probe:
no answer in 90 seconds against 1.5 for another group on the same proxy (#324). Every Echo number from
that window came from a different group, **and the dataset says so in `model`**. Two cases re-measured
on `sirens-echo/deepseek` scored 10/10 both times, with median reply length moving 36 words to 53 and 19
to 21: **the pass rates agree and the verbosity does not**, so a check scoring presence survives a group
change while **a rate carrying `max_reply_words` compares only to another rate on the same group**.

## Completion budget

A turn that calls tools re-injects their results into the next request, which inflates the prompt: four
parallel Eco calls once took a 6k prompt past 47k. A reasoning model then spent the whole fixed
completion budget on `reasoning_content` and returned empty content with `finish_reason: length`, and
the single repair hit the same wall. **That was 20 percent of turns, worst on ordinary member questions,
because the trigger is not difficulty but whether the turn reaches for a tool.** Nothing upstream
reported it: **a truncated completion is a valid response at the transport layer**, so Agent Proxy and
LiteLLM both logged `outcome=ok`.

**Tool results are capped before re-injection.** Only the copy re-entering the prompt is bounded and the
model sees a truncation marker, while the full result is kept for grounding validation, **so a bounded
result cannot make the runtime accept an action claim it should reject**. **The budget escalates rather
than repeating**: truncated *and* empty raises from 1800 up to a cap of 3600, and a raise that cannot
raise is exhaustion, while truncated output that is not empty is a usable answer. **Both bounds are
needed**, because raising the budget alone treats the symptom and bounding results alone leaves any turn
that still overruns failing at a fixed wall.

**Tokens are one budget and the clock is another.** Each MCP phase carries its own ceiling because each
fails differently: ten seconds to connect, fifteen to list, forty-five for one tool call. **The call
bound is the one that had been missing** - without it a server that never answered spent the whole turn
budget and left nothing to report the failure with, reaching the member as silence. Six tool rounds
still fit inside the turn budget, so the turn timeout stays the outer bound.

## A turn that thought until it had no room to answer

A reasoning model can spend its whole completion budget on `reasoning_content` and return empty
`content` with `finish_reason: length`. **What it is not is a backend failure**: every model call
returned 200, and one capture showed the reply drafted in full inside the reasoning, cut mid-word. Left
unmarked it lands in `stage_failed` whose stage is `model`, **so the member reads `model backend
unavailable, retry shortly`** - sending an operator to a working backend and telling a member to retry a
question that will fail the same way. `ErrBudgetExhausted` marks it, `budget_spent` counts it, and the
member reads `ran out of room to answer, ask for something narrower`. The same empty shape with no
finish reason reached this unmarked, past a stream check now removed (sirens-echo#8071);
`model.response` logs both.

`rounds_spent` and `budget_spent` both mean a ceiling this service chose ended the turn, and **sharing
one value would be the same collapse one level down**: `tool_rounds` against `max_completion_tokens`.
**It does not stop the turn failing, and it should not** - the change is what the red says. Bounding
deliberation stays with #367, a spend decision rather than a plumbing gap.
