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
