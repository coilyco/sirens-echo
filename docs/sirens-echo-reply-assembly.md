# Reply assembly

Some of a reply is written by the service rather than the model: the tool disclosure footer and the
issue reference block are both appended **after** the reply checks have run, because the harness wrote
them and the checks exist to police what the model wrote. **One step appends all of them, inside one
transport budget.**

**Two independent appends budget against each other.** The first is appended, the second inside the
remaining room, and the transport cuts the tail blindly, where both live, so the second silently
truncates the first. **The failure is invisible**, and it lands on the turn that did the most for the
member, because a turn that filed an issue and called tools carries both suffixes (#413). **The answer
is what yields**: both suffixes are service-authored, short, and bounded, and the answer is none of
those, so the answer is shortened rather than a suffix dropped. A transport with no ceiling, like the
HTTP turn, declares none and nothing is shortened.

**The append order is the preference order**, because a cut reaches the tail first: a reference is a
link a member can act on and the footer is a record of what ran, so the reference goes first and the
footer yields. When the answer has yielded everything and the suffixes still do not fit, the least
preferred is **dropped whole rather than cut into a half-rendered receipt**, and a reference block that
cannot fit whole is dropped entire, **because a truncated URL is worse than no URL**. **Assert on the
assembled string, not on a budget function**: a budget can be correct while the send is not.

## A reply too large for one message

Large content has three paths and two already refuse to destroy the remainder: an upload lands in the
scratchpad and the turn is told the path and true size, and a tool result over its cap is saved whole.
**Outbound was the exception**, cut at the send budget with everything past the cut gone. Now a reply
that does not fit is sent as a message plus the whole reply as `reply.txt`, the message carrying what
fits and one line naming the file and the byte count. The file name is fixed, because **nothing
member-supplied or model-supplied reaches an attachment name**. **The file is the complete reply and the
message is a prefix of it**: attaching only the remainder would make a reader stitch two pieces
together. The bytes are held in memory for the length of the send, so no quota is spent.

**The cut was never a disclosure control.** Every reply check runs on the untruncated text, so sending
the rest as a file adds nothing that was not already approved. Three cases send the cut message exactly
as before, none as an error: a transport that cannot carry a file, a reply larger than the attachment
bound, and a send budget with no room to say the file exists. **Discord does not render mentions inside
an attachment**, so the file carries the answer as composed while the message carries the ids the
harness resolved. The job-notice path truncates at the same budget and looks like the same defect but is
not one: **a job ends on one of three fixed phrases, so that cut can never fire.**

## Repairing a refused reply

A reply check refuses the whole message, so one block naming a channel nobody has discarded eleven
correct blocks beside it, including an outage report found in a trace hours later rather than read in
the channel (#796). A repair loop already existed in the completion layer, naming the refusing check to
the model so it could fix the clause rather than rephrase blind, **but it covered only `parse` and
`response_style`**. The other five ran in `runReplyChecks` after `Complete` returned, so a grounding
refusal ended a 51-second turn outright: **the machinery was there and those checks were not wired into
it.** `ProxyClient.ValidateReply` now takes the harness's own check set, so all ten rules reach the loop.

**Repair is advisory, and the verdict did not move.** `runReplyChecks` is still the only thing that
refuses, so the hook **can turn a refusal into a rewrite and can never turn a refusal into a delivery**.
That is why the loop drops the harness checks once the repair budget is spent rather than failing on
them: failing would return `ErrResponseRepairExhausted` and report `stage=model`, true about the code
and false about the world. **A model that will not fix the clause produces the same outcome as before
this change, byte for byte.**

**Tools are already withdrawn on a repair attempt**, so the model cannot answer a grounding refusal by
substantiating the claim: **a check refusing an unsupported claim must not become a prompt to go find
support for it.** The budget is `maxResponseRepairs`, one attempt, shared with the contract checks.
Preserving a refused reply for an operator was weighed on #796 and is not implemented, because **a reply
refused by `identifier_disclosure` carries the value that check exists to keep out of logs**.
**Measurement is deliberately unhooked**: the eval scorer runs the checks itself, so the rate packs stay
comparable **instead of reporting the harness's repair as the model improving**.

## Redacting a block instead of the message

Redaction is the last rung: repair gives the model a chance to fix the block, and only when that fails
does the block go. **The unit is a block**, a paragraph or one item of a list, because the blank line is
the break a member already reads and a list item is a break too, the reply this was written for being
twelve bullets with no blank line in it. **A sentence is the wrong unit**: `[.!?]+` splits prose, not
reasoning, so a claim and the qualifier bounding it land in different sentences.

`redactableRules` is an allowlist: the four grounding rules, tool-call markup, and self-attributed
claim. Everything else refuses whole, because `response_style` and `identity_claim` are properties of
the whole voice and `parse` means there was no reply to cut. **`identifier_disclosure` looks redactable
and is not**: it collapses digits and spelled numbers across the entire reply, so a guarded value can
span two blocks and neither block fails alone.

**Two passes, and the second is the authority.** The first runs the checks over each block alone and
marks the blocks failing **the same rule that refused the message**, which keeps a block that merely
disagrees in isolation from being removed. The second runs every check again over the surviving prose,
**so what is delivered passed the full set on its own rather than merely lacking the removed block** -
which makes an imperfect first pass safe, since it picks candidates and licenses nothing. If the
remainder fails, the reply is refused whole. It stops when there is nothing to save, nothing left, or
too much to remove (`maxRedactedBlocks`).

The removal is marked in place in the harness notice shape, **whose alphabet a model reply cannot
forge**, with adjacent removals sharing one mark. The mark is part of the answer rather than a service
suffix, so length truncation can reach it when the removed block was last: **only the disclosure is ever
at risk, never the removal.** **A remainder can still depend on what was removed**, and re-validation
checks rules rather than reasoning, so the mark is the mitigation. `response.check` names the rule so a
redaction appears in the refusal rates, `response.redacted.blocks` carries the count on every turn
including zero, and `response.check.redacted` logs the rule, the sentence, and the count.

## Silence is a choice, a blank is a defect

**Silence is only a reaction** (#8364). A blank after reads only is repaired (#8326). After
a call that may speak, the turn marks `acknowledge` rather than answer twice (#895), and
`turn.reply.silent` logs it. `RequireReply` stays strict for the scorer.

So **a blank arriving at `sendReply` is a defect** (#1035): nothing is sent, `turn.reply.blank` records
it, and the message is marked `acknowledge` instead. **A mark that fails to land is not a message.**
mcp-beaver#87 covers the tool path.
