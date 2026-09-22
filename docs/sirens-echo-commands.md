# Structured commands

Free text is the right interface for conversation and the wrong one for consequential actions. "Deploy
the thing" and "deploy the other thing" are one paraphrase apart, and a natural-language turn gives the
harness no structural way to tell them apart before acting. **Commands add a structured path, not a
replacement for the conversational one.**

## The schema is an authority boundary

Two deploy-repository issues failed the same way: the surrounding tooling bounds tool **names** and not
tool **arguments**. Commands are the first place this harness owns an argument schema itself, so the
schema is treated as authority rather than input validation.

* An **undeclared argument is refused**, never ignored, because ignoring one lets a caller believe it
  took effect.
* Every string carries a **length bound**, defaulted rather than optional. A value naming a thing
  carries **choices**, the tightest bound available; one that cannot be a fixed set carries a `pattern`.
* A command **submits exactly one job kind**, and that kind must be declared, so the command surface
  cannot outgrow the job surface.
* The declaration is **validated at startup**, so a malformed command fails before a caller reaches it.

**The same declaration renders Discord's options and binds the incoming arguments**, so the advertised
schema and the enforced one cannot drift.

## A command is a summon path

Discord routes an interaction to the bot because the user picked it, not because they mentioned anyone,
**which bypasses the mention gate by construction** (#127). So a command passes the gates a message
passes, in the same order: access policy first, then admission. **A command reaches nothing a message
from the same caller in the same place could not.** Registration is a **write** to Discord's API needing
the `applications.commands` scope, so it is off unless `SIRENS_ECHO_DISCORD_COMMANDS` says otherwise.
Every path answers with a notice, so a command never ends in silence.

## Thread binding

Once work is durable, a follow-up like "cancel that" needs a referent, and the thread is the natural
one. The binding lives **on the job record** as `Origin.ThreadID`, so resolving is a lookup of a
recorded fact rather than an inference from recent history, **and the record is the single source of
truth rather than a second index that can drift**. It is **singular in both directions**: a thread
cannot be bound to a second job and a job cannot be repointed at another thread, while rebinding the
same pair is idempotent so a retry is safe. `job-status` and `job-cancel` take an optional job id and
fall back to the binding, an explicit id always wins, and outside a bound thread with no id the command
says it has no referent rather than guessing. **A job binds to the thread it was started in, and only to
a thread**, because binding a channel would make it resolve to one arbitrary job of many.

A command carries **no per-requester authority**: every job runs under pod-level authority regardless of
who asked (#145). The principal is recorded, and **recording is not granting**.

## Publishing the commands

A slash command has three parts: a declaration, a handler, and a registration with Discord. This
repository had the first two and not the third, so **every command it ships was unreachable even with
the deployment switch on**. **Registration goes per guild, to each guild the access policy admits**,
because a global registration appears in **every** guild the bot is in, including ones the policy
refuses, where the commands would be visible, invocable, and answered with `not permitted here`,
**advertising a summon path this deployment does not offer**. A deployment admitting no guild registers
nowhere and says so rather than falling back to global.

The write is a **bulk overwrite per guild**, so the published set is exactly the declared set and a
command removed from `JobCommands()` disappears instead of lingering as an invocable ghost. **Failure is
never fatal**: a guild that refuses the write is logged as `discord.commands.failed` and the loop
continues, with the first error returned at the end. It runs on `discord.ready`, and **an install
predating the `applications.commands` scope correction has to be re-authorised** first.

## A server prompt as a slash command

An MCP server publishes prompts and Discord publishes slash commands. The mapping is mechanical and the
constraints are not, because **a prompt name is server-supplied and satisfies none of Discord's shape by
construction**. A malformed command fails the **whole registration**, so one server's prompt would cost
every other command, which makes refusing a single prompt the cheaper failure. A name that cleans to
nothing is refused, so is a missing description, so are more arguments than Discord allows. **Truncation
is the one repair for an over-long description**: still true, only shorter. An empty one is refused
rather than filled from the name, which would tell a member nothing. The name mapping lower-cases,
replaces separators with hyphens, drops anything Discord refuses, and trims.

**The part that is not mechanical**: a prompt is user-selected instruction reaching the model through a
structured channel, the same class as an uploaded file, data the turn may read and never instructions it
obeys. No filter separates a prompt that describes an instruction from one that issues it, so **the
bound is posture rather than detection**.

`CommandFromPrompt` has no production caller, by design: it is the mapping half of sirens-echo#127,
open with Kai's approval recorded. Not blocked by access policy, since the reference policy names all
six summon paths and `onInteraction` already gates an interaction like a mention. What is left:
registration is a live API call whose failure mode is a malformed set in a real guild, **the
promotable-prompt allowlist does not exist yet** (a prompt is not a command until this repository says
so, its schema declared here rather than taken from the publishing server), and
`SIRENS_ECHO_DISCORD_COMMANDS` defaults false. Underneath: **an interaction must be answered in three
seconds and a model turn takes minutes**, so a prompt command needs a deferred response or the job path
(sirens-echo#884).

## /poll

A member-invoked, free-form native poll: the member initiates, not Echo, keeping it inside the
no-announcement exclusion. Its duration and rune caps are Discord's own limits, exposed as knobs.

## /mcps and /mcp

`/mcps` names the MCP servers this deployment reaches and their tools, taking no arguments and no job.
`/mcp <server>` describes one, adding **what each tool does**: an index and a detail, not a long and a
short form.

**Both read what the process reached, not what the file declares.** The roster names servers, whether
one answered is a different fact, and **the two come apart exactly when someone needs this**. So three
states stay distinct, collapsing them hiding the interesting one: `name (n): tools`, `name: no tools`
(answered, advertising nothing), and `name: did not answer this turn`.

**Neither carries addresses**: a name is a fact about the deployment, an address is an identifier, and
a test asserts the absence. **Both are ephemeral**, since introspection belongs to whoever asked and
Echo's channel carries members who did not, and the declaration carries that flag rather than the
handler, so **a denied one is not the public event it was avoiding**. `runCommand` answers both above
the job guard, and each reply is cut to one interaction and says so, since **a silently short list reads
as a short one**.

**`/mcp`'s argument carries choices**, a server name naming a thing and the roster being fixed at image
build. It is read at registration and **again at dispatch**, so a choice outliving its roster is
refused, and no MCP publishes no `/mcp`. **Neither threads**: a thread hangs off a channel message, and
an ephemeral reply is not one.
