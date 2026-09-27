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

## Not done
* n=1 per tool except the two reruns. The pass and partial split is one sample
  and sits within noise, and the 4 refusals follow from one quoted sentence.
* `trade_watchers` was probed with `list` only, because a `create` probe would
  write a watcher into eco-app's store.
* The `get_world` and `get_crafting_atlas` claims that no per-action or
  per-player mining breakdown exists were not checked against the payload.
* The Eco focus itself was never under test, because the deployed lane does not
  load it. The run after the swap is the one that measures it.
