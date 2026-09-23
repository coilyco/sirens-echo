# Rebuild of dimension 01, after the footer parser lost a row

`CORRECTION.md` says what is wrong with `fidelity.py` as merged at `91de8ece`
and refuses to fix it at speed. This page is the fix, and it opens with what was
predicted before any of it ran, because the previous three readings of this
defect each looked finished and then moved.

Tracked as `teable:coilyco-flight-deck/housecast#7566`.

## Predictions, written 2026-09-12 19:47Z before the rebuild ran

Every figure below is EXPECTED. Nine came to me as another seat's measurement
and one is my own reading of the renderer, so none of them is evidence yet. The
point of writing them here first is that a criterion invented after the output
is seen fits whatever came back.

* Footer rows across the 47 replies: **41**, glyphs hammer 40 and book 1.
* Footer runs parsed from the reply text: **70**. From the `disclosed` field
  as the corpus carries it: **68**.
* Records where those two disagree: **1**, at ordinal 45.
* Replies carrying a non-empty disclosure: **15** before the rebuild and 15
  after, so the board's population does not move and no cell changes from a
  non-score to a scorable one.
* Ordinal 45's book row: label `skill`, 2 runs, against a trace of
  `skills/read_skill` 2 and `tvmaze/search_tv_show` 2.
* Ordinal 36 stays the one real failure: footer 4 against trace 2 on
  `gbif/search_species`.
* The rebuilt aggregate: footer runs **70** against in-window trace calls
  **68**, disagreeing by exactly the size of ordinal 36.

## One prediction of my own, which disagrees with the record

`teable:coilyco-flight-deck/housecast#7566` settled on mapping a book row to
the skill-read class. I predict that mapping is **wrong in the general case**,
and that the corpus is too small to show it.

Read at `coilyco-gaming/sirens-echo` `main` `28d9b4a`: `toolDisclosureLine`
emits the book glyph whenever `call.Detail != ""`, and `Detail` is set in two
places rather than one. `skilltool.go` sets it to a skill's display name, and
`trackeradapter.go` sets it to a tracker record key on filing, commenting and
closing. So a book row means "this call carried a display value", and a reader
who maps the glyph straight onto the skill-read tool has hardcoded the only
case this corpus happens to contain.

The same file gives the join that does work. `proxy.go` sets
`mcp.tool.skill` on the tool span from that same `Detail`, so the trace knows
which calls rendered as book rows and under which real tool name. A book row
should be matched on that attribute, and a cell whose evidence lacks it should
be reported unresolvable rather than guessed in either direction.

## What ran

Corpus pulled fresh from S3 rather than reused from a session directory, and
verified against the MANIFEST's hashes before anything read it:

    edce509566350697  all-raw.json          matches MANIFEST
    d2233067b9d18e4b  deep-replies.json     matches MANIFEST
    b083e14ba65dbe40  deep-replies.jsonl    matches MANIFEST

The renderer was read at `coilyco-gaming/sirens-echo` `main` `28d9b4a`:
`internal/community/tooldisclosure.go` for the row shape and the two glyph
vocabularies, `notice.go` for the alphabet a book row's label was printed
through, `proxy.go` for the span attribute that carries the same value, and
`skilltool.go` and `trackeradapter.go` for the two places that set it.

`footer.py` is the one parser both the extractor and the pass read, because the
defect was a parse the comparison could not see. `extract.py` re-derives the
three disclosure fields from `all-raw.json`. `fidelity.py` now parses the reply
itself and reports any disagreement with the corpus field rather than trusting
either.

## What the rebuilt board says

Every figure below is MEASURED, and the commands and their full output are in
`extract.txt`, `fidelity.txt`, `fidelity_control.txt` and
`fidelity_permutation.txt` beside this page.

The re-derivation moved exactly one record:

    footer rows          40 -> 41
    footer runs          68 -> 70
    replies disclosing   15
    records changed      1
      ord 45  rows 1 -> 2   runs 2 -> 4
    rows by kind         {'tool': 40, 'skill-read': 1}

The board:

    cells                15
      pass              14   ordinals [5, 8, 23, 24, 26, 27, 29, 33, 34, 35, 38, 45, 46, 47]
      fail, count only   1   ordinals [36]

    the aggregate, over the cells that were actually compared
      footer runs across those cells 70
      in-window trace calls          68
      They disagree by +2

**Ordinal 45 is a pass, and it is a pass on evidence.** Its book row is placed by
the trace, not by a glyph mapping: the `mcp.tool.call` spans on trace
`c7dbe813` carry `read_skill` with `mcp.tool.skill` of `SKILL` twice and
`search_tv_show` with no such attribute twice, and `SKILL` normalises to the
`skill` the footer printed. Two classes, four runs, exact.

Every prediction above held, including all nine that arrived as another seat's
measurement. The tenth, my own, is the one that needs qualifying below.

## The two controls that now exist

    24 controls, 11 of them differences the pass must catch
    PASS: every control landed

Thirteen of those are parse cases, which the merged sheet had none of. Two hand
the parser an unseen kind glyph and an unseen outcome glyph and require it to
raise. Run against the **un-rebuilt** corpus the pass still scores ordinal 45 a
pass and reports the field as wrong:

      rows parsed from the reply      41
      rows in the corpus field        40
      cells where the two disagree, and the field is the one that is wrong:
        ord 45  field 2 runs, reply 4 runs

Run with the skill spans withheld, ordinal 45 becomes
`unresolved, no skill evidence` rather than a silent pass or a false failure. So
the two ways this instrument was previously wrong now both announce themselves.

Independence holds on the rebuilt board: 14 of 15 against their own trace, 3 of
210 across every offset, the same 1.4% and the same three single-tool signatures
as before.

## My own prediction, half right and worth stating as such

The general claim holds and the corpus does not exercise it. `Detail` is set in
two places, so a book row can name a tracker record key rather than a skill, and
a parser mapping the glyph onto the skill-read class would be wrong on that row.
MEASURED over 2026-08-15 to 2026-08-20: every span carrying `mcp.tool.skill` in
that window is `read_skill`, 10 rows across 5 traces, no tracker call among them.
So the hardcode this rebuild avoided would have produced the right answer on this
corpus, and the reason to avoid it is the next corpus rather than this one.

That is the honest shape of it: a design argument grounded in the source, not a
defect this data demonstrates.

## What this rebuild checked and found clean

* **No undisclosed skill read inside the corpus.** 4 of the 5 skill-bearing
  traces are not among the 15 disclosure-carrying replies, so I resolved their
  `discord.reply` message ids against the corpus: MEASURED, none of the three
  that carry one is among the 47 replies, and the fourth has no `discord.reply`
  span at all. No corpus reply made a skill-read call its footer hid.
* **The corpus reply text is the raw text.** 47 of 47 records resolve into
  `all-raw.json` by `reply_id` with byte-identical content, 0 differing. That is
  what licenses re-deriving the disclosure fields without rebuilding the rest.

## Two faults fixed in passing, both inert on this corpus

* `claimed_total` summed every record's footer while `recorded_total` summed
  every evidence entry, two populations nothing constrained to match, and it ran
  outside the loop that stamps a cell unreachable. The aggregate is now
  accumulated inside the loop over the cells that actually reached a comparison.
  Inert here, because all 15 cells compared and no cell was unreachable.
* An unrecognised glyph produced a shorter list. It now raises.

## What is still owed

* `teable:coilyco-gaming/sirens-echo#7448` - the borrowed window. Unchanged by
  this rebuild, and it still carries none of the result: ordinal 36 is among the
  9 cells where the window excluded nothing.

## What is settled rather than owed

The corpus has no committed build script, so only its disclosure fields are
re-derivable. Kai declined a fix on 2026-09-12: the Discord pull does not get
repeated, the corpus lives where it lives, and losing the copy is fine. So the
S3 prefix holds the only copy by decision, and if it goes the board's provenance
goes with it. `teable:coilyco-flight-deck/housecast#7571`, closed as declined,
and noted here so nobody files it again as an oversight.
