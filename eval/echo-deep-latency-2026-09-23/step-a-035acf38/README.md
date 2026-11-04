# Step A: clock after the stable prefix, Jev off

**Raw outputs removed on 2026-11-04 (COI-3294).** Live-run output can carry player names, so `raw/` and `results.jsonl` were deleted. They were last held at commit `1dd38a836a17e600c4d93149df031ff243bd4923`. The findings below stand, and the paths they name now resolve only at that commit.

2026-09-24, scientist seat, `teable:coilyco-gaming/sirens-echo#8139`. Image
`035acf38` on both lanes (code of #1212, `SIRENS_ECHO_JEV_MODEL` empty), read before
and after the run. Same 3 prompts as the baseline, 10 reps x 2 lanes, shuffled seed
81391, one discarded warm-up per lane. `PREDICTION.txt` was written before the roll.

## Measured (`analysis.txt`, SigNoz aggregates over 00:25:55-00:30:50 UTC)
* echo p0 (2+2) median 2.20s [95% CI 2.10-2.34], against 10.82s [10.77-10.94] before
* echo p1 3.32s against 15.66s, echo p2 5.96s against 18.43s
* deep p0 3.56s [3.38-3.85] against 3.90s [3.69-4.08], no move beyond noise
* echo warm-up, the first turn on the new pod, 11.85s
* agent-proxy `request.chat` p50 `agentproxy.stream.first_token_ms`: echo route
  9289ms before, 748ms after. deep route 962ms before, 1015ms after.
* p50 `gen_ai.usage.input_tokens`: ornith:9b 40061 before, 40047 after
* 60/60 turns ok, every 2+2 answer on both lanes was 4

## Inference, not measured
* Input tokens did not move while time to first token fell 92%, which is prefix-cache
  reuse rather than a smaller prompt. The cold warm-up turn at 11.85s fits a miss.
* Per-turn pruning (Step B) changes the tool block, which the qwen35 renderer puts
  first, so it would likely miss the cache on every turn whose pruned set differs.
