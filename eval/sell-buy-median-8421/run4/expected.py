"""Expected per-stage figures for run 3, recomputed from eco-app docs/price-history.md
("Median price per stage", at 88d4723) rather than from eco-app's code. Usage:
expected.py <norms.json> > expected-run4.tsv"""

import json
import sys

STAGES = ["none"] + [f"{t} {n}" for t in ("Basic", "Advanced", "Modern") for n in range(1, 5)]
MIN_N = 5
ITEMS = ["Iron Bar", "Copper Bar", "Hewn Log", "Lumber", "Bread", "Steel Bar", "Brick", "Nail",
         "Glass", "Basic Upgrade 4", "Flat Steel", "Copper Wiring", "Gold Bar",
         "Advanced Upgrade 3", "Scholars Basic Upgrade 4", "Scholars Modern Upgrade 2", "Nylon Fabric"]

d = json.load(open(sys.argv[1]))
index = {c["cycle"]: c["basketIndex"] for c in d["cycles"]}[14]


def fill(anchors: dict[int, float], lo: int) -> dict[int, float]:
    """Linear on stage index between anchors, flat carry past the outermost, from lo to Modern 4."""
    keys = sorted(anchors)
    out = {}
    for s in range(lo, len(STAGES)):
        if s in anchors:
            out[s] = anchors[s]
        elif s < keys[0]:
            out[s] = anchors[keys[0]]
        elif s > keys[-1]:
            out[s] = anchors[keys[-1]]
        else:
            a = max(k for k in keys if k < s)
            b = min(k for k in keys if k > s)
            out[s] = anchors[a] + (anchors[b] - anchors[a]) * (s - a) / (b - a)
    return out


print(f"# docs/price-history.md at eco-app 88d4723, cycle 14 basketIndex {index}")
print("item\tstage\tkind\tn\tspectres")
for item in ITEMS:
    e = d["items"][item]
    cross = e["crossCycle"]
    seen = set(cross) | {s for c in e["cycles"].values() for s in c["stages"]}
    floor = min(STAGES.index(s) for s in seen)
    real = {STAGES.index(s): v["median"] for s, v in cross.items() if v["n"] >= MIN_N}
    anchors = real or {STAGES.index(s): v["median"] for s, v in cross.items()}
    for s, basket in fill(anchors, floor).items():
        stage = STAGES[s]
        kind = "real" if s in real else "est"
        n = cross.get(stage, {}).get("n", 0)
        print(f"{item}\t{stage}\t{kind}\t{n}\t{round(basket * index, 2):.2f}")
