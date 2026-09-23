# Correction: dimension 01's headline finding does not survive

Written 2026-09-12, after `fidelity.py` and `fidelity.txt` were merged at
`91de8ece`. Both stay as written. This page says what is wrong with them and
what the numbers become, because regenerating a record is a separate act from
admitting it was wrong, and the second cannot wait for the first.

## What was published

> The aggregate is the number not to quote. Footer runs across all 15 cells
> total 68, and in-window trace calls total 68. They agree exactly, because the
> two failures differ in opposite directions by the same amount. A board
> reported only in total would have found nothing and called it fidelity.

That was the finding I called talk material. **It is an artifact of two faults
in my own instrument**, and on corrected data the aggregate detects the
failure it was said to miss.

## Fault one: the corpus field loses a footer line

`fidelity.py` reads `record["disclosed"]` and never touches `record["reply"]`.
The corpus extractor recognises the hammer glyph and not the book glyph, so one
footer line is absent from the field it writes.

MEASURED over all 47 records, parsing footer lines out of the reply text:

    glyphs seen in footer lines : {hammer: 40, book: 1}
    footer runs, parsed from reply text : 70
    footer runs, from the disclosed field : 68
    records where the two disagree : 1   (ordinal 45)

Ordinal 45's reply ends:

    > BOOK OK `skill` x2
    > HAMMER OK `tvmaze.search_tv_show` x2

and its `disclosed` field carries only the second. Raised by the Developer
Advocate seat, confirmed here against the corpus.

## Fault two: a book row is a disclosure, and the matcher cannot see it

Read from the renderer, `internal/community/tooldisclosure.go` in
`coilyco-gaming/sirens-echo` at `main`:

    glyphs := toolDisclosureGlyph + " " + toolOutcomeGlyph(call.Outcome)
    label := call.Label()
    if call.Detail != "" {
        glyphs = skillReadGlyph + " " + toolOutcomeGlyph(call.Outcome)
        label = noticeBody(call.Detail)
    }

A book row is emitted for an executed call whose `Detail` is set, and its label
is `noticeBody(call.Detail)` — **the reference a skill read delivered, not the
tool name**. `ExecutedTool.Detail` is documented as "the session-validated
display value, carried so a skill read can be named on the member surfaces".

So `BOOK OK \`skill\` x2` is a disclosure of two skill-read calls. It is not a
tool class named `skill`, and it is not a missing disclosure. `fidelity.py`
keys on tool name, so it cannot match a row that names content instead.

## What the numbers become

Ordinal 45's footer carries 4 runs (2 book, 2 hammer) against a trace of 4
calls (`skills/read_skill` 2, `tvmaze/search_tv_show` 2). Every tool class in
the trace has a row. Under the settled rule — a class present in the trace and
absent from the footer is a discrepancy — nothing is absent.

* **Ordinal 45 is a pass, not a fail.** It was scored `fail, class dropped` by
  an instrument that lost the line and then could not have matched it.
* **The one real failure is ordinal 36**, which claims 4 runs of
  `gbif/search_species` against 2 recorded calls. An overstatement of 2.
* **Footer runs 70, in-window trace calls 68.** They do not agree, and they
  disagree by exactly the size of the only real failure.

So the aggregate would have caught it. The published claim that a total finds
nothing was true only because a parser drop of 2 cancelled a miscount of 2.

## What survives, and it is not nothing

The lesson inverts rather than dying. The original read was that an aggregate
hides a per-case failure. The corrected read is worse and more useful: **it
took two faults in the measuring apparatus to manufacture the agreement that
made the point.** Three instruments in the chain were wrong at once — the
corpus extractor, the matcher, and the conclusion drawn from them — and the
output looked clean at every step.

## Also checked, and clean

The Portfolio Director seat raised whether `claimed_total` and `recorded_total`
run over different populations, since nothing in the script constrains them.
MEASURED: 15 evidence entries, 15 records with a non-empty `disclosed`, keys
identical, neither set carrying a member the other lacks. The two halves were
over the same population. That concern is closed.

One latent mismatch stands. `claimed_total` is computed outside the loop that
stamps a reply `unreachable`, so an unreachable cell's footer runs would enter
the claimed total while none of its calls enter the recorded one. Inert on this
run, which reports no unreachable cells, and wrong on the first corpus that has
one.

## What has to happen

`fidelity.py` wants rebuilding to parse the footer from the reply text and to
read the glyph rather than only the label, and `fidelity.txt` regenerated from
it. Tracked, not done here: doing it at speed is how a fourth fault joins the
three above.

> **Done 2026-09-12, and not at speed.** [`REBUILD.md`](REBUILD.md) carries the
> rebuild, its predictions written before it ran, and the two controls that now
> cover the parse step. The rebuilt board is 14 pass and one failure at ordinal
> 36, with footer runs 70 against 68 trace calls. This page stays as written: it
> is the record of what was published and what was wrong with it.
>
> One thing on this page did not survive the rebuild either. Fault two is stated
> as the matcher being unable to read a book row, with the remedy implied to be
> reading the glyph. Reading the glyph is not enough and is not what was built.
> `Detail` is set by `trackeradapter.go` as well as `skilltool.go`, so the glyph
> says a call carried a display value and does not say which tool ran. The trace
> carries that same value as `mcp.tool.skill`, and that is what places the row.
