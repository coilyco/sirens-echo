# Run 3 prereg  2026-09-28T06:03:39Z

Lane: the deploy repository at 0d26e302 pins sirens-echo image f3152f4. That is
runs 1-2's harness plus eval-only commits, so sirens-echo #1264 (when_unmatched) is
not live. eco-app main is df11c6d. The live price_by_stage shows the 88d4723 estimate
shape from 06:02:48Z, and a direct read at 06:03Z matched run3/expected-run3.tsv on
134 of 134 stage rows (run3/tool-vs-expected.txt). Whether df11c6d itself is live
does not matter here, because its template needs #1264.

Changed vs run 2: eco-app estimates and floor (#8423), and price_by_stage's
description now covers pay, buy, good price and worth (#8424 item 1).

Probes: the core 12 (probes.tsv) and the 3 phrasing probes (probes-phrasing.tsv),
one sequential pass, run3/run.py over raw JSON-RPC.
Rule: README, run 1 rule plus Amendments 1 and 2 plus game-dev's run 3 calls. Every
figure is checked against run3/expected-run3.tsv. A no-item answer that names another
game fails. A real item answered "couldn't match" fails.

EXPECTED, written before the first turn:
* Core real items (P01-P10): 9 of 10 pass. All 10 take the template path, and the
  one miss is on the model path if any probe still goes there.
* Phrasing: Q01 and Q02 pass on the template. Q03 (two items) is partial, because
  the template fills one item.
* No-item (P11-P12): 1 of 2. Both take the model path, since #1264 is not live. P11
  names another game again (3 of 3 answers so far did) and fails. P12 passes.
* 0 out-of-budget turns.
