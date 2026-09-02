# Tool-call markup and disclosure

Two things a reply must not carry, one it must, and the durable record of what ran.

## Tool-call markup

`forbid_tool_call_markup` rejects a reply carrying the model's own tool-call delimiters as content.
**A member reads that verbatim, and no other check in the family can see it.** Opt-in per case (#301).
The first live Deep gate run scored a reply as a pass whose body was `<｜｜DSML｜｜tool_calls>` markup,
because the case's only forbidden pattern was `https?://`: the model wanted a tool the roster did not
carry and emitted the call as prose. **A partial roster is a live condition and nothing strips this.**

The target is the delimiter syntax, not the words, so prose about tool calls and a quoted JSON
`"tool_calls"` field stay clean. **The name set was closed around the wrong thing**: a probe of 5 live
turns asking Deep to file an issue emitted markup in 4 and caught none, because `<create_issue> {...}`
and `<tool_round> {...}` were uncaught. Two patterns were then added, a tag carrying a `name="..."`
attribute and the observed `mm_tool_calls` and `tool_round` wrappers, **validated against 396 replies
persisted in `evaluations/` with zero false positives**. Echo's `ornith:35b` form is unverified (#324).

It is opt-in because the rate depends on the request, 1 of 5 for a case asking for no action and 4 of 5
when it asks for one, and **an always-on check would turn the gate flaky on a non-security behaviour**.
It runs last in `runScopedChecks`, leaving precedence unchanged. **Every rate case sets it, and a test
refuses one that does not**: a rate pack gates nothing, and omitting it buys a rate computed over
replies that were never answers, the case that found this reporting 10 of 10 while nine were markup.

## Tool-name markup

`toolNameMarkupFailures` matches the case's `required_tool`, qualified and bare, so `<create_issue>` is a
finding when the case declares `teable__create_issue`. **The delimiter set is a closed list of names
taken from published formats, and the model does not use those: it builds the tag from the tool's own
name.** A tool name is a value from configuration rather than a word from a vocabulary, which is why
`checkPrincipalEcho` survives translation while English word lists do not (#253). On the live English
and French action probes, the delimiter set alone caught 3 of 7 markup replies and the pair caught 6
with no false positives: **the two checks are complementary rather than overlapping**.

**It cannot reach the reply path, and that shape was chosen for that reason.**
`ValidateNoToolCallMarkup` takes only a reply, so it has no case and no declared tool. Widening the
shared delimiter set makes production refuse more, **and a refusal has no repair loop, so a leak becomes
a silence**. Two misses are accepted and pinned by test: a case declaring no `required_tool` is not
covered, including `no-invented-surface` where this defect was first found, and an aliased tool in tag
content escapes, because `<tool_uri> <tool>issue-create</tool>` puts the name in the *content*.

## Tool call disclosure

A reply that called tools carries a footer naming them. **It is the receipt a reader can see, where the
grounding check is a guard only the service can see.**

```
> 🔨 ✅ `eco.get_market`
> 🔨 📭 `eco.find_trade` — no results
> 🔨 ❌ `eco.get_stores`
```

| Glyph | Meaning |
| --- | --- |
| ✅ | the call returned data |
| 📭 | the call worked and returned nothing |
| ❌ | the call failed |

**Three states, because two would conflate**: a reply that reported an empty result as a confident zero
is the defect this distinction exists to prevent. The glyph is a scanning anchor rather than the
message, so an empty result also says `no results` in words.

A run of the same tool at the same status collapses to one line with a count. **Any other tool breaks
the run**, so `A A B A` renders three lines and the order the model worked in survives, and a status
change breaks it too, because a failure is never counted inside a run of successes.

* **No tools, no footer.** A refusal calls nothing and stays short.
* **Names only, never arguments.** Names come from the roster and are safe; arguments can carry member
  text, and reflecting that back would build a surface into the data-borne injection vector.
  The skill-read 📖 carve-out is in [the worklog doc](sirens-echo-worklog.md).
* **Service-authored**, appended after the reply checks rather than passed through them, alongside issue
  references, because the harness wrote it and the checks police what the model wrote.
* **Inside the send budget.** A transport with a ceiling declares it, and the answer is shortened to
  leave room rather than the footer being truncated away, because **a receipt that vanishes on a long
  reply reads as no tools ran**, exactly when the reply is long and tool-heavy.
* **A reference outranks the receipt.** Every service suffix is appended by one step that owns the whole
  budget, footer last, so at the ceiling the footer yields and the link a member can act on survives.
* **Both lanes disclose**, the reply path being shared.

**Outcome is recorded where the call completes, not inferred later**, because an empty result is
recoverable from the result text afterwards and a failure is not. A transport failure never reaches the
footer, because it ends the turn instead of returning a result.

## Mirroring the trajectory

Deep emits a durable record of what it called into Temporal Cloud. **Metadata only, and Temporal
observes the turn rather than running it** (decided with Kai on 2026-08-16): an activity per tool call
would put a round trip in front of a Discord member, and a Temporal retry policy stacked on the
agent-proxy fallback would compound a 502 into a slow expensive one.

`ToolCallRecord` has five named fields: server, tool, outcome, elapsed millis, trace id. **It is
deliberately not the span's attribute slice**, because `StartSpan` is a variadic passthrough, so copying
from it would mirror whatever any caller passed, sight unseen, forever: someone adds
`attribute.String("message.text", ...)` one day and member content starts flowing to a third-party SaaS
with no change here and nothing to notice it. Widening means adding a field to that struct, **a
disclosure decision someone makes on purpose**, and a test enumerates the fields and fails when the set
changes.

**Only `RecordToolCall` mirrors.** Agent Proxy logged 68,900 `http receive` spans against 4,300 real
requests in 30 days, so hooking span-start points a firehose at a service that bills per action. One
`SignalWithStartWorkflow` per tool call is one action either way, keyed on
`sirens-deep-trajectory-<trace id>`, **so a turn's calls arrive as one ordered trajectory instead of one
workflow each**. `ToolTrajectoryWorkflow` performs no activity and reaches nothing, and nothing polls
that queue, so **`TimedOut` is expected and history is the record**: 0 Completed is the design, not a
break (sirens-echo#930).

**Never on the turn's path.** The send is a non-blocking channel write and a full queue drops. A single
worker owns the only call into the mirror, with a hard timeout, on a context detached from the turn
**so a finished turn does not cancel its own audit record**. Errors are swallowed and a panicking client
recovered. Every one of those paths increments `sirens_echo.mirror.drops`, **so an outage is a number
rather than a silence**, because a mirror that fails quietly is the shape of #137 and #190.

It is off unless a deployment supplies all three of `SIRENS_ECHO_TEMPORAL_HOST`, `..._NAMESPACE`, and
`..._TASK_QUEUE`. **A half-filled connection fails at boot**, a typo that quietly turned the mirror off
being the silent failure the drop counter exists to prevent, while a dial failure is only logged:
**Temporal being unreachable must never stop this service answering**.
`SIRENS_ECHO_TEMPORAL_API_KEY` comes from the pod environment (#444). Deep only, chosen by which lane
sets the variables.
