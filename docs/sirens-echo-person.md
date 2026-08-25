# The community person package

`agent/compose/person` is an external agent-compose person package holding the roles the community owns
and the Core Roster does not. **Selection is exclusive per request**: a request naming a person-source
composes only that package's roster, so the stage script bakes the Core Roster roles from the embedded
default and each package role from a request that gains one `person-source "person"` node. The package
sits under `agent/compose` because a request's person-source path must be relative and clean, so it
cannot reach a sibling directory. A slug owned by both rosters would silently overwrite a core bundle,
so the script fails on the collision instead.

The package uses agent-compose's sectioned person layout (`roles/`, `personalities/`, `definitions/`),
because the flat `data/` layout documented upstream is wired for the embedded roster alone. The
personalities restate the core bodies verbatim, since agent-compose never merges entities across
packages. The bake compares each restated body against the same
skill as delivered from the embedded roster and fails on a byte difference, so the copies are checked
mirrors rather than silent forks. A package-native personality with no core counterpart is skipped
with a note, which is the trade: drift is caught only while some core role still delivers the body.

`librarian` is the first package role: custodian of the community record, composed for a future fourth
lane. Lookup-first, provenance-first retrieval is the charter: every interaction
opens with a retrieval against whatever sources the runtime grants, answers read from a named source with the citation
attached, gaps filed rather than papered over, and the record presented beside a conflicting memory
without adjudicating it. It carries no boundary or adjacency entities, because their owners and targets
are not defined roles in a single-role package, and the charter body carries the hand-offs instead. No
lane composes it yet, and `SIRENS_ECHO_ROLE=librarian` selects its baked bundle once one does.

`engineer`, `ops` and `creator` joined it once the Core Roster reflowed to seven bonded seats and
dropped all three. Every deployed lane names one of them in `SIRENS_ECHO_ROLE`, so the alternative was
remapping running Discord agents onto seats chosen for a different purpose. Owning the slugs here
instead leaves the deployment untouched and puts upstream roster churn out of reach of these lanes. The
definitions are lifted verbatim from the roster that had them rather than reauthored, so the bundles
carry what the lanes already carried: `ops` recomposes byte-identical apart from its source label. The
trade is the one the package always makes, and it is now the larger half of the bundle: these lanes no
longer inherit Core Roster doctrine updates, and somebody has to carry a change across by hand.

Adjacency is dropped for the same reason `librarian` has none - `qa`, `ai`, `director`, `design` and
`exec` are not roles here - so those hand-offs live in the charter bodies. Boundaries are kept, because
dropping one silently loosens a lane rather than leaving a gap in prose. Three own themselves
(`build-software` to `engineer`, `modify-live-system` to `ops`, `suggest-human-comms` to `creator`).
`seek-external-validation` was `exec`'s, and an owner outside the package fails validation, so it moved
to `creator`: `ops` keeps deferring it, which is what protects Echo, and Deep already carries the
outward-reaching tool surface that makes the reach its own.

## See also

* [Composing a lane identity](sirens-echo-compose.md) - the allowlist, the request, and the bake.
* [Identity](sirens-echo-identity.md) - role and voice as separate axes.
