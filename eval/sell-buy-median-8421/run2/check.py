"""Mechanical half of the run 2 grade: per reply, every stage row checked against
expected.tsv, and every number that is neither a stage label, an expected median,
nor a trade count listed as a stray figure. The verdict is read by hand from this
output and the reply, never taken from it. Usage: check.py results.jsonl"""

import csv
import json
import re
import sys
from collections import defaultdict
from pathlib import Path

HERE = Path(__file__).resolve().parent
expected: dict[str, dict[str, tuple[str, str]]] = defaultdict(dict)
with open(HERE.parent / "expected.tsv") as f:
    rows = [r for r in csv.reader(f, delimiter="\t") if r and not r[0].startswith("#")]
for item, stage, n, _cycles, spectres in rows[1:]:
    expected[item][stage] = (n, spectres)

ROW = re.compile(r"^\s*[-*]?\s*\**(?P<stage>none|no upgrade|(?:Basic|Advanced|Modern) [1-4])\**:?\s*(?P<fig>[\d.]+|too few trades(?:\s*\(<\s*5\))?)\s*\((?P<n>\d+)[^\n]*", re.I | re.M)
# price_by_stage's one-line template: "Basic 2 1.05 (28), 3 0.82 (36). no upgrade too few (3)."
CELL = r"(?P<fig>[\d.]+|too few) \((?P<n>\d+)\)"
TIER = re.compile(r"(?P<tier>Basic|Advanced|Modern) (?P<cells>[1-4] [\d.a-z ]+? \(\d+\)(?:, [1-4] [\d.a-z ]+? \(\d+\))*)")
NONE = re.compile(r"no upgrade " + CELL)
INNER = re.compile(r"(?P<num>[1-4]) " + CELL)
# A model-written markdown table: "| Basic 4 | 219.13 | 13 |".
TABLE = re.compile(r"^\|\s*(?P<stage>none|no upgrade|(?:Basic|Advanced|Modern) [1-4])\s*\|\s*(?P<fig>[\d.]+|too few trades)\s*\|\s*(?P<n>\d+)\s*\|[^\n]*", re.I | re.M)
NUM = re.compile(r"(?<![\w.])\d+(?:\.\d+)?")
# Digits that belong to the stage labels, the item name, or the MIN_N notice.
LABEL = re.compile(r"(?:Basic|Advanced|Modern|Upgrade)\s+[1-4]|<\s*5|Cycle\s+\d+|\b\d+\s+trades?", re.I)

for line in open(sys.argv[1]):
    r = json.loads(line)
    res = r["result"]
    reply = (res.get("structuredContent") or {}).get("reply") or ""
    notice = " ".join(c.get("text", "") for c in res.get("content", []) if res.get("isError"))
    exp = expected.get(r["expect"], {})
    seen, bad = set(), []
    cells = [(m["stage"], m["fig"], m["n"]) for m in ROW.finditer(reply)]
    cells += [(m["stage"], m["fig"], m["n"]) for m in TABLE.finditer(reply)]
    cells += [("none", m["fig"], m["n"]) for m in NONE.finditer(reply)]
    for t in TIER.finditer(reply):
        cells += [(f"{t['tier']} {c['num']}", c["fig"], c["n"]) for c in INNER.finditer(t["cells"])]
    for raw_stage, raw_fig, raw_n in cells:
        stage = "none" if raw_stage.lower() in ("none", "no upgrade") else raw_stage.title()
        seen.add(stage)
        want_n, want_fig = exp.get(stage, ("?", "?"))
        fig = raw_fig.strip()
        fig_ok = (fig.startswith("too few") and want_fig == "too few") or (
            not fig.startswith("too few") and want_fig != "too few" and abs(float(fig) - float(want_fig)) < 0.006
        )
        if raw_n != want_n or not fig_ok:
            bad.append(f"{stage}: got {fig} ({raw_n}) want {want_fig} ({want_n})")
    # Whole stage lines out first, then any number left is outside the table,
    # even when it equals a stage median (a "0.30-0.38" summary is two of them).
    stray = NUM.findall(LABEL.sub("", NONE.sub("", TIER.sub("", TABLE.sub("", ROW.sub("", reply))))))
    missing = sorted(set(exp) - seen)
    print(f"== {r['id']} {r['expect']} secs={r['secs']} isError={res.get('isError')}")
    print(f"   stage rows {len(seen)}/{len(exp)}  mismatched {bad or 0}  missing {missing or 0}  stray numbers {stray or 0}")
    if notice:
        print(f"   notice: {notice[:200]}")
