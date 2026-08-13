# The rate pack

`agent/rate-echo.yaml` and `agent/rate-deep.yaml`, run with `ward exec
rate-echo` and `ward exec rate-deep`. They measure how often an intermittent
behavior happens, gate no deployment, and are never wired into CI. See [the
battery](sirens-echo-battery.md) for the gate and [the
board](sirens-echo-board.md) for the graded layer.

## Why a third instrument

Neither existing one can hold an intermittent behavior. The battery hard-fails
a deployment, so a case that fails 13 percent of the time makes the gate flaky.
The board is human-graded, and grading is per-artifact attention, so it cannot
economically run one case fifteen times.

Before this existed, an observed rate lived only as prose in an issue body.
Nothing regenerated the number after a fix, and nothing noticed a fix that took
a rate from 13 percent to 4 percent rather than to zero.

## How a run is scored

A case declares its own `runs` and `max_failure_rate`. Each attempt is scored by
`community.ScoreEvaluationCase`, the same function the gate uses, so the two
instruments cannot drift. A rate for a check the gate does not apply would
measure something nobody enforces.

An attempt is one of three outcomes. A pass, a fail, or an error. An error is a
failure of the substrate rather than of the agent, such as a 502 from Agent
Proxy, and it is reported and excluded from the denominator. Counting one as a
behavioral failure corrupts the rate.

The run exits non-zero only when a case beats its declared ceiling, or when
every attempt of a case errored. An unmeasured case is not a passing case, and
reporting it as one would be certifying rather than measuring.

## The dataset is the evidence

Every reply is persisted verbatim. Three first-pass findings in the QA that
motivated this pack were defects in the check rather than the agent, and only
reading the text separated them. Provenance travels with it: a rate without its
definition, pack, model, and roster is not comparable to the next run.

Every failing check is recorded rather than the first. A run recorded a user ID
disclosure as a handle echo and read zero ID leaks.

## The promotion path

A case starts here to establish its rate. When a fix drives that rate to zero
and holds at high N, the case may move into `agent/evaluation-deep.yaml` as a
deterministic regression.

Do not promote on a small clean run. Passing 5 of 5 is not evidence of
determinism: five runs put a weak upper bound on the true rate and nothing
more. A behavior at 13 percent passes 5 of 5 about half the time.

Promotion is also where the battery's rules reattach. A promoted case must not
be able to fire on a correct reply, and its target set must be closed.

## Cost

One attempt is one completion plus up to six tool rounds. Affordable on demand,
which is why this is an invoked verb rather than a CI step.

## Provenance

What each field can be trusted to say, and why `image` stays `unrecorded`:
[rate dataset provenance](sirens-echo-rate-provenance.md).

## What it cannot measure

A tool that lied, a fixture's own payload, and any behavior with no
deterministic check: [rate blind spots](sirens-echo-rate-blind-spots.md).
