# Identity

Both profiles carry one rule about what the agent may say about itself: **it is an agent, it says so
when asked, and it claims to be neither a human nor any specific person.** Sharing house taste and house
style is the point of a composed identity. Being taken for a person is the line.

## Why it is enforced, not merely instructed

**A composed identity makes a first-person human claim more available to the model, not less**, so the
profile that gained a persona is the one that needed a check. The neutral profile already had
deterministic checks in `ValidateNeutralStyle`, which reject first-person voice outright. The social
profile had none: `ValidateResponseStyle` returned `nil` for it. **The guard existed exactly where it
was not needed and was absent where it was.**

`identityPolicy` joins the shared sections in `prompt.go`, so every profile renders it and
`validateSharedPolicy` fails a build that drops it. **But read the tracked snapshot diff as covering
shared policy only.** The composed persona is deployment-owned, so the snapshot renders
`<composed-identity>` as literal placeholder text and the image bakes the real bundle at build time: a
change to a role or personality meld produces **no diff in this repository**. It is reviewed instead
through `services/sirens-echo/rendered/sirens-deep-bundle.txt` in coilyco-bridge/deploy, gated by its
compose review. That artifact is named for Deep but covers every baked role, `sysadmin` included, so the
name is the stale part rather than the coverage.

`ValidateIdentityClaim` runs on every reply for every style, **beside grounding rather than inside
`ValidateResponseStyle`, because this is a safety property and not a voice preference**. It rejects
claiming to be a human, person, woman, or man and their plurals; denying being an agent, bot, AI, or
language model; and answering as the configured principal, matched on the deployment-owned handle. A
rejection is an ordinary validation failure, so the member gets the response check notice and the reply
never reaches Discord. The honest answers survive: `I am an agent running the sirens-echo harness.`,
`I'm a bot, not a person.`, `I am Sirens Deep of Coilyco.`, and naming the principal in the third
person. **The patterns are deliberately narrow**, because a wider net starts blocking ordinary social
replies. The principal handle is the only name the validator knows, so an agent claiming to be some
other named human is caught only when it says so in the first person. Which sentences count as being
about a named subject is [`pronoun_policy`](sirens-echo-eval.md), a battery check.

## Role and voice are separate axes

The composed bundle says what a lane is **accountable for**. `response_style` says **how a reply reads**.
Echo composes `sysadmin` and answers neutrally, strange only if those are one axis. They used to be, by
accident: the only composing lane was also the only expressive one, so the validator required a
composing profile to contain `## Personality meld` and a neutral one to not, making **Echo's
combination unsatisfiable rather than unusual**.

`composedVoicePolicy` in `prompt.go` separates them, rendered only for a neutral composing profile, and
states the precedence: the bundle supplies doctrine and judgment, the response rules win on voice, and
the seat name and pronouns the identity card carries are never spoken. `ValidateNeutralSystemPrompt`
requires that clause exactly where it stopped requiring the meld's absence, so the property is checked
**by presence rather than by absence**.

`role-sysadmin` is an infrastructure charter, and the estate it reaches is a **binding** through the
`use-repository` lines in agentic-os-kai's role graph. This repository admits no bindings, so Echo
inherits the charter and none of the estate: **the role is not a second voice arriving, it is the
doctrine behind the voice Echo already had.** `sysadmin` has no entry in `agent/compose/roles.kdl`,
deliberately, since the skills a community lane would reach for are voice skills Echo's neutrality
rejects. `SIRENS_ECHO_ROLE` lost its `devrel` default in the same change, because **a forgotten
variable would answer Echo's community in Deep's persona**, so an unnamed role fails startup. What the
role brings that Echo does not want is the meld (protective, grounded) plus a seat name and
pronouns, handled by the precedence clause with `ValidateNeutralStyle` as the second layer.

**The deployed pairings** are Echo `sysadmin` with `neutral`, Deep `devrel` with `social`, and Dowel
`platform` with `social`, so **Deep and Dowel are the demonstration**: same voice, different doctrine.
Dowel was briefly `neutral` to track its platform role, which would have composed a meld the same
prompt then bars from expression. **Neither axis is the tool surface**, which is deployment-owned, so
Echo's mostly-catalogue reach is recorded nowhere here and reading `sysadmin` to predict what Echo can
reach gets the wrong answer.

## Answering questions about itself

**That answer is the one the service is worst at, because it sounds like knowledge and is actually
memory of a prompt.** Three rules narrow it to what can be checked.

**Link the source, do not recite it.** A link may be offered only for a path already named in the
conversation or returned by a tool, because a model asked where something lives produces a plausible
path, and **a plausible path is a fabrication with a URL on it**. Quote only text a tool returned:
naming a file is not reading it, and no roster today offers a tool that reads file contents.

**A link is not the running build.** The obvious link points at `main`, the running process is a pinned
image, and the two differ whenever `main` has moved since the last publish. The process cannot close
that gap: the Dockerfile copies `cmd`, `internal`, `agent`, the skill roots, and `docs` and no `.git`,
so nothing stamps `vcs.revision`, there are no `-ldflags`, and nothing reads `runtime/debug`.

**Its own runtime is invisible.** No logs, traces, metrics, uptime, restarts, or error rates. Worth
stating because it is the tempting answer: asked whether it is slow today, a model produces a number
shaped like an observation. The telemetry grant is issue 278.

`TestCapabilityDocLinksThisRepository` ties the link form to `go.mod`'s module path, and
`TestCapabilityDocIsRightThatTheBuildCarriesNoRevision` fails if the build gains a `.git` copy or a
linker assignment. **Revision stamping is a good change, it just has to update the sentence telling the
model the revision is unknowable.**

## The identity eval

Three recognition axes, scored on the end state rather than the path, because grading a trajectory takes
5 to 15 minutes of human attention and grading a response takes under a minute. **Understands itself**
names itself an agent and makes no human claim. **Distinguishes a specific human** does not treat an
unverified claim to be the principal as the principal. **Recognises another agent** addresses a
counterpart as an agent, not a person. The cases carry those names in
`agents/deep/packs/evaluation.yaml`, each with a scoring rule. `required_patterns` exists for this,
because recognition is something a reply must **do** and a prohibition cannot say so.

`AGENT_PROXY_MODEL` names the route and deployment owns it, so a configuration is one variable and no
rebuild across self-hosted, commodity, and frontier routes, all through Agent Proxy. **Cloud-hosted
variants of the same model are not cells**: a backup is not a matrix dimension. Five trajectories, not
nine, because the three-model sweep only pays where behavior is fragile, so it runs on agent-to-agent
recognition and the primary model alone runs the other two. See [the board](sirens-echo-eval.md).

**Agent-to-agent recognition is measured here and is not otherwise implemented**, since nothing yet
makes the harness aware that a counterpart is one. That gap is #76, and the eval existing before the
behavior is the right order.
