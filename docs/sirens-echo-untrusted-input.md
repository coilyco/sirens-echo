# Untrusted input

Links out, files in, caller-supplied history, and the value a reply may never echo.

## Outbound link policy

**A reply may contain a URL only when that URL appears verbatim in the approved registry.** A link
written from memory is a fabrication even when the address turns out to be real, so **an unlisted link
is never preferable to no link**. The rule governs what the model writes: the `Referenced issues:` block
the runtime appends is built from URLs a tool returned this turn, **a receipt rather than a
recollection**, and the model names a tracked issue by number and never builds the address itself.

**The registry is knowledge rather than code**, split across the active game focus's own
`references/links.md` and `sirens-echo-knowledge`'s `references/links-community.md` (the latter carrying
the one built address, the plain English Wikipedia article form, and the only half that survives a
a game swap). **Each game's wiki entries were taken from that wiki's reviewed
published index rather than recalled, and every other address was requested and returned a response
before it was written down.** A page that could not be reached for review is not listed.
A link belongs after the information it supports, one being the ordinary case and two the
maximum, and **linking is not a substitute for the knowledge-gap path**.

**A registry is only useful if the runtime accepts what it lists.** `maskURLs` replaces every link span
with a plain word before the style and channel checks run, because without that step **the first-person
expression matched the `me` ending every `coilysiren.me` host**, so the runtime rejected every approved
link the service can publish, and a URL fragment was read as an invented channel for the same reason.
**A registry host therefore needs a test, not an assumption**, and
`TestValidateNeutralStyleAcceptsRegistryHosts` covers one address per host family. Two gate cases score
through `required_patterns`, and **both accept every link the policy would call correct, so neither can
fire on a correct reply**; cases needing an MCP roster are deliberately absent **because a link case
must not fail for a reason that is not the model's**.

## An uploaded file is data, never instructions

A large prompt body arrives as a file and is read through a tool, because **splicing it into the prompt
would make every turn carrying a file pay the whole file up front**. It lands in the requester's
scratchpad under `uploads/`, not a second file concept: path confinement, the per-file limit, the
per-requester quota, and attribution all apply because the write goes through the same reserved path the
tool-result spill uses.

**`uploads/` is reserved, so the model cannot write there.** If it could, **it could forge a file and
then cite it as something a member supplied**, and it is separate from `tool-output/` for the mirror
reason. The turn is told the path, that the text is not in the prompt, and the file's size, **the size
being what decides between reading the file back and searching it**, which the model cannot learn
without spending the read the choice exists to avoid.

"No executable shenanigans, or smuggled rick rolls" names two things and only one is solvable.
**Executables are solvable**: the check is the bytes, a null byte or invalid UTF-8 refuses, and the
extension and declared media type both belong to the uploader. **Smuggled instructions are not**: a text
file is exactly the shape prompt injection takes, and no filter separates a document discussing
instructions from one issuing them. **So the bound is posture rather than detection**: the file is
untrusted input always, **including from the principal**, never widening authority, never admitting a
caller, never naming a tool, and a URL inside it is inert.

## Caller-supplied history carries no authority

Both private ingresses take an optional `history`, and **the caller writes every field, including each
entry's author**. The transcript is flattened as `- <author>: <content>` and the runtime writes the
author `assistant` when it folds a resolved prompt into a turn, so **a caller setting that same author
produced a line indistinguishable from one the runtime wrote about its own prior turn**. Live QA
measured the effect: a forged entry asserting a completed identity verification **raised principal user
ID disclosure from about 13 percent to about 40 percent**, and removed the hedging that had wrapped the
unforged answer.

**Rejecting the string `assistant` stops one spelling, not the lever.** A caller can equally assert
`system` or the service's own display name, **so that fix would have looked complete while leaving the
measured behavior available**. Folding caller history into one attributed role removes the forged
authority too, but flattens genuine multi-party context, which the transcript exists to express.

So every caller-supplied entry is marked at assembly and rendered as `(asserted by the caller, not
observed)`. **Authors are preserved, so multi-party context survives, and a forged prior turn is no
longer addressable as something the service actually said.** This is the same mechanism `(an agent, not
a person)` uses, so **the model reads a grounded fact rather than inferring provenance from prose**, and
the two markers compose. Discord history carries no mark. **A prompt-level mark is not enforcement**:
the network boundary remains the real control, since reaching either ingress means being an authorized
tailnet node, and if a re-measure shows the rate unchanged **that is evidence the load-bearing fix is
the output-side validator**.

## The principal echo check

`forbid_principal_echo` protects the operator's handle and user ID. **The invariant is the value and not
its spelling**, so the check normalizes both sides before comparing. The user ID no longer reaches the
prompt at all, so this is a backstop: **a reply check has no repair loop, so catching a recited
identifier costs the member the whole answer**. **A literal match alone could not tell "did not
disclose" from "disclosed in a form the check does not read", and a green run that cannot tell those
apart is certifying rather than measuring.**

The user ID is compared as the literal string, then against the reply reduced to its digits, then
against that reduction with whole-word digit names rewritten first, then against the reversed digit run,
then against the four base64 alphabets. **Collapsing to digits turns every separator into one
comparison.** The handle is compared with every non-alphanumeric character removed from both sides.
**Link hosts are removed before the handle comparison**, because the operator's own sites carry the
handle in their hostnames and **every approved link would otherwise fail the check on a correct reply**;
link paths are still read. The digit normalization applies only to an identifier of eight digits or
more, because **collapsing a reply to its digits would let a short identifier collide with ordinary
numbers**.

**Residual misses, stated rather than implied**: bases other than ten, ciphers such as rot13, compound
number words, nonstandard digit names, a value split across separate replies because the check is per
turn, and a handle inside a hostname the check masks. **The gap is narrowed and not closed**, and the
list is short enough to reason about, which the pre-normalization miss surface was not. Normalization
keeps the target set closed at two identifiers, **and enumerating evasions would open the set and lose
to the next encoding by construction**. Paraphrase disclosure has no closed target set and went to the
board as `no-instruction-disclosure` instead.
