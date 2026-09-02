# The worklog element

A long turn shows what it is doing, one row per tool call, resolving in place:

```
Working on it
> ✅ `eco.get_market`
> 📖 ✅ `astronomy`
> ❌ `teable.search_issues`
> 🔨 `eco.find_trade`
4 tools, 14 seconds elapsed
```

## The surface

The embed needs `EMBED_LINKS`, and the permission decides the surface: granted gives the embed, absent
gives stacked notice lines. The permission is read rather than assumed, and **a channel that reads as
unknown is treated as granted**, because a direct message always reads that way. A refusal degrades and
never fails: Discord answering `50013` routes the turn to notice lines and latches, so a channel without
the grant pays one rejected call rather than one per beat.

## Rows

The glyphs are the reaction vocabulary, shared with the reactions and the disclosure footer rather than
invented here: 🔨 invoked, ✅ ok, 📭 empty, ❌ failed. An unresolved row on a stopped element keeps 🔨,
meaning invoked with the outcome never learned.

**Every row is a harness notice**, the same shape a member reads outside an embed. The notice alphabet
carries the underscore, because the tool name is the payload and `list_issue` sanitized to `list issue`
is a name nobody can look up. The backtick is still stripped. Rows cap at twelve with a count standing
in for the rest, and **arguments are never shown**: names are roster-derived, arguments carry member
text. The one carve-out is a resolved skill read, whose row names the reference behind a 📖: the name
comes from the session's validated closed set, never from the argument, and a refused read names
nothing.

`ExecutedTool.Label` is the one spelling the row and the disclosure footer both take, so a call watched
running is findable afterwards. Neither surface filters anything, and the receipt's collapsed runs state
their count.

## Resolution

A delivered answer **deletes** the element, because the reply carries the disclosure footer and two
lists of the same tools is one too many. Anything else resolves to `Did not finish` and stays. **A block
is not tellable from any other stop**: one wording, no reason carried, and nothing in the view takes a
category, so a progress surface can never name the classifier.

The element does not become the answer. Delivery does not route through the progress message, which
would bypass the overflow-attachment path and the thread routing.

The threshold is 5 seconds. `turnProgressAfter` is an operator knob with the beat and the thread
threshold derived from it, so moving it moves all three.

## Attribution

Reading the record of any effect answers "who asked for this" without inference. **Attribution is
evidence, authority is enforcement.** A system can have either without the other: missing authority lets
the wrong thing happen, missing attribution means nobody can find out that it did. This records who
asked and decides nothing.

The recorded principal is the **Discord user ID**, stable and opaque. A handle is readable and mutable,
so it is display only and never the recorded value.

**Attribution does not reach telemetry as a span attribute.** The job id is promoted on spans and log
rows and resolves to a principal through the record, which keeps a user identifier out of the telemetry
store: telemetry is exported, retained on someone else's schedule, and read by more things than the
record is, while `sirens_echo.job.id` means nothing on its own and joins only where the access question
already lives. A test asserts that neither a span nor a log row carries the principal.

**Retention is unanswered**, and is a policy question rather than an engineering one. It needs an answer
before the guild widens beyond a two-party channel.

`AttributeJob` returns the requester from the record. `AttributeEffects` resolves a job's applied effects
to the principal that caused them, with the step, its detail, and the terminal state. `ListByPrincipal`
lists a principal's jobs. **No production path calls the first two**, and `AttributeEffects` reads
`job.Effects`, which only `RecordEffect` writes and nothing calls, so that map is empty for every job
that has run.

Attribution survives the job: one that failed or was cancelled still names its requester and still
appears in that principal's listing. It does not decide what a principal may cause, and it puts no
message content anywhere.

## The consult gate

`autonomy/async-consult` decides how `cli-guard` dispatches an issue and what a human sees when they
look for work that needs them. It is one of an exclusive `autonomy/*` set, so applying it removes
`autonomy/headless`, and a command line has to say the whole label or it silently applies nothing.

The label is written once and nothing reconciles it against the thread afterwards, so it drifts two
ways. **An answer arrives and the label stays**, so the work looks blocked and nobody takes it. **A
question is asked in prose and no label is written**, so the work looks free and nobody answers it,
which is the worse direction: unlabelled is fail-closed for dispatch and invisible to the human, leaving
the issue both undispatched and unqueued.

**Recording a decision removes the label. Asking a human a question adds it.** You are already writing
to the issue at that moment, so each costs one call, and a comment ending with a question for a director
or an operator is a consult issue by definition.
