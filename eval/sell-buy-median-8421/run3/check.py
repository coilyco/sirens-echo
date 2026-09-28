"""Mechanical half of the run 3 grade, against run3/expected-run3.tsv. Per reply, per
item: every stage row found, its figure, whether it is marked as an estimate, and its
count, checked against the expected row; stages from the floor to Modern 4 that are
missing; rows below the floor; and every number outside the rows. The verdict is read
by hand from this and the reply. Usage: check.py results.jsonl"""

import csv
import json
import re
import sys
from collections import defaultdict
from pathlib import Path

HERE = Path(__file__).resolve().parent
TIERS = ("Basic", "Advanced", "Modern")
STAGES = ["none"] + [f"{t} {n}" for t in TIERS for n in range(1, 5)]
expected: dict[str, dict[str, tuple[str, str, str]]] = defaultdict(dict)
with open(HERE / "expected-run3.tsv") as f:
    rows = [r for r in csv.reader(f, delimiter="\t") if r and not r[0].startswith("#")]
for item, stage, kind, n, spectres in rows[1:]:
    expected[item][stage] = (kind, n, spectres)

FIG = r"(?P<est>~)?\s*(?P<fig>\d+(?:\.\d+)?)(?P<est2>\s*\(?est\.?\)?)?(?:\s*\((?P<n>\d+)(?:\s*trades?)?\))?"
# Template: "Basic 1 ~1.05, 2 1.05 (28), 3 0.82 (36). Advanced 1 ..." and "no upgrade ~0.52".
TIER = re.compile(r"(?P<tier>Basic|Advanced|Modern) (?P<cells>[1-4] ~?[\d.]+(?: \(\d+\))?(?:, [1-4] ~?[\d.]+(?: \(\d+\))?)*)")
INNER = re.compile(r"(?P<num>[1-4]) " + FIG)
NONE = re.compile(r"no upgrade " + FIG)
# A model's bullet or table row: "- Basic 4: ~1.97 (est.)", "| Basic 4 | 1.31 | 348 |".
# [ \t] not \s: a \s here crosses the newline and swallows the next bullet.
LINE = re.compile(r"^[ \t]*[-*|]?[ \t]*\**(?P<stage>none|no upgrade|(?:Basic|Advanced|Modern) [1-4])\**[ \t]*[:|][ \t]*" + r"(?P<est>~)?[ \t]*(?P<fig>\d+(?:\.\d+)?)(?P<est2>[ \t]*\(?est\.?\)?)?[ \t]*(?:\|[ \t]*|\()?(?P<n>\d+)?[^\n]*", re.I | re.M)
BEFORE = re.compile(r"(?:none|not traded) before (?:Basic|Advanced|Modern) [1-4]", re.I)
NUM = re.compile(r"(?<![\w.])\d+(?:\.\d+)?")
LABEL = re.compile(r"(?:Basic|Advanced|Modern|Upgrade)\s+[1-4]|Cycle\s+\d+|\b\d+\s+trades?", re.I)


def cells_of(text: str) -> list[tuple[str, str, bool, str | None]]:
    out = []
    for m in NONE.finditer(text):
        out.append(("none", m["fig"], bool(m["est"] or m["est2"]), m["n"]))
    for t in TIER.finditer(text):
        for c in INNER.finditer(t["cells"]):
            out.append((f"{t['tier']} {c['num']}", c["fig"], bool(c["est"] or c["est2"]), c["n"]))
    for m in LINE.finditer(text):
        stage = "none" if m["stage"].lower() in ("none", "no upgrade") else m["stage"].title()
        out.append((stage, m["fig"], bool(m["est"] or m["est2"]), m["n"]))
    return out


def check(item: str, text: str) -> str:
    exp = expected[item]
    floor = min(STAGES.index(s) for s in exp)
    seen, bad = set(), []
    for stage, fig, est, n in cells_of(text):
        seen.add(stage)
        if stage not in exp:
            bad.append(f"{stage}: row outside floor..Modern 4 ({fig})")
            continue
        kind, want_n, want = exp[stage]
        if abs(float(fig) - float(want)) >= 0.006:
            bad.append(f"{stage}: {fig} want {want}")
        if kind == "est" and not est:
            bad.append(f"{stage}: estimate not marked")
        if kind == "real" and est:
            bad.append(f"{stage}: real median marked as estimate")
        if kind == "real" and n != want_n:
            bad.append(f"{stage}: count {n} want {want_n}")
    missing = [s for s in STAGES[floor:] if s not in seen]
    return f"{item}: rows {len(seen & set(exp))}/{len(exp)} bad {bad or 0} missing {missing or 0}"


for line in open(sys.argv[1]):
    r = json.loads(line)
    res = r["result"]
    reply = (res.get("structuredContent") or {}).get("reply") or ""
    notice = " ".join(c.get("text", "") for c in res.get("content", []) if res.get("isError"))
    items = [i.strip() for i in r["expect"].split("+")] if r["expect"] != "-" else []
    print(f"== {r['id']} {r['expect']} secs={r['secs']} isError={res.get('isError')}")
    for item in items:
        # A two-item reply is split at the second item's name, so each half is checked alone.
        part = reply
        if len(items) > 1:
            names = [re.escape(i) for i in items]
            spans = [(m.start(), m.group()) for m in re.finditer("|".join(names), reply)]
            starts = sorted({min(s for s, g in spans if g == i) for i in items if any(g == i for _, g in spans)})
            mine = [s for s, g in spans if g == item]
            if mine:
                a = min(mine)
                b = min([s for s in starts if s > a], default=len(reply))
                part = reply[a:b]
            else:
                part = ""
        print(f"   {check(item, part)}")
    stripped = BEFORE.sub("", reply)
    for rx in (LINE, NONE, TIER):
        stripped = rx.sub("", stripped)
    print(f"   stray numbers {NUM.findall(LABEL.sub('', stripped)) or 0}")
    if notice:
        print(f"   notice: {notice[:200]}")
