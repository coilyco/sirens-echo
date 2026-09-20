# Composing a lane identity

Both lanes compose an Agent Compose role bundle. Deep composes `advocate` and takes its voice from it,
Echo composes `admin-assist` and does not ([role and voice](sirens-echo-identity.md)).
The [community person package](sirens-echo-person.md) adds lane roles the Core Roster lacks.

## The allowlist is a role graph

`agent/compose/roles.kdl` is the one tracked allowlist, in agent-compose's role-graph format, with
globs. **It is purely additive**: it grants a role its skills and does not decide which roles exist. The
roster does that and the build bakes a bundle per roster role, so a role with no entry composes the
roster identity alone. **An entry with no role fails the build.**
`agent/compose/request.kdl` is the second tracked file, and **only
`declaration=` is permitted**: with `root=`, the source repository's own `.agents/roles.kdl` decides,
and agentic-os-kai's creator role deliberately binds Kai's career, job-search, and LinkedIn context,
because that role serves Kai, not an agent answering strangers. The declaration is generated, its
`path="skills/<name>"` derived.

`cmd/sirens-echo-compose` expands the graph and fails when a pattern reaches a name in
`DeniedComposedSkills`, matches nothing, or globalizes a private repository. **An empty selector hides
an upstream rename, so it is an error.** Every admitted source lives in the public
`coilyco-flight-deck/agentic-os`, under that repository's `docs/composed-house-taste.md` placement rule,
and **the graph declares no provider repositories**, so sources reach a bundle through the request's
declarations and a globalized provider would bypass that review.
`internal/community/compose_test.go` re-runs those failures plus `root=` usage and tracked build output,
with the deny list in `composepolicy.go` beside the expander, **so build and suite enforce one list.**

`scripts/stage-compose-sources.sh` reads the roster for the role list, runs the generator per role, and
stages each admitted `COMPOSED.md` as `SKILL.md` beside a written declaration, because a declaration's
paths resolve beneath its own directory. That staged tree is build output, removed on exit. The image
clones the catalogue at `AOS_CATALOG_REF`, **which floats on `main` by design**, so reproducing an older
bundle means overriding the build arg. At runtime `composed: true` makes a bundle mandatory and
`SIRENS_ECHO_ROLE` selects which one loads, so flipping the role needs no rebuild, while **a missing or
unreadable bundle stops the process**: a profile that asks for an identity never answers without one.

## Reviewing a wider compile

The image bakes a bundle from the public catalogue it can reach. A layer holding catalogues this build
cannot read composes the same allowlist against all of them, so the wider result is reviewable alongside
the shipped one, and **neither compile replaces the other**. It runs from the image itself with
catalogues mounted read-only, so **it is this repository's expander over this repository's graph**:
every rule above still applies and the other layer contributes checkouts and delivery, never policy.
Two rules only the wider set makes visible: **a name owned by two catalogues is an error rather than a
silent first-wins**, and a pattern naming a denied source **exactly** is fatal while a family glob that
merely brushes one drops that member and prints it, without which `personal-preference-*` would become
unusable the moment a catalogue holding the denied `personal-preference-social` is present.

## Every role must compose

CI bakes the bundles in the `test` job and the image build checks the same thing again over the bundles
it ships: **the second stops a bad image, the first tells you in seconds rather than two thirds of the
way through a build** (sirens-echo#788). Loading a bundle renders and validates that role's prompt,
because a bundle that failed to compose would ship as a quietly neutral agent and one filed under the
wrong slug would make `SIRENS_ECHO_ROLE` select the wrong identity. **Prompt sizes are printed per role
and never gated.**

`just role-check` bakes and checks in one step, the gate CI runs, and it bakes to a scratch directory it
removes, **because pre-commit walks the filesystem and a baked bundle is a tree of skill files those
hooks then read as this repository's own**. `just compose-bundles` and `role-load` work against
`agent/bundles` and need `AOS_CATALOG`.

**This repository tracks no record of what each role selected.** It used to, as
`agent/rendered/roles/<role>.bundle.txt`, and the record was removed because it froze a selection that
two upstream sources move on their own: the catalogue ref floats on `main`, and CI runs inside the
floating `agentic-os:release` tag. A republish of either changed what the bake produced and reddened
`main` with no commit in this repository to explain it, which is a gate nobody here could act on
(`teable:coilyco-flight-deck/agentic-os#7734`). What survives is the check that matters at runtime: the
bundle composed, the prompt renders, the slug is right. `roles.kdl` still says what a role **may** have,
and the bundle in the shipped image is what it **got**.

## Who the agents belong to

Sirens Discord is the community. coilyco Gaming is a separate organization holding a staffing and
product contract with it, the Robotics Division is the part of coilyco Gaming that contract is with, and
Echo and Deep are that division. It lives in `.agents/skills/coilyco-org`, **the only local skill root
the two profiles share**, which makes "both agents read the same text" a property a test can hold, and
**two agents improvising the same relationship separately is the drift that prevents**. The prompt
carries one line naming the division.

**The separation is the load-bearing part.** An agent is never Sirens Discord staff, and Sirens Discord
staff are never coilyco Gaming employees: either half alone is a different and wrong claim. The contract
exists and its terms are not something to disclose, and nothing in the source names a person, account,
or internal system.

## Recognising a counterpart agent

Deep can tell whether the account it is answering is an agent or a person. **Discord marks bot accounts,
and that flag is ground truth costing nothing**, whereas guessing from writing style would be a worse
version of the problem, because anyone can write "I am an agent" and a member who does must not become
one. So recognition reads `Author.Bot` and nothing else, asserted in tests rather than assumed.

Bot authors were rejected outright, so recognition required admitting them, which widens the summon
surface, so it is **opt-in by name**: `agents.allow` lists the counterpart accounts answered, empty
answers none, which is the shipped posture, and Deep never answers itself. **A named counterpart is
admitted, not trusted**, passing the same channel and guild gates a member does. Two agents each
answering the other is a runaway, so consecutive agent-authored turns in one channel are capped, a human
turn resets the run, **a capped exchange does not resume by waiting so the cap is not a speed limit**, a
quiet channel forgets, and the bound is per channel. The check runs before admission spends anything.

An agent-authored line is marked in the turn context as `- Sirens Echo (an agent, not a person):`, with
a matching sentence naming the requester. **A human turn gains no marking at all**, because an
annotation that is sometimes absent is more useful than one that is sometimes wrong, and the member's
message itself is untouched per the final-user-turn rule. **What deliberately does not change is
behaviour**: no disclosure line, no register change, no altered trust. An admitted counterpart widens
the requester set beyond one account, so `CheckExecutionAdmission` refuses execution while one is
allowed. Both refusals log without an identifier, as `discord.agent.ignored` and
`discord.exchange.bounded`.
