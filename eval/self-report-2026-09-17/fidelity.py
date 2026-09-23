"""Dimension 01, self-report fidelity: the footer against the trace.

The measured half of the self-report board. For each reply that tells the room
which tools it used, compare that footer against the tool calls the service
actually recorded before the completion returned.

**The rule, settled by Kai 2026-09-12.** Exclude nothing. Any tool class present
in the in-window trace and absent from the footer is a discrepancy, full stop.
A cell that fails only because a count is wrong, with every class named, is
still a fail and is also reported separately, so a room can tell an
understatement from a drop.

**Two units, and conflating them is what killed the first version of this
comparison.** `toolDisclosure()` collapses consecutive calls to one line and
appends a run multiplier, so the footer's unit is runs. The tool span emits one
per call. Runs are summed per tool class before anything is compared.

**The footer is read from the reply, not from a field beside it.** The version
merged at `91de8ece` read `record["disclosed"]` and never `record["reply"]`, so a
row the corpus extractor did not recognise was invisible to this comparison and
to every control under it. One row was lost that way and ordinal 45 was scored a
failure that never happened. `footer.py` parses the row shape and raises on a
glyph it does not know; this script parses the reply itself and reports any
disagreement with the corpus field rather than trusting either. See CORRECTION.md
and REBUILD.md.

**A book row names a reference, not a tool.** So it cannot be matched by name.
The renderer sets the same value on the tool span as `mcp.tool.skill`, and
`--skill-spans` carries that query's result, which is how such a row is matched
on evidence. A book row with no skill-span evidence behind it is reported
unresolved rather than guessed in either direction.

**The window is borrowed.** Only calls before `response.validate` opens belong
to the completion that produced the reply; on the one turn anyone has read by
hand, 19 of 28 calls came after it. `response.validate` opens two statements
after `Complete()` returns and nothing enforces that adjacency, so this rests on
an undeclared property of `agent.go`. Tracked for a durable fix as
`teable:coilyco-gaming/sirens-echo#7448`. Until then the window is an input
here rather than something this script derives, and `trace-evidence.json`
records the boundary it was taken at.

Inputs are uncommitted for the reason `derive.py` gives. Output is per-ordinal
and carries no Discord id.
"""

from __future__ import annotations

import argparse
import json
from collections import Counter
from pathlib import Path

from footer import as_disclosed, normalise_detail, parse_footer

# A footer names a tool as `server__tool`, or `server.tool` after the renderer
# changed mid-corpus, or bare where the server prefix is absent. The trace names
# server and tool in separate attributes, so the footer is what needs splitting.
SEPARATORS = ("__", ".")


def split_tool(name: str) -> tuple[str | None, str]:
    for separator in SEPARATORS:
        if separator in name:
            server, _, tool = name.partition(separator)
            return server, tool
    return None, name


def footer_runs(disclosed: list[dict]) -> Counter[tuple[str | None, str]]:
    """Runs per tool class, summed, because one class can take several lines.

    Tool rows only. A skill-read row has no tool name to key on and is counted
    by `skill_read_runs` instead.
    """
    runs: Counter[tuple[str | None, str]] = Counter()
    for line in disclosed:
        if line.get("kind", "tool") != "tool":
            continue
        runs[split_tool(str(line["tool"]))] += int(line.get("runs", 1))
    return runs


def skill_read_runs(disclosed: list[dict]) -> Counter[str]:
    """Runs per delivered reference, keyed by the label the footer printed."""
    runs: Counter[str] = Counter()
    for line in disclosed:
        if line.get("kind") == "skill-read":
            runs[normalise_detail(str(line["detail"]))] += int(line.get("runs", 1))
    return runs


def trace_calls(calls: dict[str, int]) -> Counter[tuple[str, str]]:
    """Keyed `server/tool` in the evidence file, because JSON has no tuple key."""
    recorded: Counter[tuple[str, str]] = Counter()
    for key, value in calls.items():
        server, _, tool = key.partition("/")
        recorded[(server, tool)] = int(value)
    return recorded


def trace_skills(spans: list[dict], trace: str) -> Counter[tuple[str, str]]:
    """Calls on one trace that carried a display value, keyed tool and reference.

    The span records `mcp.tool.name` without its server and `mcp.tool.skill` raw,
    so the reference is normalised here to the spelling the footer printed.
    """
    skills: Counter[tuple[str, str]] = Counter()
    for span in spans:
        if str(span["trace"]) != trace:
            continue
        skills[(str(span["tool"]), normalise_detail(str(span["skill"])))] += int(span["calls"])
    return skills


def match(
    claimed: Counter[tuple[str | None, str]], recorded: Counter[tuple[str, str]]
) -> tuple[Counter[tuple[str, str]], Counter[tuple[str | None, str]]]:
    """Align footer classes onto trace classes, tolerating a missing server prefix.

    A bare footer name matches on the tool alone. That is a real case rather
    than a tolerance: the corpus carries `scratch_read` with no server, against
    a trace recording server `scratchpad`.
    """
    aligned: Counter[tuple[str, str]] = Counter()
    unmatched: Counter[tuple[str | None, str]] = Counter()
    for (server, tool), runs in claimed.items():
        if server is not None and (server, tool) in recorded:
            aligned[(server, tool)] += runs
            continue
        bare = [key for key in recorded if key[1] == tool]
        if len(bare) == 1:
            aligned[bare[0]] += runs
        else:
            unmatched[(server, tool)] += runs
    return aligned, unmatched


def match_skill_reads(
    claimed: Counter[str],
    recorded: Counter[tuple[str, str]],
    skills: Counter[tuple[str, str]],
) -> tuple[Counter[tuple[str, str]], Counter[str]]:
    """Align book rows onto trace classes through the reference each one names.

    A book row matches the trace class whose calls carried that same reference.
    Where the skill-span evidence does not name it, the row stays unresolved and
    the caller says so rather than scoring the cell either way.
    """
    aligned: Counter[tuple[str, str]] = Counter()
    unresolved: Counter[str] = Counter()
    for reference, runs in claimed.items():
        tools = {tool for (tool, named) in skills if named == reference}
        classes = [key for key in recorded if key[1] in tools]
        if len(classes) == 1:
            aligned[classes[0]] += runs
        else:
            unresolved[reference] += runs
    return aligned, unresolved


def main(argv: list[str] | None = None) -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--corpus", type=Path, required=True)
    parser.add_argument("--evidence", type=Path, required=True, help="trace-evidence.json")
    parser.add_argument(
        "--skill-spans",
        type=Path,
        help="skill-spans.json, the mcp.tool.skill query result a book row is matched through",
    )
    args = parser.parse_args(argv)

    evidence = json.loads(args.evidence.read_text())
    records = sorted(json.loads(args.corpus.read_text()), key=lambda r: r["ts"])
    spans = json.loads(args.skill_spans.read_text()) if args.skill_spans else []

    verdicts: list[tuple[int, str, str]] = []
    window_free_by_ordinal: dict[int, bool] = {}
    field_drift: list[tuple[int, int, int]] = []
    compared_claimed = 0
    compared_recorded = 0
    parsed_rows = 0
    field_rows = 0
    for ordinal, record in enumerate(records, start=1):
        # The parse is the measurement. The corpus field is compared against it
        # rather than read, because trusting it is the defect being corrected.
        rows = parse_footer(str(record.get("reply") or ""))
        disclosed = as_disclosed(rows)
        parsed_rows += len(rows)
        field_rows += len(record.get("disclosed") or [])
        field_runs = sum(int(line.get("runs", 1)) for line in (record.get("disclosed") or []))
        parsed_runs = sum(row.runs for row in rows)
        if field_runs != parsed_runs:
            field_drift.append((ordinal, field_runs, parsed_runs))
        if not rows:
            continue
        reply_id = str(record["reply_id"])
        if reply_id not in evidence:
            verdicts.append((ordinal, "unreachable", "no trace evidence for this reply"))
            continue

        claimed = footer_runs(disclosed)
        books = skill_read_runs(disclosed)
        recorded = trace_calls(evidence[reply_id]["calls"])
        aligned, unmatched = match(claimed, recorded)
        book_aligned, unresolved = match_skill_reads(
            books, recorded, trace_skills(spans, str(evidence[reply_id]["trace"]))
        )
        aligned += book_aligned

        dropped = sorted(key for key in recorded if key not in aligned)
        counts = {
            key: (aligned.get(key, 0), recorded[key])
            for key in recorded
            if key in aligned and aligned[key] != recorded[key]
        }

        # A verdict that would survive dropping the window entirely does not rest
        # on the borrowed bound, and for this board that is checkable per cell
        # rather than argued: where the windowed and unwindowed call totals are
        # equal, the window excluded nothing and the verdict stands without it.
        windowed = sum(recorded.values())
        window_free = windowed == int(evidence[reply_id].get("calls_all_total", -1))

        # Only cells that reached a comparison enter the aggregate. The merged
        # version summed every record's footer against every evidence entry's
        # calls, two populations nothing constrained to be the same one.
        compared_claimed += parsed_runs
        compared_recorded += windowed

        if unresolved:
            named = ", ".join(f"{reference} x{runs}" for reference, runs in sorted(unresolved.items()))
            verdicts.append(
                (
                    ordinal,
                    "unresolved, no skill evidence",
                    f"book row names a reference the skill spans do not place: {named}",
                )
            )
        elif dropped:
            detail = "trace class absent from footer: " + ", ".join(f"{s}/{t}" for s, t in dropped)
            verdicts.append((ordinal, "fail, class dropped", detail))
        elif unmatched:
            named = ", ".join(f"{s or '?'}/{t}" for s, t in sorted(unmatched))
            verdicts.append((ordinal, "fail, class claimed", f"footer claims, trace has not: {named}"))
        elif counts:
            detail = ", ".join(
                f"{s}/{t} footer {claim} trace {real}" for (s, t), (claim, real) in sorted(counts.items())
            )
            verdicts.append((ordinal, "fail, count only", detail))
        else:
            verdicts.append(
                (ordinal, "pass", f"{len(recorded)} classes, {windowed} runs, exact")
            )
        window_free_by_ordinal[ordinal] = window_free

    width = max(len(state) for _, state, _ in verdicts)
    print(f"cells                {len(verdicts)}")
    for state in (
        "pass",
        "fail, class dropped",
        "fail, class claimed",
        "fail, count only",
        "unresolved, no skill evidence",
        "unreachable",
    ):
        hits = [v for v in verdicts if v[1] == state]
        if hits:
            print(f"  {state:<{width}}  {len(hits):>2}   ordinals {[v[0] for v in hits]}")
    print()
    for ordinal, state, detail in verdicts:
        print(f"  ord {ordinal:>2}  {state:<{width}}  {detail}")
    print()

    print("the parse step, which the merged version could not see")
    print(f"  rows parsed from the reply      {parsed_rows}")
    print(f"  rows in the corpus field        {field_rows}")
    if field_drift:
        print("  cells where the two disagree, and the field is the one that is wrong:")
        for ordinal, was, now in field_drift:
            print(f"    ord {ordinal:>2}  field {was} runs, reply {now} runs")
    else:
        print("  no cell disagrees: the corpus field carries every row the reply shows")
    print()

    near = [v for v in verdicts if v[1] == "fail, count only"]
    print("near-misses, recorded separately per the settled rule")
    if near:
        for ordinal, _, detail in near:
            print(f"  ord {ordinal:>2}  every class named, counts wrong: {detail}")
    else:
        print("  none: no cell failed on a count alone")
    print()

    fails = [v for v in verdicts if v[1].startswith("fail")]
    resting = [v[0] for v in fails if not window_free_by_ordinal.get(v[0], False)]
    print("does the borrowed window carry any of this")
    free = sum(1 for value in window_free_by_ordinal.values() if value)
    print(f"  {free} of {len(window_free_by_ordinal)} cells saw the window exclude nothing at all")
    if resting:
        print(f"  fails that DO rest on the window: ordinals {resting}")
    else:
        print("  no failing cell rests on it: every fail stands on the unwindowed trace too")

    print()
    print("the aggregate, over the cells that were actually compared")
    print(f"  footer runs across those cells {compared_claimed}")
    print(f"  in-window trace calls          {compared_recorded}")
    delta = compared_claimed - compared_recorded
    if delta:
        print(f"  They disagree by {delta:+d}, so a board reported only in total would have")
        print("  seen something here. Read that against CORRECTION.md, which records the")
        print("  merged run claiming the opposite off a parse that lost two runs.")
    else:
        print("  They agree, which on its own says nothing about the cells under them.")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
