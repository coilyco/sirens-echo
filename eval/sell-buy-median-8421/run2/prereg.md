# Run 2 prereg  2026-09-28T05:35:52Z

Lane: the deploy repository at 9cac7ae9 pins sirens-echo image 5f6626d, which is
run 1's 12d518f plus the eval-only #1261. The eco-game tools come from the live
public_coilyco_eco, carrying price_by_stage at eco-app 38e571c. A direct call at
05:35Z resolved "a basic upgrade 4", which only 38e571c does, and returned the
expected.tsv figures for Basic Upgrade 4 and Hewn Log.

Only variable changed vs run 1: eco-app gains price_by_stage and the server
instruction "answer from price_by_stage alone". Echo's harness and knowledge are unchanged.
Instrument change: run2/run.py calls raw_turn.sh, so an error notice is kept.
The rule does not change.

Criterion: ../README.md, run 1 rule plus the amendment (Spectres, n<5 carries no
number, candidate names allowed on a no-item reply). Each stated median is checked
against expected.tsv.

Seen before this prereg: prod-director's spot probe of "hewn logs" at about 05:30Z
returned the stage table plus "the most common price point is around 0.30-0.38
Spectres". Under this rule that is a fail, and it informs the prediction below.

EXPECTED, written before the first turn:
* Real items (P01-P10): 5 of 10 pass. Every one calls price_by_stage and carries
  the correct table. The fails come from summary text that adds a range or a
  "typical" price, as in the spot probe.
* 0 out-of-budget turns, because price_by_stage's payload is small.
* No-item (P11-P12): 2 of 2 pass, because the server instruction says to stop.
