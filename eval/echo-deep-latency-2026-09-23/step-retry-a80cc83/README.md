# Retry-round fix: tools array kept on repair rounds

**Raw outputs removed on 2026-11-04 (COI-3294).** Live-run output can carry player names, so `raw/`, `results.jsonl` and `run.log` were deleted. They were last held at commit `1dd38a836a17e600c4d93149df031ff243bd4923`. The findings below stand, and the paths they name now resolve only at that commit.

2026-09-24, scientist seat, `teable:coilyco-gaming/sirens-echo#8139`. Image
`a80cc83` (#1215) on both lanes, read before the run (`images-before.txt`). Same 3
prompts, 10 reps x 2 lanes, seed 81391, the same turn order as Step A, and one
discarded warm-up per lane. `PREDICTION.txt` was committed before the first turn
(`0ea48a1`). The baseline is `../step-a-035acf38`. Run window 00:56:45-01:01:54 UTC.

## Measured (`analysis.txt`, and SigNoz for the same window)
* echo p1 (hello) median 3.41s [95% CI 3.15-3.51], Step A 3.32s [3.08-4.22]. No move.
* echo p2 (tidal lock) 5.78s [5.56-6.05], Step A 5.96s [5.82-6.25]. The CIs overlap.
* controls: echo p0 2.30s [2.10-2.48] against 2.20s. deep p0 3.79s against 3.56s.
  deep p1 3.03s against 3.03s. deep p2 9.55s [6.88-13.75] against 7.72s, with CV 52%.
* 60/60 turns ok. Warm-ups: echo 11.28s, deep 4.71s.
* `model.response.repair` on sirens-echo: 20 in this window and 20 in Step A. The
  treatment arm got one repair round per echo p1 and p2 turn in both runs.
* `model.tool_call.refused`: 0 in both windows.
* agent-proxy `request.chat` `agentproxy.stream.first_token_ms` on
  `sirens-echo/default`: p50 694ms (Step A 748ms), p90 976ms (Step A 1069ms). 51
  spans, which is 31 turns plus 20 repair rounds.
* echo p1 replies: "Hello." 8/10, "Acknowledged." 2/10. Step A: "Hello." 3/10,
  "Acknowledged." 7/10. `model.response.shipped` with "social opening" rose in the
  same ratio (3 to 8 per turn count, counted twice per event by the log grouping).

## Inference, not measured
* In Step A the tools-less repair round was already hitting the cache. Its p90 first
  token was about 1s, not the 9s of a cold 40k-token prefill. So #1215 had no latency
  to recover on these prompts, which was the second branch of the prediction.
* Keeping the tools array did change what the repair round says. The hello repair
  now ships "Hello." past the social-opening check 8 times in 10, against 3.
