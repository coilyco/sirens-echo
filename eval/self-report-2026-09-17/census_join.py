"""Join the message census to the span census, record by record.

Two seats counted this corpus by two routes that never touched each other. From
the message side: how many records carry no reply of Deep's, whether because
the service spoke instead or because the body is empty. From the span side: how
many records carry no `discord.reply` span. The two counts are consistent, and
consistent is not checked, because nothing has confirmed they are the same
records.

`deep-replies.jsonl` carries no trace field, so the seat holding the message
census cannot run this. This is the join, and what it settles is the claim that
the instrument gap is one reply rather than four or seven.

Filed as `teable:coilyco-flight-deck/housecast#7560`. Reads the same uncommitted
corpus `derive.py` does, plus the span census, and prints ordinals rather than
Discord ids so its output is committable.
"""

from __future__ import annotations

import argparse
import json
from pathlib import Path

from derive import SERVICE_NOTICE


def main(argv: list[str] | None = None) -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--corpus", type=Path, required=True)
    parser.add_argument("--spans", type=Path, required=True, help="discord_reply and receive-only")
    args = parser.parse_args(argv)

    spans = json.loads(args.spans.read_text())
    replied = {str(i) for i in spans["discord_reply"]}
    received = {str(i) for i in spans["discord_receive_only"]}
    records = sorted(json.loads(args.corpus.read_text()), key=lambda r: r["ts"])

    rows = []
    for ordinal, record in enumerate(records, start=1):
        text = str(record.get("reply") or "")
        if not text.strip():
            message = "empty body"
        elif SERVICE_NOTICE.match(text.strip()):
            message = "service notice"
        else:
            message = "Deep prose"
        reply_id = str(record["reply_id"])
        if reply_id in replied:
            span = "discord.reply"
        elif reply_id in received:
            span = "discord.receive only"
        else:
            span = "no span"
        rows.append((ordinal, message, span, len(text)))

    non_reply = [r for r in rows if r[1] != "Deep prose"]
    no_reply_span = [r for r in rows if r[2] != "discord.reply"]
    both = [r for r in no_reply_span if r[1] != "Deep prose"]
    gap = [r for r in no_reply_span if r[1] == "Deep prose"]
    traced_non_reply = [r for r in non_reply if r[2] == "discord.reply"]

    print(f"records                                  {len(rows)}")
    print(f"message census, no reply of Deep's       {len(non_reply)}")
    print(f"span census, no discord.reply span       {len(no_reply_span)}")
    print(f"  of which no span at all                {sum(1 for r in rows if r[2] == 'no span')}")
    print(f"  of which discord.receive only          {sum(1 for r in rows if r[2] == 'discord.receive only')}")
    print()
    print(f"in BOTH censuses                         {len(both)}")
    for ordinal, message, span, chars in both:
        print(f"  ordinal {ordinal:>2}  {message:<15} {span:<21} {chars:>5} chars")
    print()
    print(f"no discord.reply span, and Deep DID speak  {len(gap)}")
    for ordinal, message, span, chars in gap:
        print(f"  ordinal {ordinal:>2}  {message:<15} {span:<21} {chars:>5} chars")
    print()
    print(f"no reply of Deep's, and a discord.reply span exists  {len(traced_non_reply)}")
    for ordinal, message, span, chars in traced_non_reply:
        print(f"  ordinal {ordinal:>2}  {message:<15} {span:<21} {chars:>5} chars")
    print()
    print("verdict")
    print(f"  {len(both)} of the {len(no_reply_span)} without a discord.reply span had no reply to miss.")
    print(f"  The instrument gap is {len(gap)} reply, not {len(no_reply_span)} and not"
          f" {sum(1 for r in rows if r[2] == 'no span')}.")
    print(f"  The {len(traced_non_reply)} non-replies that DO carry a span are the suppressed class:")
    print("  the service emitted a reply message, and its content is a block notice.")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
