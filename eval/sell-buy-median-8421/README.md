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
