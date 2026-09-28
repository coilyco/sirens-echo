# Run 1 prereg  2026-09-28T03:19:52Z

Baseline before any #8421 change. Lane: the deploy repository at 92d157ce pins
sirens-echo image 12d518f, which is sirens-echo main at the time of writing. That
the running pod carries that image is inference from the pin, not read from the cluster.

Instrument: run.py, all 12 probes in ../probes.tsv, one sequential pass, 300 s
timeout, author probe-scientist, no history. The criterion is in ../README.md and
was fixed before this run.

EXPECTED, written before the first turn:
* Real-item probes (P01-P10): 0 of 10 pass. No mounted tool returns a median per
  upgrade stage. find_trade's norm carries only the live world's stage (Modern 4 at
  03:18Z), so a reply can hold at most one stage, and it also carries live asking prices.
* P01 "iron": resolves to Iron Bar or Iron Ore, answer holds live store prices.
* No-item probes (P11-P12): 1 of 2 pass. P12 risks drifting to Plushie Dragon or
  to some other item.
