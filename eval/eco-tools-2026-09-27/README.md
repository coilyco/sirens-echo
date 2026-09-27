# Can Echo use every eco-app tool before cycle 15

2026-09-27. The question: five days before the cycle 15 cut on 2026-10-02, does
the deployed Sirens Echo lane call the right eco-app tool when a member question
needs it, for each of the tools the `eco-game` roster entry mounts. This measures
the running lane through `/v1/turn`, as the game focus section of
[AGENTS.md](../../AGENTS.md) asks, and reads no green test as a stand-in for it.

## Setup
* Tools under test: the 25 names eco-app registers at `d58b679`, counted from
  `src/eco_mcp_app/wave{1,2,3}_routes.py` plus `get_social` and `trade_watchers` in
  `server.py`. Every one has a probe in `probes.tsv`.
* Lane: sirens-echo image `f31b1d9`, the deploy repository at `201a14f` pins the
  same full sha. The deployed definition loads `sirens-game-enshrouded`, so the Eco
  focus is not in the prompt, and `game-seasons.md` says Eco is off season.
* One member-shaped question per tool, no history, one author, sent through the
  `sirens_echo` MCP `turn` tool. Failed turns were retried once when the notice
  named the backend, and failures naming the tool or the budget were rerun once to
  check they reproduce.
* The Eco server was up during the run: cycle 14, day 84, 0 online, 0.13.0.4.

## Correctness, fixed before the first probe
* **pass** - the reply carries a value that only that tool's output holds, checked
  against a direct call to the same tool, or shows that tool in its footer, and it
  answers the question asked.
* **partial** - the right tool ran, and a named part of the question went
  unanswered or the reply misreads the payload.
* **fail** - no tool ran, or the turn ended in an error notice.

Prediction written at 05:33:10Z, before any probe: at least 12 of 25 fail, mostly
off-season refusals driven by `game-seasons.md`. That prediction was wrong on the
count and right on the dominant cause.

## Measured (`probes.tsv`)
* 9 pass, 11 partial, 5 fail. The right tool ran in 20 of 25.
* **4 fails are off-season refusals with no tool call**: `get_stores`,
  `get_economy`, `get_milestones`, `get_climate`. A direct call to each returned
  live cycle 14 data in the same hour. `game-seasons.md` says "No Eco world is
  running ... Say it is off season, name the date, and stop", and the server says
  otherwise. The same sentence colours 3 partials (`get_market`, `fair_price`,
  `get_progression`), which ran the tool and then blamed the season.
* **`get_region` fails in eco-app, not in Echo**: "tool call failed" 2 of 2 through
  Echo, and a direct call from another client dropped the transport after 120 s.
* **`get_map` exhausts the reasoning budget** on "who owns the biggest plot" 2 of 2
  (`noticeBudgetSpent`), and answers a narrower deed question with the tool.
* Where a tool ran, it was the right one in 20 of 20. No probe drew on a wrong
  tool's data. Tool choice is not the gap. What the knowledge tells Echo to do with
  the season is, plus one eco-app defect and one budget ceiling.
* Recipe answers come from a vanilla AutoGen graph fetched 2026-08-13
  (`serverSpecific: false` in the `price_recipe` payload), so they describe
  the current graph, not the 0.14.1 modset cycle 15 runs.
* Runner noise across 33 turns: 4 "model backend unavailable" and 1 rate limit,
  each retried once and not scored.

## Run 2, after the seasons fix (`run2.tsv`)
* Lane `416cc89`, available from 06:04:30Z, carrying sirens-echo PR 1239. Focus
  still `sirens-game-enshrouded`. Same 25 probes and rule, 06:08 to 06:13Z.
* 13 pass, 10 partial, 2 fail, against 9, 11 and 5 in run 1. **0 off-season
  refusals**, down from 4.
* Prediction written at 06:08:27Z: 0 refusals, `get_region` still failing, `get_map`
  still out of budget. `get_region` answered, so that part was wrong, and a direct
  call recovered too, with no fix seen landing.
* New failures: `get_economy` returns an empty reply 2 of 2, and `fair_price`
  answers about a currency instead of the ingot price.
* Rescored: the run 1 `find_trade` pass named a store holding 0 lumber. I checked
  the price and not the stock.
* Direct calls separate the gaps. The tailor, milestone and biggest-plot answers
  are in the payload and Echo misses them. Per-player mining, per-player road
  and terraform counts, and an item filter on trades are absent from the tools.

## The can't-answer catalog (`cases.tsv`)
Every question Echo cannot answer across both runs, one row each, with whether
the payload holds the answer and the tracker record that owns the fix. Rerun a
probe from `probes.tsv` against the deployed lane to close a row.

The cycle 15 cut moved to 2026-10-09 after this was written, so the dates above
describe the plan as it stood on the day of the run.

## Not done
* n=1 per tool except the two reruns. The pass and partial split is one sample
  and sits within noise, and the 4 refusals follow from one quoted sentence.
* `trade_watchers` was probed with `list` only, because a `create` probe would
  write a watcher into eco-app's store.
* The `get_world` and `get_crafting_atlas` claims that no per-action or
  per-player mining breakdown exists were not checked against the payload.
* The Eco focus itself was never under test, because the deployed lane does not
  load it. The run after the swap is the one that measures it.

## Tool functionality audit (`tool-audit.tsv`)
Kai asked whether some eco-app tools never worked, to disable them if so. One
direct call per tool at 06:33 to 06:35Z, graded against the fields each
description promises, with the grade rule written at 06:33:07Z before any call.

* 16 work, 7 degrade on a secondary field, 2 fail their purpose.
* `get_economy` reads 0 of 15 datasets while `get_currency` reads one of the same
  datasets, so the fault is its own read path.
* `fair_price` returns an in-game price for 1 of 5 items it accepts. Its Iron and
  Copper entries name items Eco lacks and have since the tool was created.
* Jev put disabling both at 0.82 and 0.77. Records `teable:coilyco/eco-app#8345`
  and `#8346`.
* SigNoz traces cannot show "never worked": 30 days hold 2 to 60 calls per tool
  and 1 error span, because a tool that returns empty data still returns 200.

## Run 3, on b3f7f86 (`run3/`, `cases.tsv` column `run3_b3f7f86`)
After `get_economy` and `fair_price` were disabled, `get_region` was fixed and the
direct-caller blank reply was repaired. 07:52 to 08:03Z. The 9 model-dependent
probes ran through `run3/run.py` at a 300 s timeout, because turns took 60 to 90 s
and a 60 s client timed out 3 of 3. Raw replies are in `run3/results.jsonl`.

* Fixed: biggest plot (1 of 1, after 3 of 3 out of room) and biomes.
* Still open: 11 rows. Fair price is out of room 2 of 2 now that its tool is gone.
* Two rows are not model answers at all. Lumber and steel axe return in under a
  second, byte-identical across three builds, from eco-app reply templates that
  answer with no model call (`internal/community/directtool.go`).
* The blank-reply repair does not reach Discord by construction, and these probes
  go through `/v1/turn`, so no row here speaks for what a Discord member sees.

## Run 4, on c8ce1d7 (`run4/`)
All 25 probes at 18:16 to 18:40Z, for acceptance 4 of `teable:coilyco/sirens-echo#8364`
(an empty reply only alongside a reaction). The prediction is in `run4/prereg.md`.

* Confounded: deploy #1009 unmounted `eco-game` at 16:19Z (tool_count 97, down
  from 120), so 0 of 25 turns called an eco-app tool. Echo answered from Discord
  scrapes and recall, and some answers are wrong: the Steel Axe recipe came back as 200
  Limestone and 75 Iron Ore. Kai chose to remount (`teable:coilyco/deploy#8367`).
* Acceptance 4 is inconclusive. 4 turns returned `reply ""` with rc 1, and the runner
  did not keep stderr, where an MCP error's notice text goes. `run4/run.py` now keeps
  it, and reruns of those 4 (`run4/retry.jsonl`) were non-empty.
* No catalog column for this run, because it measured Echo without the tools the
  catalog is about.
