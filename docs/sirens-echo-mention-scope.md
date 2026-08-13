# Mention scope

Which runs of a reply a display name is read in. Who can be reached at all, and
what happens once they are, is in [mentions](sirens-echo-mentions.md).

## Prose only

**Resolution runs on prose only.** A link, a code span and Discord markup are
carried through byte-identical, because their contents are an address, a
command or an id rather than a person. A member called `eco`, `wiki` or `main`
cannot corrupt a URL or a tool name by being in the room. Bold, italic and
quoted text is prose, so a name inside one is still that person.

The non-prose runs are named rather than prose being named, because the miss
costs differ. An unlisted markup kind gets rewritten and someone reports it. An
unlisted prose kind silently stops reaching people, and nobody reports that.

**A name inside a dotted identifier is not that person either.** A host written
without a scheme is not a link by that shared definition, so the spans do not
cover it and a second rule does. A name immediately preceded by a dot, or
followed by a dot and then a letter or digit, is a label rather than a person.
A trailing dot before a space or the end of the reply is the punctuation of a
sentence, so a name that ends one still resolves. See sirens-echo#481.

Resolution reads every occurrence in a span rather than the first, because the
first can be a hostname label while the person is named later in the sentence.

## Why the non-prose runs are the ones enumerated

Three constructs were added one at a time, each after a member saw a mangled
artefact: links, Discord markup, then code spans. The alternative shape is to
enumerate prose instead and read names only there.

That trade is decided by how each one fails. An exclusion list fails loudly: an
unlisted construct is rewritten, someone sees a broken link or a tool name that
became a person, and it gets reported. An allowlist fails silently: an unlisted
construct stops resolving, and a member who should have been reached simply is
not. Nobody reports a notification that never arrived.

The sets are also different sizes. What carries a payload that is not a person
is short and nearly closed. What carries prose is not: bold, italic, underline,
strikethrough, spoilers, blockquotes, headers, list items, and plain text all
hold real names.

## Considered and not excluded

Discord timestamps, written `<t:1699999999:R>`, are not covered. The payload is
digits and a single letter, and the roster refuses a name under three runes, so
no name can collide with one. A guard that cannot fire is a guard that only ever
passes, which the battery doctrine rejects for the same reason.

## See also

- [mentions](sirens-echo-mentions.md) - the roster and the delivery decision.
