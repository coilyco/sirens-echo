# Run 4 prereg  2026-09-28T06:28:43Z

Lane, read at 06:28Z:
* Echo pod sirens-echo-84f67f6f95-vcb2d runs image 20a40d3 (k3s pod read), which
  carries #1264 (when_unmatched) and #1266 (argument spans).
* The live eco-app behaves as be79c2d: "masonry au5" resolves to Masonry Advanced
  Upgrade, and "bu5" gives 21 candidates.
* price_by_stage matches run4/expected-run4.tsv on 156 of 156 stage rows across 17 items
  (run4/tool-vs-expected-be79c2d.txt).

Changed vs run 3: the Echo image (#1264, #1266) and eco-app e8a65e7 then be79c2d
(upgrade-module routing, shorthand, stage qualifier, recipe-only items as "no
recorded trades").

Probes: all 25 (probes.tsv, probes-phrasing.tsv, probes-8425.tsv), one pass, run4/run.py.
Rule: README as for run 3, plus the probes-8425.tsv expectations. A qualifier probe
leads with its stage row and the market-not-your-cost clause, and other stages may
be cut by the 280-character cap.

EXPECTED, written before the first turn:
* Core 12: 12 of 12. P10 reaches the template (pick p .98 per eb64), and P11 and P12
  get the when_unmatched miss.
* Phrasing: Q01 and Q02 pass. Q03 does not pass, because #8431 is not built.
  Partial (one item's block) is more likely than fail.
* Shorthand: 8 of 10. The two most at risk are the ones with no sell or buy verb:
  S07 "what's mu0 worth?" and S02 "price check AU 3".
