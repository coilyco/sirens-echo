# Summons and mentions

What addresses this service, and what a reply may reach back to.

## What counts as a summon

Five things address this service in a guild: an explicit mention, a mention of a role this account
holds, a reply to one of its own messages, an edit that adds a mention to a message that did not have
one, and any message at all in a thread this service created. A direct message is addressed to it by
definition.

**A thread this service opened is its own conversation, so every message in it summons**, for the
thread's life and from any member, not only the one it was opened for. `Channel.OwnerID` is the signal,
answered from cached state, and a miss takes one lookup cached per channel rather than per message. A
thread a member opened keeps the mention gate.

**A role mention summons, because a member who @s the role is addressing what holds it.** Discord
delivers the mentioned role ids on the payload, and the account's own roles come from the member Discord
sends on `GuildCreate`, which arrives without the privileged members intent; a miss falls back to one
REST read written back to state, so a guild costs at most one lookup. **`@everyone` does not summon**:
its role id is the guild's and every member holds it, so an announcement would otherwise address every
agent in the channel. Nothing downstream changes, a role mention taking the same access policy,
allowlist, and admission budget a direct mention takes.

**A reply summons when the referenced message was authored by this service.** The Gateway payload
usually carries the referenced message already, so the common case costs no API call, and when the
payload carries only a reference id the runtime looks it up and compares the author, that lookup passing
through the admission gate **so a busy channel cannot turn replies into a stream of API calls**. A reply
aimed at another member is not a summon, and an unresolved reference is never guessed either way.

Discord delivers `MESSAGE_UPDATE` under the guild messages intent the runtime already requests, so an
edit involves no additional intent or permission and runs the same eligibility, exchange, access,
admission, and summon path a new message runs. Three gates come first.

* **A member edit, not a link preview.** Discord emits the same event when it resolves an embed, with no
  member involved. Only a member edit sets `edited_timestamp`, so an update without one is ignored.
  Without this gate **a message that already named the service would summon again every time Discord
  finished unfurling a link in it**.
* **Guild only.** An update payload is partial, and a missing guild id would otherwise read as a direct
  message, which summons without a mention and answers to a different policy.
* **An explicit mention.** A partial payload is not a sound basis for re-deriving a reply reference, so
  an edit summons on a mention alone.

**The duplicate gate keys on message id, and an edit keeps the id of the message it edited**, so a
message this service already answered stays answered once however many times it is edited afterwards.
Only a message it had no reason to answer before can become a summon, which is what "newly mentioned"
means here.

## Mentions

Naming someone in a reply reaches them. **Only people already in the conversation can be reached, and
the harness decides which, never the model.**

`Parse` is empty on every reply, so Discord parses no mentions at all and even a correctly formed
`<@id>` arrives as inert text. The allowance is an explicit list of ids the harness resolved, so a
mention is something the harness decided to deliver rather than something the model wrote, because
**parsing reply text would let a model be talked into pinging everyone**.

**The roster is the conversation.** A name resolves only if it belongs to someone already in the turn:
an author of a message in the history, the member who spoke, or someone one of those messages mentioned.
That needs no membership lookup and no API call, since all of it is already in the payloads the turn was
built from, and it is the narrowest of the plausible rosters. Widening to guild members or roles is a
change of source rather than a change of shape, and stays unbuilt because the narrow version answers the
case it was asked for.

Two of those three sources are member-authored, so **a member naming someone puts that person in the
candidate set**. That is correct, because reaching the person a conversation is about is the point, and
the two decisions stay separate: the candidate set is member-influenced, the delivery decision is not.
Anyone widening the roster is widening a set that member text already reaches into.

## What a mention will not do

**A name shorter than three characters never resolves**, because it matches too much ordinary prose to
be safe. **A name inside a longer word is not that person**, matching being bounded so `alphabet` does
not reach `alpha`. **A longer name wins over a shorter one it contains**, so a display name that happens
to start with another person's name resolves as itself. **A name a tool returned never resolves**, nor does any
other spelling of that account, because a store owner a tool quoted is not someone the harness should ping (COI-3283).

**A person is named in prose, and that is what bounds this.** A name resolves only where prose puts one:
after whitespace, an opening bracket, an emphasis mark, or the start of the reply. That one rule is what
keeps a name from matching inside a link, a code span, Discord markup, or a dotted identifier, and
**enumerating those kinds one at a time was losing**. The specific rules below remain as defence in
depth, and their corpora are this rule's regression suite.

A link, a code span, and Discord markup are carried through byte-identical, because their contents are
an address, a command, or an id rather than a person, so **a member called `eco`, `wiki`, or `main`
cannot corrupt a URL or a tool name by being in the room**. Bold, italic, and quoted text is prose, so a
name inside one is still that person. The non-prose runs are named rather than prose being named,
because the miss costs differ: an unlisted markup kind gets rewritten and someone reports it, while **an
unlisted prose kind silently stops reaching people, and nobody reports that**.

**A name inside a dotted identifier is not that person either.** A host written without a scheme is not
a link by the shared definition, so a second rule covers it: a name immediately preceded by a dot is a
label, so is a name whose following run of label characters arrives at a dot that begins another label,
and a trailing dot before a space or the end of the reply is sentence punctuation, so a name that ends
one still resolves.

**The rule walks forward rather than reading the adjacent character**, because the *first* label of a
host has nothing before it but a space, which is what prose looks like. In `eco-app.coilysiren.me` the
name `coilysiren` sits after a dot and `eco` does not, so reading adjacency alone made the same host
safe for one member and not for another. A hyphen joins labels, so the walk crosses one, and a hyphen
that arrives at no dot is an ordinary word: **a member named `eco` is still reached in `eco-friendly
builds`**, which is the case that keeps this from becoming "a name before a hyphen never resolves".
Resolution reads every occurrence in a span rather than the first, because the first can be a hostname
label while the person is named later in the sentence.

**Someone named four times is reached once**, because a reply that mentions a person repeatedly should
notify them, not ping them per sentence, and the remaining occurrences stay as the name and still read
correctly. **An existing mention is left alone**, so nothing nests.
