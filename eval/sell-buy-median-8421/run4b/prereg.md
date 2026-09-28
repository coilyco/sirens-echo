# Run 4b prereg  2026-09-28T06:32:07Z

Repeat of the core 12 only, on the same lane as run 4 (Echo pod image 20a40d3, eco-app
be79c2d or 7f313d5, where #390 changes only the "masonry mu5" reply, which no probe
uses), before closing #8421 on a single pass. Same instrument (run4/run.py pointed here)
and the same rule. This prereg is written in the command that starts the pass, not
pushed first.
EXPECTED: 12 of 12 again, all 12 on the template or when_unmatched path in under 2 s.
