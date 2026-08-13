# Rate dataset provenance

A rate is comparable to another rate only if the dataset can say what
produced it. These fields decide that, and each is covered by test rather
than by whoever last edited the struct.

| Field | What it says |
| --- | --- |
| `runner` | The checkout that produced the prompt, definition, skillpack and checks. This is what actually produced the numbers. |
| `roster` | The MCP roster, or `empty`. |
| `fixture` | The tool fixture, or `none`. Exclusive with the roster. |
| `substrate` | Host state at run time, free text, set through `SIRENS_ECHO_SUBSTRATE`. |
| `image` | A deployed image, only when one participates. |

**`image` stays `unrecorded` for every run of this instrument**, and that is
correct rather than a gap. `cmd/sirens-echo-eval` assembles the prompt locally
and posts to the proxy, so no pod takes part in a run. Read `runner` instead. A
number attributed to an image that had no part in producing it is worse than a
number with no image at all.

**`roster: empty` on its own does not mean no tools.** A fixture run reports an
empty roster and still serves tools, which is the whole point of the fixture. The
pair to read is `roster` and `fixture` together. Getting this wrong turns a
meaningful result into a vacuous one: the data-borne injection cases only mean
something if the payload actually arrived in a tool result.

`runner` is filled automatically by `scripts/ward-command.sh` from the current
checkout, so it is present without anyone remembering it. `SIRENS_ECHO_RUNNER`
overrides it, and a build carrying a `-X` revision stamp takes precedence over
both. Provenance that depends on a human remembering is provenance that goes
missing, which is how the first datasets were filed with the SHA hand-written
into `substrate`.


See [the rate instrument](sirens-echo-rate.md) for how a run is scored.
