# Echo and Deep turn latency, pre-fix baseline

**Raw outputs removed on 2026-11-04 (COI-3294).** Live-run output can carry player names, so `raw/`, `results.jsonl` and `pilot/results.tsv` were deleted. They were last held at commit `1dd38a836a17e600c4d93149df031ff243bd4923`. The findings below stand, and the paths they name now resolve only at that commit.

2026-09-23, scientist seat. Tracked in `teable:coilyco-gaming/sirens-echo#8139`.
The question: how long a `turn` takes on each lane, and how much it varies, before
PR #1212 (Jev server pruning, clock after the stable prefix) rolls. This is the
baseline that Step A (image `59a9c10`, Jev off) and Step B (Jev on for echo) are
compared against.

## Method
* Client: `mcporter call tailnet_coilyco_sirens_{echo,deep}.turn --output json`
  from kais-macbook-pro over the tailnet, author `perf-test`, sequential.
  The latency is client wall clock around the whole call.
* Prompts: p0 `hi, quick check: what is 2+2?`, p1 `Say hello back in one short
  sentence.`, p2 `In about 80 words, explain what a tidal lock is.`
* `run.py`: 3 prompts x 20 reps x 2 lanes, order shuffled with seed 8139, plus one
  discarded warm-up turn per lane. `PREDICTION.txt` was written before the first
  turn and amended before the confound below was inspected.
* Both deployments were on image `d9971294` at the same generation before and after
  (`images-before.txt`, `images-after.txt`).
* Confound: another seat's direct Ollama calls forced echo model reloads from
  23:48:50 to 23:50:14 UTC. Every echo turn overlapping that window is dropped by
  time rather than by slowness, which drops 3 turns including one that looked clean.
  `retake.py` re-ran the same 3 cells afterwards.
* `analyze.py` reports medians with a seed-fixed bootstrap 95% CI, p90, cv, distinct
  replies, and Spearman rho against reply bytes, run order and tool-call count.
  It was checked against a synthetic fixture with known answers before use.
* `pilot/` is the first 18-turn run (n=3 per cell) that motivated this one.

## Measured (`analysis-noretake.txt`, `analysis-retake.txt`)
* echo p0 n=20 median 10.82s [10.77-10.94], cv 1.2%, 1 distinct reply
* echo p1 median 15.66s [15.46-15.87] with retakes, replies split `Hello.` 12 and
  `Acknowledged.` 9 across all echo p1 files
* echo p2 median 18.43s [18.06-18.77] with retakes, 4 distinct replies
* deep p0 n=19 median 3.90s [3.69-4.08], calculator called 19/19
* deep p1 n=20 median 2.94s [2.68-3.18]
* deep p2 n=20 median 7.05s [5.80-12.12], cv 82.9%. The 12 turns with no tool call
  took 4.0-7.4s, the 8 with 2 to 9 tool calls took 9.9-42.3s, rho(secs, tool calls)
  0.73.
* 1 failure: deep p0 rep 19 returned exit 0 with an empty reply in 2.0s.
* Deep turn 026 (p2) called `teable.search_issues` twice and attempted
  `teable.create_issue`, which failed. A perf prompt reaches side-effecting tools.

## Prediction against result
* Right: echo p0 median near 11.0s (10.82s), echo p1 near 15.7s (15.66s), deep p2 bimodal
  with a cluster near 5s and one above 10s.
* Wrong: echo p0 cv was 1.2% against a predicted cv under 1%. Echo replies are not byte-identical across reps (p1 and p2 vary), and deep
  p2 latency does correlate with reply length (rho 0.74), because longer replies
  carry more tool calls.

## Inference, not measured
* Echo's floor is prompt reading on the local 9B model (`ollama_chat/ornith:9b`, about
  40K input tokens per the sysadmin seat). Measured: in SigNoz trace
  `0bd517d7810f417076e0e78ca392353f`, `model.chat` holds 10195.9ms of a 10196.7ms
  `community.turn`.
* Echo's tight spread with varying text suggests the cost is fixed by prompt size
  rather than by what the model writes.
