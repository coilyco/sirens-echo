# Mentions

Naming someone in a reply reaches them. Only people already in the conversation
can be reached, and the harness decides who, never the model.

## Two separate things

**The candidate set** is who could be reached. It is built from the turn's own
payloads: the author of each message in history, and anyone those messages
already mentioned. Both are member-authored.

**The delivery decision** is who is reached. The harness resolves a name to an
account and passes that account explicitly.

Keeping these apart is the whole safety property. The reply's allowed mentions
carry an empty parse list and an explicit set of account ids, so a mention is
something the harness decided to deliver rather than something the model wrote.
Parsing the reply for names would let a model be talked into reaching everyone,
which is what the empty parse list has always prevented.

## The candidate set is member-influenced

A member who names someone in their own message puts that person in the
candidate set. That is correct: reaching the person a conversation is about is
the point, and excluding anyone a member named would exclude most of the people
worth reaching. The delivery decision stays the harness's either way.

It matters when the roster widens. Drawing candidates from guild members or
roles is a change of source rather than of shape, and it would widen a set that
member text already influences. Read this section before doing that.

## Four bounds

**A short name never resolves.** Three runes is the floor, because a one or two
character display name matches too much ordinary prose.

**A name inside a longer word is not that person.** Matching requires a word
boundary and a preceding character that is not part of an existing mention.

**Longest name first.** A display name containing another is matched before its
substring, so the more specific person wins.

**Once per person.** A reply naming someone four times reaches them once. Four
pings for one sentence is what makes people mute a channel.

An existing mention is left alone, and the first message to claim a name keeps
it, so the oldest message in the turn decides an ambiguous one.

## Where it does not apply

The HTTP path has no Discord identities to resolve, so its roster is empty and
`resolveMentions` returns the reply untouched.

## See also

See [notices](sirens-echo-notices.md) for the other harness-authored additions
to a reply, and [reactions](sirens-echo-reactions.md) for the status surface
that shares this one's vocabulary.
