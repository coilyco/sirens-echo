# Prediction

Written 2026-09-12T16:57:59Z, before any cell on this board was graded. Committed so the
grading can falsify it rather than be read back through it.

## What is already settled, and is not a prediction

These were measured while building the board and are recorded in
`independence.txt` and `derive.txt`:

* 47 records, of which 39 carry prose Deep wrote
* 40 of 47 join a `discord.reply` span
* 15 carry a tool rollup in the body, and all 15 are traced
* the untraced set is enriched for empty and near-empty replies

## Predictions

**01 // `self-report-fidelity` passes on most of its 15.** The one reply
checked by hand during spec review disclosed nine calls against nine
pre-completion calls, exactly. I expect 11 to 15 passes, and I expect every
failure to be an understated count rather than a dropped tool class.

The reason to doubt it: the exclusion rule this dimension compares against has
not been written yet. It is a judged call the spec assigns to one human for
the whole board, and until it exists the measured pass has no rule to run
against. If it is written loosely, this dimension passes 15 of 15 and measures
nothing.

**02 // `failure-disclosure` is the dimension at risk of a ceiling.** The
service renders a failed call with a failed glyph whether or not the agent
mentions it, so the receipt is never missing and only the prose is in
question. I expect 34 to 38 passes out of 38, which is close enough to the
ceiling that a null result here is the likely outcome. The spec's own check
applies: if this passes 38 of 38, the board measured nothing on it and the
talk has no disagreement to show from this dimension.

**03 // `premise-correction` carries the fewest eligible premises.** Most
prompts in this corpus are ordinary questions with no false premise, and the
rubric passes those on the second clause alone. I expect fewer than 8 of the
38 to contain a premise worth correcting, and I expect graders to disagree
most here, because "was there a false premise" is prior to "was it corrected
well" and the rubric does not separate the two.

**04 // `bounded-refusal` is where a real fail lives if one does.** 39
scorable, the largest set, and the only dimension whose failure mode does not
need telemetry to see.

**Inter-rater.** I expect the two graders to agree on more than 85 percent of
cells overall, and for disagreement to concentrate in 03. If disagreement is
spread evenly across all four, the rubric is underspecified rather than the
cases being hard.

## The negative control

If `self-report-fidelity` and `failure-disclosure` both pass at ceiling, the
board has calibrated nothing and the two judged dimensions have no measured
anchor. That is a reportable outcome and not a reason to regrade.
