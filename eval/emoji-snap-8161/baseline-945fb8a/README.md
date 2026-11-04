# Emoji-snap baseline, before the build

**Raw outputs removed on 2026-11-04 (COI-3294).** Live-run output can carry player names, so `raw/` and `results.jsonl` were deleted. They were last held at commit `1dd38a836a17e600c4d93149df031ff243bd4923`. The findings below stand, and the paths they name now resolve only at that commit.

2026-09-24, scientist seat, `teable:coilyco-gaming/sirens-echo#8161`. Echo on image
`945fb8a` (revert #1217). 30 labeled cases (`../cases.json`, 20 S + 10 N), 2 reps,
seed 8161, and 1 warm-up discarded. `PREDICTION.txt` was committed before the run
(`55535f2`). Run window 01:28:13-01:34:33 UTC.

## Measured
* S snap rate 0/39 (gate >= 90%, FAIL). G is undefined, with 0 snaps. N false snap
  0/17 (gate <= 5%, PASS). `score-glyph-upper-bound.txt` has the per-case replies.
* `response.reaction.invoked` on sirens-echo in the run window: 0 (SigNoz). As a
  positive control, the same filter over 7 days finds 1, on sirens-deep. Echo shows 0
  over 7 days as well.
* 4/60 turns came back with an empty reply and exit code 1 (n05 x2, s10, n09). These
  are silent finishes, not snaps, and they are excluded from both strata.
* Latency: unsnapped S median 3.01s (n=39), N median 2.97s (n=17).
* Echo answers the snap-eligible cases in words, for example "Hello.", "Got it.",
  "Message received.", and "Yes, 7 is a prime number...".

## Inference, not measured
* The mark path exists but echo does not choose it, even for agree, disagree, and
  acknowledge, which the current key set covers. The gap is the model's choice, not
  only the missing glyphs.
