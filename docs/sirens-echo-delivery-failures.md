# Why a ready reply did not land

A turn that fails after the reply is composed has already spent its completions
and its MCP calls. The work was done and paid for, then dropped at the last
step. That is worse than failing early, and it is invisible to every instrument
that watches the model.

## The event recorded that it happened and discarded why

`discord.turn.failed` carried one field, `error_type: turn_failed`. Rate
limiting, a message over the length limit, a missing permission, and a dropped
gateway all looked identical, so a count could be produced and a cause could
not.

`discord.reply.failed` now records the send itself:

| field | separates |
| --- | --- |
| `discord_failure` | whether Discord answered at all |
| `discord_status` | rate limiting, permissions, length |
| `discord_code` | two failures sharing one status |
| `reply_bytes` | the length case, which status alone does not prove |

`no_response` is a classification rather than a gap. A dropped gateway produces
no HTTP exchange, and knowing Discord never answered is itself the diagnosis.
`abandoned` is separate: our own budget ending a send is not an outage.

## What is deliberately absent

No channel, no member, no reply body. The telemetry contract is metadata and
byte counts, and no identifier reaches a label. Status and code separate every
candidate cause without any of them, so widening the contract was not necessary
and was not done quietly.

## A stage is not a cause

`error_type` is the stage, so a timeout and a backend outage at the model stage
collapse into one value while showing a member two different notices. Both were
countable through the notice string, which is prose rather than a label, so
they could be counted after the fact and never alerted on.

`failure_cause` is a closed set: `shutdown`, `timeout`, `tool_failed`,
`rounds_spent`, `reply_refused`, `budget_spent`, `stage_failed`, derived in the
same order the notice is chosen so the label and the phrase a member reads
cannot disagree. See [budget exhaustion](sirens-echo-budget-exhaustion.md).

## The member is told, once

A composed reply that fails to send used to end the turn silently, which is
indistinguishable from being ignored. The turn now marks itself failed and
attempts one short notice, `reply could not be delivered, retry shortly`.

One attempt, never a retry. A loop would turn one dropped reply into a flood
against the transport that just refused. The second send is worth trying rather
than assumed futile because the failure classes differ in size: a reply refused
for length succeeds as a short notice, and a permissions failure costs one call
and fails again. Losing that call beats a member concluding they were ignored.

## When the notice itself cannot be sent

A member who has waited long enough already has an acknowledgement in the
channel: the progress line. Deleting it after a notice fails to send ends the
turn with less than they had, and dead air is the worst outcome this service has.

So a notice that cannot be sent is carried by that line instead. An edit is a
different call against a message that already exists, so it can land where the
send did not, and a claimed line is never deleted even when the edit fails too.
A turn too short to have posted a line has nothing to carry and is unchanged.

## What this does not do

It does not reduce the failure rate. It records what a failure was, so the rate
can be attributed to a cause and then fixed or accepted with a reason. That
needs a window of live telemetry from a build carrying these fields.

## See also

* [Notices](sirens-echo-notices.md) - the phrases a member reads.
* [The turn verdict](sirens-echo-turn-verdict.md) - which event to count.
* [Observability](sirens-echo-observability.md) - reading these events live.
