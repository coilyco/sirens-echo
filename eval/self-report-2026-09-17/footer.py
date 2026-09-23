"""The disclosure footer, parsed from the reply text it was rendered into.

One parser, read by the corpus extractor and by the measured pass, because the
defect this module exists to close was a parse the comparison could not see. The
corpus carried a `disclosed` field, `fidelity.py` read that field and never
`reply`, and a row the extractor failed to recognise was therefore absent from
every control downstream of it. See CORRECTION.md and REBUILD.md.

**Shape, not glyph.** `toolDisclosureLine` in `internal/community/tooldisclosure.go`
at `coilyco-gaming/sirens-echo` `28d9b4a` renders every row the same way: a quote
marker, a kind glyph, an outcome glyph, a backticked label, an optional run
multiplier, an optional empty note. Matching that shape and then reading the
glyphs means a row the vocabulary does not know raises rather than vanishes.

**A book row is not a tool name.** The renderer swaps the hammer for a book and
the label for `noticeBody(call.Detail)` whenever `Detail` is set, so the label is
the reference a skill read delivered. `Detail` is set in two places rather than
one: `skilltool.go` sets it to a skill's display name and `trackeradapter.go`
sets it to a tracker record key. So the glyph says the call carried a display
value, and it does not say which tool ran. The trace is what knows that: the
same `Detail` reaches the tool span as `mcp.tool.skill`.
"""

from __future__ import annotations

import re
from dataclasses import dataclass

# Mirrors the renderer's constants rather than restating their meaning. The
# outcome set is closed by `ToolOutcome`, which is why an unknown one raises.
TOOL_GLYPH = "\U0001f528"
SKILL_READ_GLYPH = "\U0001f4d6"
OUTCOME_GLYPHS = {"✅": "ok", "❌": "failed", "\U0001f4ed": "empty"}
KIND_GLYPHS = {TOOL_GLYPH: "tool", SKILL_READ_GLYPH: "skill-read"}

EMPTY_NOTE = " — no results"

# Two glyph tokens before a backticked label is the footer row's signature. A
# service notice is `> ` plus one backticked phrase and matches nothing here,
# which is how the five declined turns stay out of the disclosure population.
ROW = re.compile(
    r"^> (?P<kind>\S+) (?P<outcome>\S+) `(?P<label>[^`]*)`"
    r"(?: ×(?P<runs>\d+))?(?P<empty>" + re.escape(EMPTY_NOTE) + r")?$"
)


class UnknownFooterGlyph(ValueError):
    """A footer-shaped row carrying a glyph this vocabulary does not know.

    Raised rather than skipped. The silent skip is the original defect: the
    extractor recognised the hammer, met a book, and wrote a shorter list.
    """


@dataclass(frozen=True)
class FooterRow:
    kind: str
    outcome: str
    label: str
    runs: int
    empty: bool

    @property
    def ok(self) -> bool:
        """The corpus field's older boolean, kept so its shape does not change."""
        return self.outcome == "ok"


def parse_footer(reply: str) -> list[FooterRow]:
    rows: list[FooterRow] = []
    for line in reply.splitlines():
        match = ROW.match(line)
        if match is None:
            continue
        kind = KIND_GLYPHS.get(match["kind"])
        outcome = OUTCOME_GLYPHS.get(match["outcome"])
        if kind is None or outcome is None:
            raise UnknownFooterGlyph(
                f"footer row with unrecognised glyphs: {line!r}. Kind {match['kind']!r} "
                f"and outcome {match['outcome']!r}, against kinds {sorted(KIND_GLYPHS)} "
                f"and outcomes {sorted(OUTCOME_GLYPHS)}. Recheck the renderer at "
                f"internal/community/tooldisclosure.go before widening this."
            )
        rows.append(
            FooterRow(
                kind=kind,
                outcome=outcome,
                label=match["label"],
                runs=int(match["runs"] or 1),
                empty=bool(match["empty"]),
            )
        )
    return rows


def as_disclosed(rows: list[FooterRow]) -> list[dict]:
    """The corpus `disclosed` field: one entry per row, carrying its kind.

    A tool row keeps the `tool` key it already had. A skill-read row carries
    `detail` instead, because it has no tool name to put there and inventing one
    is the mistake that made ordinal 45 read as a dropped class.
    """
    entries: list[dict] = []
    for row in rows:
        entry: dict = {"kind": row.kind}
        if row.kind == "tool":
            entry["tool"] = row.label
        else:
            entry["detail"] = row.label
        entry["ok"] = row.ok
        entry["outcome"] = row.outcome
        entry["runs"] = row.runs
        entries.append(entry)
    return entries


# `noticeAllowed` and the trim cutset from `internal/community/notice.go`, which
# is the alphabet a book row's label was printed through. Underscore is in the
# alphabet and deliberately not in the cutset.
NOTICE_DISALLOWED = re.compile(r"[^a-z0-9 ,./_-]+")
NOTICE_CUTSET = " ,./-"
NOTICE_FALLBACK = "unspecified harness error"


def normalise_detail(detail: str) -> str:
    """`noticeBody` as the footer applied it, so a span attribute can be compared.

    The trace carries `mcp.tool.skill` raw and the footer carries it sanitized, so
    the two are the same string only after this. Observed: `SKILL` on the span
    against `skill` in the footer, which is the whole of ordinal 45's match.
    """
    cleaned = NOTICE_DISALLOWED.sub(" ", detail.lower())
    cleaned = " ".join(cleaned.split()).strip(NOTICE_CUTSET)
    return cleaned or NOTICE_FALLBACK
