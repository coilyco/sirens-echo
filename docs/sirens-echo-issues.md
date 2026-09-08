# Knowledge gaps and corrections

How Sirens Echo turns an unanswered question or a correction into a tracked issue. **A definition
without `issue_tracker` does none of this** and must return `issue: null`, stating uncertainty in the
reply.

For a definition with `issue_tracker`, an unanswered question produces a `knowledge-gap` draft and an
explicit correction a `correction` draft, **values affecting only the title prefix**. The runtime removes
Discord links, mention syntax, and long identifiers from drafts, and requires a summary without member
identity, handles, raw quotes, secrets, or personal details. The reporter calls the guarded tracker MCP
to reuse an exact-title open issue or create an ordinary one.

## Records in, issues out

The tracker is a Teable base, so its MCP publishes generic record verbs against a table id. **An issue
there is a header plus a comment thread, and carries no body field by design**: the text goes in the
first comment, which is the house pattern for this tracker rather than a gap in it. Handing those verbs
to the model would put decisions in the prompt that belong to the harness, and would be wrong in a way
prose cannot fix, because **a model that made only the first of the two writes has filed a titled issue
nobody can read.**

So `internal/community/trackeradapter.go` hides the record verbs and offers `create_issue`,
`search_issues`, `comment_issue`, and `close_issue` instead, composing each from the calls the guardfile
grants. **The record verbs are removed rather than left alongside**, because two ways to file an issue
means the unguarded one gets used. **The bound is still the guardfile**: the adapter reaches no verb the
tracker MCP did not publish, and there is no tracker credential in Echo.

## A 2xx is not proof

This API reports success without doing the thing in five confirmed ways, and the deploy surface states
that `create_record` and `edit_record` have no read-back. A declarative guardfile cannot read back. The
adapter can:

**A filing writes the issue row, reads it back, and only then writes the first comment.** A read-back
that finds nothing, or a row whose title is not the one just written, returns a tool result saying it is
unconfirmed and telling the model not to report one, and **the body is not written against a row that
was never confirmed**. A close is read back the same way, and a row that still reads open is reported as
unconfirmed. The order matters: **a row with no body is recoverable by a reader who can still see the
title, and a body with no row is not.**

## Search reads a window, not the tracker

The MCP's `filter`, `filterByTql`, `search`, and `projection` parameters are unreachable from here: they
want array-shaped values the MCP sends as strings, and `{` is refused by the argument policy. **A view is
the filter instead.** `SIRENS_ECHO_TRACKER_OPEN_VIEW` names a view whose own filter bounds the read,
`SIRENS_ECHO_TRACKER_SEARCH_ROWS` caps how much of it is read, and the match happens in process.

**The result carries that bound**, so a reply built on it cannot turn an empty window into "no such issue
exists". The prompt's coverage policy makes the model carry it through.

## Naming what a turn observed or filed

**A short reference such as `#233` resolves against no repository once it leaves the channel, and an
issue filed without being mentioned leaves no trace at all.** Prose prompting has not stopped either
habit, so the harness appends the references: after the response checks pass, a `Referenced issues:`
block carries the canonical key for a short-form reference whose number a tool result **in this same
turn** returned, and for any issue this turn filed, whether or not the reply named it. **Every appended
key came back from a tool call**, and a number the turn never observed stays unnamed rather than guessed.

**The key is the reference, and there is no link.** The tracker is reachable on the tailnet only, so
`coilyco-gaming/sirens-echo#233` is the durable form. This is a loss for the member, who could previously
follow a Forgejo URL, and it is the honest state: a link to a host they cannot reach is worse than a key
they can quote.

The block is service-authored and added after validation, so it carries no first person, no exclamation,
and no emoji beyond an object's own. **A long answer is shortened to make room for it rather than the
block being dropped**, it resolves against the answer that will actually be sent, and a block that still
cannot fit whole is dropped rather than truncated into a broken key.

**One number can name an issue in two repositories**, and a search returns keys from several, so **a
number observed with two different keys is suppressed**: on a collision it is the number-to-key
association that would be the guess. The filed issue is read from the key the filing tool confirmed, **so
a key quoted inside a new issue's body is not mistaken for the issue just filed**.

## What a member's ticket has to clear

**The shape to catch is not abuse.** #907 was polite, on topic, well formed, and produced a tracker entry
with nothing to act on (#852). A member-originated filing passes two model checks first, each answering
from a closed list: **validity** refuses `placeholder` and `unclear`, **scope** refuses `out-of-scope`. A
refusal returns as a tool result saying what would fix it. **A failed checker files anyway**, matching the
content gate, and the principal is exempt. **The checks run before the first write**, so a refused filing
leaves no row behind for a later reader to triage.

## The field values a filed issue carries

The model supplies a title and a body; **everything else on the row is the harness's, and the model
cannot omit it.**

**This service files in response to member input**, so the body contains text a member influenced, and
the issue lands in a tracker several agents read and act on: **without a marker, attacker-influenceable
content is indistinguishable from work an agent authored.** The marker is the `⚠️ SANDBOX ⚠️` value on
the `autonomy` field, and it is **not configurable**: a deployment may decline to file at all by naming
no tracker, but it may not file while calling the contents verified.

`org` and `repo` replace the move-to-repo label, and the deployment names them. `priority` and `roles`
fill the remaining notNull fields. **Each is independently optional**, and an unset one leaves the
tracker's own default rather than writing an empty select, **so a missing knob is a tracker default
rather than a wrong value.**

The values are **set rather than merged**: **a model that could name its own destination could route its
own issue away from the people who read this tracker.** A filing call carrying `repo`, `org`, `autonomy`,
`priority`, or `state` has them discarded, none being arguments the tool declares.

## The issue tracker surface

Deploy fixes the tracker MCP to the issue-tracker base, **the token's scope being the only base bound**:
`pin` fixes query values and `baseId`/`tableId` are path parameters, so the guardfile cannot constrain
which base is reached. The surface is records only and mounts no field write, because that instance
applies field writes unreliably. A schema change is made in the Teable UI on the tailnet and read back,
never through this service.

Naming that server as `issue_tracker` selects the prompt's filing policy and the adapter both. Both lanes
name one now, mounting a single MCP between them. **The roster answers what is reachable, the tracker
name answers what is encouraged, and the adapter answers what shape it arrives in.**
