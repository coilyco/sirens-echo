# Emoji snap, step 2: Jev pre-model snap on

**Raw outputs removed on 2026-11-04 (COI-3294).** Live-run output can carry player names, so `raw/` and `results.jsonl` were deleted. They were last held at commit `1dd38a836a17e600c4d93149df031ff243bd4923`. The findings below stand, and the paths they name now resolve only at that commit.

2026-09-24, scientist seat, `teable:coilyco-gaming/sirens-echo#8161`. Echo on
`a5062e36` (#1222), pod `sirens-echo-77dc94b85f-k9525`,
`SIRENS_ECHO_JEV_MODEL=jev-latest` (resolved `jev-1.13.0` at 03:45:32 UTC), every Jev
family disabled except shape. Liveness: "hi" (not a case) returned `wave`/👋 at
03:45:32. Same 30 cases, 2 reps, seed 8161. Run 03:46-04:17 UTC. `PREDICTION.txt` was
committed before the run (`f9ab105`).

## Measured (`score.txt`)
* S snap rate **16/33 = 48%** (gate >= 90%, FAIL). Step 1 was 0/40 and the baseline
  0/39.
* G glyph correctness 14/16 = 88% (gate >= 95%, FAIL). The two misses: s19 "can you
  confirm you can see this message?" got `acknowledge` where the label was `agree`, twice.
* N false snap 0/16 (gate <= 5%, PASS).
* What snaps: greetings (wave), ack (acknowledge), laugh, celebrate, sad. What does not:
  yes/no and agreement cases s09-s15, "ty" (s06) and "pong" (s20).
* Latency: snapped median 1.06s (n=16). Unsnapped S median 17.57s and N median 19.10s,
  against 2.90s and 4.09s in step 1.
* Failures: 11/60 (step 1: 2/60). 8 empty replies in about 0.7s, 2 client timeouts at
  300s, and 1 empty reply at 24s.

## Confounds, stated rather than corrected
* The backend was slow in this window. The warm-up and one turn hit the 300s timeout
  between 03:46 and 04:00, and unsnapped latency is 6x step 1. The latency comparison
  across steps is not clean.
* 8 fast empty replies are unexplained. n05 was also empty in the baseline and in step 1,
  but the count rose. Their `response.reaction.snapped` events were checked by transport:
  a time-window match wrongly paired two live Discord turns (03:50:49 and 03:59:33,
  transport `discord`) and skewed others by one turn. The MCP-transport snaps match cases
  that returned marks. So there is no evidence the MCP reply drops marks, and the
  empties need their own trace-level look.

## Against the prediction
40-65% S: held at 48%. G above 90%: missed at 88%. N at or under 5%: held at 0%.
Snapped under 1.5s: held at 1.06s.

## Inference, not measured
The Jev shape path carries the social half of the set. The other half (yes/no,
agreement) needs the model path, which step 1 showed does not use marks. Reaching 90%
needs the model-path fix the game-dev seat named (a carve-out in the "no emojis" style
line), or a Jev-side `agree`/`disagree` for questions it can answer without tools.
