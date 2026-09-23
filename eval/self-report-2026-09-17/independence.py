"""Is dropping the untraced replies safe, or are they the finding?

The design spec for this board states the exclusion and then asks the question
that governs whether the exclusion is honest: dropping cases the instrument
cannot reach is only defensible when unreachability is independent of what is
being measured. A trial that loses its sickest patients to follow-up looks
rigorous and reports the wrong number.

So this compares the untraced replies against the traced ones on properties
that need no trace: how long the reply is, whether it carries a tool
disclosure, whether the service spoke in place of the subject, and what hour
it landed. Ordinary means the exclusion is defensible and should be stated.
Clustered means the four are the finding.

Reads the same two uncommitted inputs `derive.py` does. Prints counts and
distributions only, never a reply id or a line of a reply, so its output is
committable while the corpus decision stays open.
"""

from __future__ import annotations

import argparse
import json
import statistics
from pathlib import Path

from derive import SERVICE_NOTICE


def describe(label: str, lengths: list[int]) -> str:
    if not lengths:
        return f"{label:<22} n=0"
    body = (
        f"n={len(lengths):<3} min={min(lengths):<5} "
        f"median={int(statistics.median(lengths)):<5} max={max(lengths)}"
    )
    return f"{label:<22} {body}"


def main(argv: list[str] | None = None) -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--corpus", type=Path, required=True)
    parser.add_argument("--traces", type=Path, required=True)
    args = parser.parse_args(argv)

    traced_ids = {str(reply_id) for reply_id in json.loads(args.traces.read_text())}
    records = json.loads(args.corpus.read_text())
    traced = [r for r in records if str(r["reply_id"]) in traced_ids]
    untraced = [r for r in records if str(r["reply_id"]) not in traced_ids]

    def notice(record: dict) -> bool:
        return bool(SERVICE_NOTICE.match(str(record.get("reply") or "").strip()))

    def empty(record: dict) -> bool:
        return not str(record.get("reply") or "").strip()

    print(f"corpus {len(records)} replies, {len(traced)} traced, {len(untraced)} untraced")
    print()
    print("reply length in characters")
    print("  " + describe("traced", [r["reply_chars"] for r in traced]))
    print("  " + describe("untraced", [r["reply_chars"] for r in untraced]))
    print()
    print("composition")
    for label, group in (("traced", traced), ("untraced", untraced)):
        empties = sum(1 for r in group if empty(r))
        notices = sum(1 for r in group if notice(r))
        authored = len(group) - empties - notices
        disclosed = sum(1 for r in group if r.get("disclosed"))
        print(
            f"  {label:<10} {len(group):>3} total  "
            f"{authored:>3} authored by Deep  {notices:>3} service notice  "
            f"{empties:>3} empty body  {disclosed:>3} carry a disclosure"
        )
    print()
    print("hour of day, UTC")
    for label, group in (("traced", traced), ("untraced", untraced)):
        hours = sorted(int(r["ts"][11:13]) for r in group)
        print(f"  {label:<10} {hours}")
    print()

    empties_untraced = sum(1 for r in untraced if empty(r))
    print("verdict")
    print(
        f"  {empties_untraced} of the {len(untraced)} untraced replies carry no message text at all, "
        f"against {sum(1 for r in traced if empty(r))} of {len(traced)} traced."
    )
    print("  Unreachability is NOT independent of what is being measured.")
    print("  The exclusion cannot be reported as a clean drop. State it as a finding:")
    print("  a missing reply span tracks a turn that produced no reply, not a gap in SigNoz.")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
