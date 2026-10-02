---
doc_goal: State how a caller reaches the agent beyond Discord, the transport contract a chat platform implements and what roster re-export offers over /mcp.
---
# Transports and re-export

A transport is one chat platform the agent listens on. Discord is the first. The turn path never sees a
platform: a transport admits a message, builds a `turnIO` for it, and hands that to `runSerialized`, so
every platform answers through the same reply checks, content gate, tool loop and phrases. The private
HTTP ingress is the same shape and is not a transport, because every deployment serves it.

## The contract

`Transport` is `Name()` and `Start(ctx)`. `Start` connects the platform and returns a stop that runs at
shutdown, after in-flight turns have drained, since a turn still answering needs the connection. A
transport that fails to start stops the ones already running. `Agent.Run` starts them through
`startTransports`, and a deployment enables one by building its session.

Per turn, a transport implements `turnIO` (`RequestID`, `Requester`, `Transport`, `Current`, `History`,
`Reply`) and whichever optional capabilities its platform can honour. **A capability a platform lacks is
left out and never stubbed**, because the turn path reads absence as "this transport cannot" and degrades
to words.

* `reactor` and `unreactor` place and remove a mark.
* `typingNotifier` holds a typing indicator for the turn.
* `replyBudget` and `overflowCarrier` bound a reply and attach the whole of one that was cut.
* `attachmentBearer` carries uploads to the tool layer.
* `prefillReporter` says what a whole-thread read dropped.
* `spanTagger` adds the platform's ids to the turn span.
* `interruptible` names the summon after the process dies.
* `progressSinkProvider` supplies the line a long turn narrates through.

`transport.go` holds compile-time assertions, so a turn that stops satisfying one fails the build.

## Discord

`discordTransport` is the gateway start that `Agent.Run` used to carry inline. The admission gates, the
session and the turn stay in the Discord files and are unchanged. See [admission](sirens-echo-admission.md).

## Adding one

Admit through `limiter.Admit` so the cooldown and the pending bound apply, drop a redelivery with
`seenMessages`, enter `drain` for the life of the turn, and keep no platform history: a transport reads
what one turn needs and stores none of it.

## Roster re-export

`SIRENS_ECHO_MCP_REEXPORT` offers the lane's own rostered tools over `/mcp` beside `turn`, so a fleet
client makes one tool call instead of paying a whole agent turn for it. Off by default. Argued at
sirens-echo#1025.

### What it changes, stated plainly

**A re-exported call is not a turn.** The lane's guards live in the turn pipeline rather than in the
tools: `runReplyChecks`, response validation, and the `IdentifierGuard` that #310 depends on. A caller
reaching `sample__find` reaches the server, and none of those run.

So this **moves a security boundary rather than adding an interface**, which is the reason for every
choice below.

### The gate

Every re-exported call needs the deployment token in an `Authorization: Bearer` header, compared in
constant time. `turn` is unchanged and still needs none.

**An empty `SIRENS_ECHO_HTTP_TOKEN` trusts nobody**, so turning re-export on without configuring a
token offers tools that refuse every caller. That is deliberate: a half-configured deployment should
fail closed, especially on a NodePort that by design refuses nobody at the network layer.

This is the existing token actually enforced on a new path rather than the real authentication #1025
says probably wants landing first. If that is judged insufficient, the flag staying off is the safe
state.

### Naming

Tools keep the `server__tool` name `proxyToolName` already gives them, so the collision rule guarding
the model's own tool list guards this surface too. Twelve servers offering `find` stay twelve
addressable tools rather than one nobody chose.

### Staleness, and why a failed refresh keeps the old list

The roster is discovered per turn, and #943 recorded it collapsing from 86 tools to 0 and back. A list
cached forever would advertise tools that stopped existing, so the advertised set refreshes on
`SIRENS_ECHO_REEXPORT_REFRESH`, one minute by default.

**A failed refresh keeps the previous list rather than emptying it.** A client whose tool list vanishes
underneath it is the worse failure, and worse here than internally: a lane's view of a server can fail
while other clients use that same server successfully, which reads as a broken tool rather than a
broken path to one.

### Cost

A call opens a roster session, calls, and closes it. That is honest rather than efficient, and it is
the first thing to improve if this carries real traffic. Admission runs through the same `transportMCP`
budget as a turn, so a tool client cannot outspend the guilds it shares a deployment with.

### Deliberately not here

* **No scoping to part of the roster.** Opting in offers all of it. A deployment wanting less trims the
  roster rather than the export.
* **No per-tool authority.** One token answers for the whole surface, so write tools and read tools are
  gated identically.
* Deploy's README still says every MCP is ClusterIP-only with no lane reaching across a namespace. That
  stays true of the servers. This changes what one lane's own listener offers, and that README wants a
  matching edit if any lane turns this on.

## Related

* [HTTP and MCP surfaces](sirens-echo-http.md) - the listener the private ingress and re-export hang off.
* [Tools](sirens-echo-tools.md) - the roster being re-exported.
* [Admission](sirens-echo-admission.md) - the gates the Discord transport applies.
