# Mentions

Naming someone in a reply reaches them. Only people already in the conversation
can be reached, and the harness decides which — never the model.

## What was wrong before

Every reply carried `Parse: []`, so Discord parsed no mentions at all. Even a
correctly formed `<@id>` arrived as inert text. Nothing anywhere resolved a
name to an account. The service could name people and could not reach them.

## Never parse

`Parse` stays empty. The allowance is an explicit list of ids the harness
resolved, so a mention is something the harness decided to deliver rather than
something the model wrote. Parsing reply text would let a model be talked into
pinging everyone, which is the failure this suppression was added for.

## The roster is the conversation

A name resolves only if it belongs to someone already in the turn: an author of
a message in the history, the member who spoke, or someone one of those
messages mentioned.

That needs no membership lookup and no API call, because all of it is already
in the payloads the turn was built from. It is also the narrowest of the
plausible rosters: the only people reachable are people in the room. Widening
to guild members or roles later is a change of source, not a change of shape.

Two of those three sources are member-authored, so a member naming someone puts
that person in the candidate set. That is correct, because reaching the person a
conversation is about is the point. The harness still decides whether to deliver
a mention, and those are separate: the candidate set is member-influenced, the
delivery decision is not. Anyone widening the roster is widening a set that
member text already reaches into.

## What it will not do

**A name shorter than three characters never resolves.** It matches too much
ordinary prose to be safe.

**A name inside a longer word is not that person.** Matching is bounded, so
`alphabet` does not reach `alpha`.

**Someone named four times is reached once.** A reply that mentions a person
repeatedly should notify them, not ping them per sentence. The remaining
occurrences stay as the name and still read correctly.

**A name is only read in prose.** A link, a code span, Discord markup and a
dotted identifier are not. See [mention scope](sirens-echo-mention-scope.md).

**An existing mention is left alone**, so nothing nests.

**A longer name wins over a shorter one it contains**, so a display name that
happens to start with another person's name resolves as itself.

## What is still open

Whether the roster should widen. Every guild member and named roles were both
considered and neither is built, because the narrow version needs no new data
and answers the case the issue was filed about: naming someone in a reply to
the conversation they are part of.

## See also

- [mention scope](sirens-echo-mention-scope.md) - which runs of a reply a name
  is read in.
- [notices](sirens-echo-notices.md) - the other thing the harness renders into
  a reply rather than letting the model write.
