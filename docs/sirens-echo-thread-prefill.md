# Prefilling a whole thread

Inside a thread, the thread is the conversation. A window into it is the wrong
unit, so the prefill carries all of it up to a bound.

## What changed

Outside a thread the prefill is unchanged: `MaxContextMessages` messages before
the current one, one API call. Inside a thread the prefill pages back to the
start of the thread instead.

`MaxContextMessages` is a window into a channel that never ends. A thread does
end, and it is the whole exchange a member is asking inside.

## No per-channel toggle

The design pass specified one, defaulting off. Kai removed it on
sirens-echo#769: this is not a per-channel thing. So the behaviour is
unconditional wherever the turn is inside a thread.

That means it ships on everywhere at once rather than behind a staged rollout,
which puts the whole weight of the context risk on the bound below.

## Two bounds, because they fail differently

**`threadPrefillRunes`** bounds what reaches the model. Six tool rounds already
admit more than this, so it is in family with what a turn carries today.

**`threadPrefillPages`** bounds the Discord round trips. A thread longer than
ten pages stops being walked, so one enormous thread cannot spend the rate
limit that every other channel shares. A short page means the start of the
thread was reached, which is the ordinary exit.

A page that errors fails the prefill rather than returning what it had. Half a
thread presented as a whole one is the failure this feature is against.

## Oldest first, and said out loud

Over the bound, the oldest messages go first. The newest exchange is the one
the member is actually in.

The reply carries `[thread context truncated, N older messages dropped]`
whenever anything was dropped. Silent truncation was rejected by name: a wrong
answer caused by missing context would look identical to any other wrong
answer, and this makes it attributable.

The notice reserves its room before the answer is fitted, for the same reason
the tool-disclosure footer does. A notice that vanishes under load reads as
context that was never missing.

Also rejected: falling back to the partial window, which would make the feature
quietly stop applying in exactly the long threads it was built for, and
summarising the older half, which adds a model call and a failure mode to every
turn.

## What a turn without threads sees

`droppedPrefill` reads an optional capability. The HTTP and MCP turns do not
implement it, report nothing dropped, and render byte-identical replies.

## See also

- [threads](sirens-echo-threads.md) - when a thread is created at all.
- [multiple contexts](sirens-echo-contexts.md) - what a context means here.
