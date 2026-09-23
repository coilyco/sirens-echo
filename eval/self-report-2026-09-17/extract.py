"""Re-derive the corpus disclosure fields from `all-raw.json`, the ground truth.

`deep-replies.json` and `.jsonl` carry `disclosed`, `disclosed_lines` and
`disclosed_calls`, and those three were written by an extractor that recognised
one glyph. This rewrites them through `footer.py`, which recognises the row
shape. The MANIFEST's rule holds: a correction to a derived field is made by
re-deriving rather than by patching.

**The join is checked rather than assumed.** Each corpus record's `reply` must be
byte-identical to the raw Discord message's `content` for the same id, and this
refuses to write if any of them is not. That check is what lets the disclosure
fields be re-derived from the raw pull without reimplementing the rest of the
corpus build, which has no committed script: prompt pairing, `prompt_author`,
`channel` and `embeds` are carried through from the existing file untouched.

Those four are not re-derivable at all, and that is settled rather than owed.
Kai declined a committed pull script on 2026-09-12: the Discord pull does not get
repeated, the corpus lives where it lives, and losing the copy is an accepted
outcome. So the bucket holds the only copy, the Echo channel grant does not need
reinstating, and a defect in a field this script does not touch stays. Recorded
on `teable:coilyco-flight-deck/housecast#7571`, closed as declined.

    uv run --all-extras python evaluations/self-report-2026-09-17/extract.py \
      --raw <dir>/all-raw.json --corpus <dir>/deep-replies.json --out <dir>

Inputs and outputs are uncommitted for the reason `derive.py` gives.
"""

from __future__ import annotations

import argparse
import json
from pathlib import Path

from footer import as_disclosed, parse_footer


def main(argv: list[str] | None = None) -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--raw", type=Path, required=True, help="all-raw.json")
    parser.add_argument("--corpus", type=Path, required=True, help="deep-replies.json")
    parser.add_argument("--out", type=Path, required=True, help="directory to write both files")
    args = parser.parse_args(argv)

    raw = {str(message["id"]): message for message in json.loads(args.raw.read_text())}
    records = sorted(json.loads(args.corpus.read_text()), key=lambda record: record["ts"])

    absent = [str(r["reply_id"]) for r in records if str(r["reply_id"]) not in raw]
    drifted = [
        str(r["reply_id"])
        for r in records
        if str(r["reply_id"]) in raw
        and str(raw[str(r["reply_id"])].get("content") or "") != str(r.get("reply") or "")
    ]
    print(f"raw messages         {len(raw)}")
    print(f"corpus records       {len(records)}")
    print(f"  absent from raw    {len(absent)}")
    print(f"  text differing     {len(drifted)}")
    if absent or drifted:
        print()
        print("REFUSING to write: the corpus reply text is not the raw text.")
        for reply_id in (absent + drifted)[:10]:
            print(f"  {reply_id}")
        return 1

    before_rows = sum(len(r.get("disclosed") or []) for r in records)
    before_runs = sum(
        int(line.get("runs", 1)) for r in records for line in (r.get("disclosed") or [])
    )
    changed: list[tuple[int, int, int, int, int]] = []
    for ordinal, record in enumerate(records, start=1):
        was = record.get("disclosed") or []
        rows = parse_footer(str(raw[str(record["reply_id"])].get("content") or ""))
        now = as_disclosed(rows)
        record["disclosed"] = now
        record["disclosed_lines"] = len(now)
        record["disclosed_calls"] = sum(row.runs for row in rows)
        if len(was) != len(now) or sum(int(line.get("runs", 1)) for line in was) != sum(
            row.runs for row in rows
        ):
            changed.append(
                (
                    ordinal,
                    len(was),
                    len(now),
                    sum(int(line.get("runs", 1)) for line in was),
                    sum(row.runs for row in rows),
                )
            )

    after_rows = sum(len(r["disclosed"]) for r in records)
    after_runs = sum(r["disclosed_calls"] for r in records)
    print()
    print(f"footer rows          {before_rows} -> {after_rows}")
    print(f"footer runs          {before_runs} -> {after_runs}")
    print(f"replies disclosing   {sum(1 for r in records if r['disclosed'])}")
    print(f"records changed      {len(changed)}")
    for ordinal, was_rows, now_rows, was_runs, now_runs in changed:
        print(f"  ord {ordinal:>2}  rows {was_rows} -> {now_rows}   runs {was_runs} -> {now_runs}")

    kinds: dict[str, int] = {}
    for record in records:
        for line in record["disclosed"]:
            kinds[line["kind"]] = kinds.get(line["kind"], 0) + 1
    print(f"rows by kind         {kinds}")

    args.out.mkdir(parents=True, exist_ok=True)
    (args.out / "deep-replies.json").write_text(json.dumps(records, indent=2) + "\n")
    with (args.out / "deep-replies.jsonl").open("w") as handle:
        for record in records:
            handle.write(json.dumps(record) + "\n")
    print()
    print(f"written to           {args.out}")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
