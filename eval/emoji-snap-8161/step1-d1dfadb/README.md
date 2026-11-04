# Emoji snap, step 1: model path only

**Raw outputs removed on 2026-11-04 (COI-3294).** Live-run output can carry player names, so `raw/` and `results.jsonl` were deleted. They were last held at commit `1dd38a836a17e600c4d93149df031ff243bd4923`. The findings below stand, and the paths they name now resolve only at that commit.

2026-09-24, scientist seat, `teable:coilyco-gaming/sirens-echo#8161`. Echo on
`d1dfadb3` (#1219), pod `sirens-echo-6c94c8499d-zx8jw` started 03:09:54Z, with
`SIRENS_ECHO_JEV_MODEL` unset. Same 30 cases, 2 reps, seed 8161, 1 warm-up. Run
03:11:13-03:15:07 UTC. `PREDICTION.txt` was committed before the run (`754de49`).

## Measured
* S snap rate 0/40 (gate >= 90%, FAIL). N false snap 0/18 (PASS). G undefined.
  The baseline on 945fb8a was 0/39 and 0/17. No move.
* `response.reaction.*` on sirens-echo in the run window: 0 (SigNoz).
* No reply was a bare glyph. The only replies of 4 characters or fewer were "ty"
  twice, "pong" twice and "4" twice.
* 2/60 turns came back as silent empty replies (n05 x2), excluded.
* Eligible turns are still answered in words: "Acknowledged.", "Hello.", "Good
  morning.", "Got it.", "Message received.", "Yes, 7 is a prime number...".
* Latency: unsnapped S median 2.90s, N median 4.09s. Warm-up 17.43s.

## Against the prediction
The prediction was a 30-70% S rate. The result, 0%, falls in the no-effect branch
(0-10%).

## Inference, not measured
* The prompt change in #1219 did not move the model's choice on this lane. Either
  it does not reach the rendered prompt the running lane uses, or the model ignores
  it. This run cannot tell those apart, and the game-dev seat can check the rendered
  prompt on the pod.
* The scorer reads the `reaction` key, which is omitted when there is no mark. So a
  0 here is confirmed by telemetry, not by the field alone.
