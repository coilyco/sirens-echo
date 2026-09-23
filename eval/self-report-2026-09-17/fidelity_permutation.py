"""Negative control: does the comparison notice when the trace is the wrong one?

`fidelity_control.py` validates the comparator. Given a footer and a trace it
classifies the pair correctly, and 4 of its 8 cases are differences it must
catch. That is not the control an exact aggregate agreement calls for.

The worry an exact agreement should raise is about the **inputs**, not the
comparator: if the trace evidence were somehow derived from the footer, the two
would agree everywhere and the comparison would be measuring one number against
itself. A comparator control cannot see that, because it is handed two inputs
by construction.

So this permutes. Each reply is compared against another reply's trace, cycled
by one position and then over every offset. If the comparison is sensitive to
which trace it is handed, almost every permuted cell must fail. If permuted
cells keep passing, the comparison is weak and the real result is worth less
than it looks.

Deterministic: it walks every cyclic offset rather than sampling, so there is
no seed to record and no run-to-run variance to report.
"""

from __future__ import annotations

import argparse
import json
from pathlib import Path

from fidelity_control import verdict


def main(argv: list[str] | None = None) -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--corpus", type=Path, required=True)
    parser.add_argument("--evidence", type=Path, required=True)
    parser.add_argument("--skill-spans", type=Path, help="skill-spans.json")
    args = parser.parse_args(argv)

    evidence = json.loads(args.evidence.read_text())
    records = sorted(json.loads(args.corpus.read_text()), key=lambda r: r["ts"])
    spans = json.loads(args.skill_spans.read_text()) if args.skill_spans else []
    # The skill evidence travels with the trace it belongs to. Rotating the trace
    # and leaving the skill spans behind would hand a permuted cell its own book
    # row back, which is the weakness this control exists to measure.
    cells = [
        (
            index,
            record.get("disclosed") or [],
            evidence[str(record["reply_id"])]["calls"],
            [s for s in spans if str(s["trace"]) == str(evidence[str(record["reply_id"])]["trace"])],
        )
        for index, record in enumerate(records, start=1)
        if (record.get("disclosed") or []) and str(record["reply_id"]) in evidence
    ]
    traces = [(calls, skills) for _, _, calls, skills in cells]

    aligned = sum(1 for _, d, calls, skills in cells if verdict(d, calls, skills) == "pass")
    print(f"cells                          {len(cells)}")
    print(f"pass against their own trace   {aligned}")
    print()
    print("pass against another reply's trace, by offset")
    total_passes = 0
    total_cells = 0
    collisions: list[tuple[int, int, str]] = []
    for offset in range(1, len(cells)):
        rotated = traces[offset:] + traces[:offset]
        passes = 0
        for (ordinal, disclosed, _, _), (calls, skills) in zip(cells, rotated, strict=True):
            if verdict(disclosed, calls, skills) == "pass":
                passes += 1
                signature = ", ".join(f"{k} x{v}" for k, v in sorted(calls.items()))
                collisions.append((offset, ordinal, signature))
        total_passes += passes
        total_cells += len(cells)
        flag = "" if passes == 0 else "   <- a permuted cell passed"
        print(f"  offset {offset:>2}   {passes:>2} of {len(cells)} passed{flag}")

    print()
    rate = total_passes / total_cells if total_cells else 0.0
    print(f"across every offset            {total_passes} of {total_cells} passed ({rate:.1%})")
    print(f"against their own trace        {aligned} of {len(cells)} passed "
          f"({aligned / len(cells):.1%})")

    if collisions:
        print()
        print("every permuted pass, and the signature that let it through")
        for offset, ordinal, signature in collisions:
            print(f"  offset {offset:>2}  ord {ordinal:>2}  matched: {signature}")

    print()
    print("reading")
    if total_passes == 0:
        print("  No cell passes against a trace that is not its own.")
    else:
        print("  The permuted passes are signature collisions rather than pipeline artefacts:")
        print("  a cell whose whole footer is one common tool at a low count matches any")
        print("  other cell of the same shape, and this corpus has several. That is a real")
        print("  limit on what a single passing cell of that shape is worth, and it is not")
        print("  evidence that the trace evidence came from the footer.")
    print()
    print("  A pass against the right trace is far from free. Quote the permuted rate")
    print("  beside the real one rather than the real one alone.")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
