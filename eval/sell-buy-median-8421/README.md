# Does Echo answer "how much should I sell / buy X for" with per-stage medians

`teable:coilyco/sirens-echo#8421`. When a member asks Sirens Echo what to sell or
buy an item for, Echo should resolve the item, give the median trade price once per
upgrade stage with its trade count and no other price, or say plainly that it cannot
find the item. This directory holds the probe set and the measurement of Echo's
replies through `sirens-echo turn`. The scientist seat authored the probe set, the
criterion, and the grading, and grades them too, so subject and criterion come from one hand.

## Probe set (`probes.tsv`)
* 12 probes, 6 sell and 6 buy phrasings, one member-shaped question each.
* 10 real items: 2 shorthand (`iron` for Iron Bar, `copper` for Copper Bar), 4
  plurals, 3 display names, 1 stage-named item (Basic Upgrade 4, whose name is also
  a stage label).
* 2 names no real item: `unobtainium`, and `dragon scales`, which sits near the real
  item Plushie Dragon and so tests that Echo does not answer about a neighbour.
* `iron` to Iron Bar is Kai's example on the record. `copper` to Copper Bar follows
  it by analogy, and game-dev owns that resolution call.

## Correctness, fixed before the first probe
* **pass, real item** - the reply names the expected item, lists at least two upgrade
  stages, gives each listed stage exactly one median with its trade count, and holds
  no other price figure: no live asking price, store price, quartile, range, or
  multiple. Each stated median matches what a direct call of the same tool returns
  in the same hour.
* **pass, no item** - the reply says it could not find the item, and gives no price
  and names no other item or currency as the answer.
* **partial** - the right item and the per-stage shape, with one named defect: a
  missing trade count, one extra price figure, or a median that does not match
  the tool.
* **fail** - anything else: a wrong or guessed item, a single price or single stage
  in place of the per-stage list, "cannot find" on a real item, a price on a
  no-item probe, or a turn that ends in an error notice.
* Runner noise (a "model backend unavailable" or rate-limit notice) is retried once
  and not scored.

## Amendment, from run 2 on
Game-dev's domain calls on #8421, received 2026-09-28 after run 1 started. Run 1
stays graded under the rule above.
* `iron` to Iron Bar and `copper` to Copper Bar are confirmed: a bare metal word
  means its Bar. Basic Upgrade 4 resolves as the item.
* The unit is Spectres: the cross-cycle stage median times cycle 14's basket
  index, the same basis as `referencePrice`. A stated median is checked against
  that product.
* A stage resting on fewer than `norms.MIN_N` (5) trades is listed as too few
  trades, with its n and no number. A number on such a row is a **fail**.
* A no-item reply may list candidate item names with no price and still pass.

## Amendment 2, from run 3 on
Kai's decision on `teable:coilyco/eco-app#8423` (comment recb6dVAn7R4178xhgr),
relayed by prod-director after runs 2 and 2b had already run. Those two stay graded
under the rule above and record the lane before estimates.
* **Floor.** An item's floor is the lowest stage it was ever traded at, counting
  any trade, including stages with n<5. Any price below the floor fails. The reply
  should say the item was not traded that early.
* **Every stage from the floor to Modern 4 carries a figure.** A stage with a real
  median shows it with its trade count. Any other stage shows an estimate marked as
  an estimate. A stage at or above the floor with no figure fails.
* **Figures are checked.** A real median must match `expected.tsv`. An estimate must
  match the method eco-app documents in `docs/price-history.md`, recomputed from
  the norms file at the deployed ref before run 3. Whether an n<5 real median feeds
  the estimates is the builder's call, taken from that doc.
* Any figure that is neither a stage median nor a marked estimate still fails: a
  range, a "typical" price, a live store price, or a restated rounded median.

## Probe sets added for run 3
`probes.tsv` stays the fixed 12 and remains #8421's done-condition, so every run
compares with every other. Two sets are added and scored separately.
* `probes-phrasing.tsv`, 3 probes, scored under the #8421 rule and Amendment 2.
  The shapes come from game-dev's Sirens Discord search: a single price check,
  item first, and two items in one ask. For two items, a pass gives each item its own
  per-stage block. Covering one item only is a partial. That default is my call,
  and game-dev may overrule it before run 3.
* `probes-8425.tsv`, 10 probes, run only once `teable:coilyco/eco-app#8425` (upgrade
  shorthand) deploys. These are exactly the spec's done list: au3, AU 3, sbu4,
  smu2, bu5, mining bu5, mu0, iron at au3, nylon fabric at MU0, bricks at bu5.
  Each row states its expected answer. The qualifier rows lead with that stage's
  row plus the market-not-your-cost clause, and the figure rules of Amendment 2 apply.
* Two expectations follow from the norms file rather than from the spec's examples.
  They were read at eco-app 38e571c, where the file is unchanged since ec35098:
  * **S06 (mining bu5).** Mining Basic Upgrade has no trade history. 12 of the 16
    tiered specialist modules the spec names never appear in it, and the other
    4 have at most 1 trade each. The expected answer names the item and says it has
    no recorded trades, with no price. The spec does not yet say this.
  * **S09 (nylon fabric at MU0).** MU0 means Advanced 4, and Nylon Fabric was
    first traded at Modern 1. Under Kai's floor rule the answer is "not traded that
    early", not an Advanced 4 row.
* Game-dev confirmed all three calls on the #8425 decision comment, with Jev at
  0.99 on the two-item default. S09 leads with "not traded before Modern 1", then
  the clause, then the priced stages from Modern 1 up. Game-dev also generalised
  S06: **any real Eco item with no trades names itself and says "no recorded
  trades". "Couldn't match" is right only for a word that is not an Eco item.**
  From run 3 on, answering a real item with "couldn't match" or "not an Eco item"
  fails.

## Data facts behind the criterion
Read at eco-app `aacb6bf`, 2026-09-28.
* **The norms pool sell and buy.** `scripts/trades_norms.py` groups every parsed
  `Sold` and `Bought` line on `(item, cycle, stage, currency)` with no side filter,
  and per-item totals in `eco_trades_norms.json.gz` equal bought plus sold in
  `eco_trades_baseline.json.gz` for all 5 items checked (Iron Bar 1819+436=2255,
  Copper Bar 626+118=744, Hewn Log 2964+759=3723, Lumber 1442+254=1696, Bread
  410+14=424). The pooled median needs no data change.
* **A stage is the world's progress, not the item's.** It is the highest
  upgrade module traded so far in that cycle (`none`, then Basic 1 to Modern 4).
  Per-stage cross-cycle medians sit in each item's `crossCycle`, in
  basket-index units rather than a currency.
* **No tool returns more than one stage today.** `find_trade(item="Iron Bar")` at
  03:18Z returned one `norm` per row, for the live stage only (`Modern 4`, n=181,
  11 cycles, reference price 0.5786). So a pass needs a tool or field change,
  owned by game-dev on the eco-app side.

## Expected figures (`expected.tsv`)
One row per item and stage from the norms file at eco-app `aacb6bf`: the
cross-cycle median times cycle 14's basket index (1.5664), in Spectres, with n<5
rows marked too few. It reproduces game-dev's Basic Upgrade 4 rows (Basic 4 219.13
on 13 trades, Modern 4 163.44 on 19), and Iron Bar Modern 4 is 0.58 against the
live `referencePrice` of 0.5786. A stage's n counts only trades in each cycle's
primary currency, because `norms()` builds `crossCycle` from those alone.

## Run 1, baseline (`run1/`)
* Lane image `12d518f` per the deploy pin, 03:20 to 03:22Z, 12 probes, one pass.
  The three `rc 1` turns were retried once over raw JSON-RPC (`run1/retries.jsonl`)
  to read the notice mcporter drops.
* **1 of 12 pass.** Real items 0 of 10, no-item 1 of 2 (P12 passes, P11 fails).
* Causes of the 11 fails: 3 out of budget ("ran out of room to answer", 2 of 2 on
  each of Iron Bar, Lumber, Glass). 3 live store prices in place of stage medians.
  2 cannot-find on a real item (steel bars, Basic Upgrade 4). 1 wrong item (copper
  answered as Copper Wiring, the #8328 shape). 1 unsourced number (hewn logs "10-30
  credits", no tool). 1 answered about another game (unobtainium as Factorio).
* Prediction check: real items 0 of 10 and no-item 1 of 2 both held. The P01 guess
  (Iron Bar or Ore with live prices) was wrong: it ran out of budget instead.
* No reply held a per-stage median, as expected, since no mounted tool returns one.

## Run 2, after price_by_stage (`run2/`)
* Lane image `5f6626d` (run 1's harness), eco-app `38e571c` live, 05:36 to 05:39Z,
  12 probes, one pass over raw JSON-RPC. Prereg `c1ba120` predates the first turn.
  `run2/check.py` checks every stage row against `expected.tsv` and lists numbers
  outside the table. It was tested first on `run2/check-fixtures.jsonl`, 5 of 5.
* **6 of 12 pass.** Real items: 4 pass, 1 partial, 5 fail. No-item: 2 of 2.
* **The answer path decides it.** 4 turns answered from the `price_by_stage`
  template in under 1 s, and all 4 pass with every stage row matching `expected.tsv`.
  Those 4 are exactly the probes phrased "how much should I sell X for?". The 6
  other real-item probes (4 buy or pay phrasings, "what's a good price to sell bread
  at?", "buy a basic upgrade 4") took the model path, and 0 of them pass. The model
  called `price_by_stage` in 1 of the 6 (nails, partial for a "most common
  0.06-0.10" summary). It used `find_trade` live prices in 3, no tool in 1 (an
  unsourced "10-20 credit" bread range), and ran out of budget in 1 (lumber, 2 of 2).
* The template path runs when `route.jev`'s tool pick clears the direct-reply bar
  (`internal/community/agent.go`, `directToolReply`). Inference, not measured:
  the `price_by_stage` description quotes "how much should I sell X for" and no
  buy phrasing, which would explain why only that phrasing clears the bar.
* No-item: dragon scales passes. Unobtainium passes on its retry, after the
  first try hit "model backend unavailable". Under the rule it passes, but it
  calls unobtainium "a Minecraft item", an unsourced claim about another game
  that the eco-app instruction forbids. Run 3 adds "no claim about another
  game" to the no-item pass.
* Prediction check: 5 of 10 real items predicted, 4 measured. The predicted cause
  (summary text on a correct table) explains 1 fail. The main cause, model-path
  routing, was not predicted. 0 out-of-budget predicted, 1 measured.

## Run 2b, variance pass (`run2b/`)
* Same lane, probes, instrument and rule as run 2, 05:44 to 05:47Z. See
  `run2b/prereg-note.md` for how its prereg was stamped.
* **6 of 12 pass again.** Real items: 4 pass, 1 partial, 5 fail. No-item: 2 of 2.
* **The answer path is stable.** Every probe took the same path in both passes.
  Template 8 of 8 on the four "how much should I sell X for?" probes, model 16 of
  16 on the other eight. The prediction (template in at least 3 of 4, model 6 of 6)
  held.
* **What the model path does is not stable.** Nails went from partial (run 2, via
  price_by_stage) to "no current nail price data" (2b, via get_market). Basic
  Upgrade 4 went from live prices (run 2) to a correct price_by_stage table plus
  "averages around 219" (2b, partial). The model called price_by_stage in 1 of 6
  real-item turns in each pass, a different item each time.
* Lumber is out of budget 3 of 3 across run 2, its retry, and 2b.
* Unobtainium was called a Factorio, a Minecraft, and a Satisfactory item across 3
  answers. Each answer names a different game, and none has a source behind it.
* Prod-director's hewn-logs spot probe at about 05:30Z took the model path, but P03
  took the template 2 of 2. Their exact wording and timing are not recorded here,
  so the difference is unexplained.
* `run2/check.py` gained a markdown-table parser for 2b's P10 and passes 7 of 7
  fixtures. On run 2's P08 it misses one row labelled "(current stage)", which a
  hand read confirms matches.

## Run 3, estimates and buy/pay routing (`run3/`)
* Lane image `f3152f4` (runs 1-2's harness), eco-app estimates live from 06:02:48Z,
  06:05 to 06:08Z, the core 12 plus the 3 phrasing probes, one pass. Prereg
  `37ccdbf` predates the first turn.
* **Tool layer first:** a direct read of `price_by_stage` for all 13 real items
  matched `run3/expected-run3.tsv` on 134 of 134 stage rows. That table is
  recomputed by `run3/expected.py` from eco-app's documented method, not its code
  (`run3/tool-vs-expected.txt`). `run3/check.py` passes 8 of 8 fixtures.
* **12 of 15 pass.** Core 12: 10 pass. Core real items: 9 of 10, against 4 in run 2.
  No-item: 1 of 2. Phrasing: 2 of 3.
* **The template path passes 11 of 11.** All buy and pay phrasings now route there,
  and lumber, out of budget 3 of 3 before, answers in 0.4 s.
* **The model path passes 1 of 4.** Basic Upgrade 4 got live prices. Unobtainium
  got "If referring to the Marvel material...", with no statement that it is not an
  Eco item. The two-item price check got "Eco tools are not called during this
  period" and no prices. Dragon scales passes.
* **What sends Basic Upgrade 4 to the model is its name.** Two unscored diagnostics
  (`run3/diagnostics.jsonl`): "buy a iron bar" takes the template in 0.5 s, and "buy
  basic upgrade 4" with no "a" still takes the model. The tool resolves that name
  directly. Inference: Echo's vocabulary match or route pick misses an item name
  that reads like a stage label.
* Prediction check: 9 of 10 core real items held, with the miss on the model path.
  Q01 and Q02 held, and so did no-item 1 of 2 and 0 out-of-budget. Q03 was predicted
  partial and is a fail: it never reached the tool.
* #8421's done-condition (every core reply passes) is not met: P10 and P11 remain.

## Run 4, routing, no-item miss, and upgrade shorthand (`run4/`, `run4b/`)
* Lane read at 06:28Z: the Echo pod ran image 20a40d3 (#1264 when_unmatched, #1266
  argument spans), and eco-app behaved as be79c2d. 7f313d5 (#390) may have rolled out
  during the pass, but it changes only the "masonry mu5" reply, which no probe uses.
  Prereg `66ae3fa` predates the first turn. price_by_stage matched
  `run4/expected-run4.tsv` on 156 of 156 stage rows across 17 items, before and after
  be79c2d. The earlier note said 18 items, a miscount.
* **Core 12: 12 of 12, twice.** Run 4 and the core-only repeat run 4b (06:33Z, prereg
  written in the command that started it) each pass 12 of 12, all in under 1.1 s.
  P10 Basic Upgrade 4 now takes the template. Unobtainium and dragon scales get
  when_unmatched's "Couldn't match that to one Eco item." with no tool call.
  **#8421's done-condition is met, 24 of 24 core turns.**
* Phrasing: 2 of 3. Q03 (two items) now calls price_by_stage twice, but answers with
  Modern 4 medians, ranges and a "1.6x" ratio rather than one block per item, so it
  fails (#8431).
* Shorthand (#8425): 5 pass, 1 partial, 4 fail.
  * **Pass:** smu2, bu5 (21 specialties, no price), mining bu5 (no recorded
    trades), iron at au3, and bricks at bu5. Each qualifier probe leads with its stage
    row and the clause.
  * **Fail:** a bare token as the item: "how much for an au3?", "price check AU 3",
    "price check sbu4". Each asks "What item ... at au3?", reading the token as a
    stage.
  * **Fail:** "what's mu0 worth?" got the vacuum permeability.
  * **Partial:** nylon fabric at MU0 leads correctly and all 4 rows match, but it
    drops the market-not-your-cost clause.
* Prediction check: core 12 of 12 held. Q03 was predicted as partial or fail, and
  it failed. Shorthand was predicted at 8 of 10 and measured 5. The prediction named
  S02 and S07 as the risks, and S01 and S03 failed the same way as S02.
