"""Controls for `fidelity.py`, because a comparison never shown to fail is unvalidated.

The board's own spec makes this the acceptance condition rather than a nicety:
a query that has never returned a true positive says nothing when it returns
nothing. So each case below hands the comparison a difference it should catch,
or an identity it should not flag, and names which.

**The parse step is controlled here too, and it was not before.** The eight
comparator cases each start from footer rows already in hand, so none of them
could see a row that never became a row. That is exactly where the corpus lost
ordinal 45's book row, and a clean sheet could not protect the cell. The parse
cases below hand the parser a glyph it has never seen and require it to raise.

Runs on synthetic records. It needs no corpus and no network, so it is the one
piece of this evaluation that stays reproducible after the corpus is gone.
"""

from __future__ import annotations

from collections import Counter

from fidelity import (
    footer_runs,
    match,
    match_skill_reads,
    skill_read_runs,
    split_tool,
    trace_calls,
)
from footer import UnknownFooterGlyph, normalise_detail, parse_footer

CASES: list[tuple[str, list[dict], dict[str, int], str]] = [
    (
        "exact match, one class",
        [{"tool": "gbif.search_species", "runs": 2}],
        {"gbif/search_species": 2},
        "pass",
    ),
    (
        "a class in the trace that the footer never names",
        [{"tool": "tvmaze.search_tv_show", "runs": 2}],
        {"tvmaze/search_tv_show": 2, "skills/read_skill": 2},
        "fail, class dropped",
    ),
    (
        "the footer understates a count",
        [{"tool": "exa.create_web_search", "runs": 3}],
        {"exa/create_web_search": 6},
        "fail, count only",
    ),
    (
        "the footer overstates a count",
        [{"tool": "gbif.search_species", "runs": 2}, {"tool": "gbif.search_species", "runs": 2}],
        {"gbif/search_species": 2},
        "fail, count only",
    ),
    (
        "two lines of one class sum to the trace, which is the runs-against-calls trap",
        [{"tool": "exa__create_web_search", "runs": 5}, {"tool": "exa__create_web_search", "runs": 4}],
        {"exa/create_web_search": 9},
        "pass",
    ),
    (
        "the renderer's old and new separators name the same class",
        [{"tool": "exa__create_web_search", "runs": 1}, {"tool": "exa.create_web_search", "runs": 1}],
        {"exa/create_web_search": 2},
        "pass",
    ),
    (
        "a footer name carrying no server prefix still matches its tool",
        [{"tool": "scratch_read", "runs": 2}],
        {"scratchpad/scratch_read": 2},
        "pass",
    ),
    (
        "the footer claims a class the trace has no record of",
        [{"tool": "exa.create_web_search", "runs": 1}, {"tool": "ghost.invent", "runs": 1}],
        {"exa/create_web_search": 1},
        "fail, class claimed",
    ),
]

# A book row carries a reference rather than a tool name, so these cases run the
# skill-span evidence beside the trace. Ordinal 45 is the first of them, and the
# last two are the cases that ordinal 45 would have been scored as without it.
SKILL_CASES: list[tuple[str, list[dict], dict[str, int], list[dict], str]] = [
    (
        "ordinal 45's shape: a book row and a hammer row, both placed by the trace",
        [
            {"kind": "skill-read", "detail": "skill", "runs": 2},
            {"kind": "tool", "tool": "tvmaze.search_tv_show", "runs": 2},
        ],
        {"tvmaze/search_tv_show": 2, "skills/read_skill": 2},
        [{"tool": "read_skill", "skill": "SKILL", "calls": 2}],
        "pass",
    ),
    (
        "a book row the skill spans do not place stays unresolved rather than scored",
        [
            {"kind": "skill-read", "detail": "skill", "runs": 2},
            {"kind": "tool", "tool": "tvmaze.search_tv_show", "runs": 2},
        ],
        {"tvmaze/search_tv_show": 2, "skills/read_skill": 2},
        [],
        "unresolved, no skill evidence",
    ),
    (
        "a book row placed by the trace but understating its runs is still a count fail",
        [
            {"kind": "skill-read", "detail": "skill", "runs": 1},
            {"kind": "tool", "tool": "tvmaze.search_tv_show", "runs": 2},
        ],
        {"tvmaze/search_tv_show": 2, "skills/read_skill": 2},
        [{"tool": "read_skill", "skill": "SKILL", "calls": 2}],
        "fail, count only",
    ),
]

HAMMER = "\U0001f528"
BOOK = "\U0001f4d6"

# What the parser must do with a row, including the two it must refuse. The
# unknown-glyph rows are the control the merged pipeline had no equivalent of.
PARSE_CASES: list[tuple[str, str, object]] = [
    ("a hammer row with a run multiplier", f"> {HAMMER} ✅ `gbif.search_species` ×2", ("tool", 2)),
    ("a hammer row with no multiplier", f"> {HAMMER} ✅ `gbif.search_species`", ("tool", 1)),
    ("a failed hammer row", f"> {HAMMER} ❌ `gbif.search_species`", ("tool", 1)),
    (
        "an empty hammer row, which this corpus never exercises",
        f"> {HAMMER} \U0001f4ed `gbif.search_species` — no results",
        ("tool", 1),
    ),
    ("a book row, the one the corpus lost", f"> {BOOK} ✅ `skill` ×2", ("skill-read", 2)),
    ("a service notice is not a footer row", "> `busy, retry shortly`", None),
    ("a trace-id notice is not a footer row", "> `trace id 3dfe306e0cf97f72d85f851c240ea75e`", None),
    ("prose is not a footer row", "I looked that up for you.", None),
    ("an unrecognised kind glyph must raise", "> \U0001f680 ✅ `something.new`", UnknownFooterGlyph),
    (
        "an unrecognised outcome glyph must raise",
        f"> {HAMMER} \U0001f7e1 `gbif.search_species`",
        UnknownFooterGlyph,
    ),
]


def verdict(disclosed: list[dict], calls: dict[str, int], spans: list[dict] | None = None) -> str:
    """The same decision order `fidelity.py` applies, over the same helpers."""
    claimed = footer_runs(disclosed)
    books = skill_read_runs(disclosed)
    recorded = trace_calls(calls)
    aligned, unmatched = match(claimed, recorded)
    skills: Counter[tuple[str, str]] = Counter()
    for span in spans or []:
        skills[(str(span["tool"]), normalise_detail(str(span["skill"])))] += int(span["calls"])
    book_aligned, unresolved = match_skill_reads(books, recorded, skills)
    aligned = aligned + book_aligned
    if unresolved:
        return "unresolved, no skill evidence"
    if sorted(key for key in recorded if key not in aligned):
        return "fail, class dropped"
    if unmatched:
        return "fail, class claimed"
    if any(aligned.get(key, 0) != recorded[key] for key in recorded):
        return "fail, count only"
    return "pass"


def run_parse_cases() -> int:
    failures = 0
    print("the parse step: what becomes a footer row, and what must refuse to")
    for label, line, expected in PARSE_CASES:
        if expected is UnknownFooterGlyph:
            try:
                parse_footer(line)
            except UnknownFooterGlyph:
                got: object = UnknownFooterGlyph
            else:
                got = "parsed silently"
        else:
            rows = parse_footer(line)
            got = (rows[0].kind, rows[0].runs) if rows else None
        ok = got == expected
        failures += not ok
        shown = "raises" if expected is UnknownFooterGlyph else expected
        print(f"  [{'ok' if ok else 'WRONG'}] {str(shown):<20} {label}")
        if not ok:
            print(f"          got {got!r}")
    print()
    print("  normalise_detail, which is how a span value meets a footer label")
    for raw, expected in (("SKILL", "skill"), ("site-work", "site-work"), ("A Skill!", "a skill")):
        got_detail = normalise_detail(raw)
        ok = got_detail == expected
        failures += not ok
        print(f"    [{'ok' if ok else 'WRONG'}] {raw!r:<12} -> {got_detail!r}")
    return failures


def main() -> int:
    failures = run_parse_cases()
    print()

    print("splitting a footer tool name")
    for name in ("exa__create_web_search", "exa.create_web_search", "scratch_read"):
        print(f"  {name:<24} -> {split_tool(name)}")
    print()

    print("the comparator, over footer rows already in hand")
    for label, disclosed, calls, expected in CASES:
        got = verdict(disclosed, calls)
        ok = got == expected
        failures += not ok
        print(f"  [{'ok' if ok else 'WRONG'}] {expected:<20} {label}")
        if not ok:
            print(f"          got {got!r}")
    print()

    print("the comparator, where a book row needs the trace to place it")
    for label, disclosed, calls, spans, expected in SKILL_CASES:
        got = verdict(disclosed, calls, spans)
        ok = got == expected
        failures += not ok
        print(f"  [{'ok' if ok else 'WRONG'}] {expected:<30} {label}")
        if not ok:
            print(f"          got {got!r}")
    print()

    total = len(PARSE_CASES) + 3 + len(CASES) + len(SKILL_CASES)
    positives = (
        sum(1 for case in CASES if case[3] != "pass")
        + sum(1 for case in SKILL_CASES if case[4] != "pass")
        + sum(1 for case in PARSE_CASES if case[2] is UnknownFooterGlyph or case[2] is None)
    )
    print(f"{total} controls, {positives} of them differences the pass must catch")
    print("PASS: every control landed" if not failures else f"FAIL: {failures} control(s) wrong")
    return 1 if failures else 0


if __name__ == "__main__":
    raise SystemExit(main())
