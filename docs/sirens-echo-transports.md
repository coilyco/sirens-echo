---
doc_goal: State how a caller reaches the agent beyond Discord, the transport contract a chat platform implements and what roster re-export offers over /mcp.
---
# Transports and re-export

A transport is one chat platform the agent listens on, and Discord is the first. The turn path never sees a
platform: a transport admits a message, builds a `turnIO` for it, and hands that to `runSerialized`, so every
platform answers through the same reply checks, content gate, tool loop and phrases. The private HTTP ingress has
the same shape and is not a transport, because every deployment serves it.

## The contract

`Transport` is `Name()` and `Start(ctx)`. `Start` connects the platform and returns a stop that runs at shutdown,
after in-flight turns have drained. A transport that fails to start stops the ones already running
(`startTransports`). `discordTransport` is the gateway start `Agent.Run` used to carry inline: the gates, the
session and the turn stay in the Discord files, see [admission](sirens-echo-admission.md).

Per turn, a transport implements `turnIO` and whichever optional capabilities its platform can honour:
`reactor`, `unreactor`, `typingNotifier`, `replyBudget`, `overflowCarrier`, `attachmentBearer`, `prefillReporter`,
`spanTagger`, `interruptible` and `progressSinkProvider`. **A capability a platform lacks is left out and never
stubbed**, because the turn path reads absence as "this transport cannot" and degrades to words. `transport.go`
holds compile-time assertions for each turn type.

A new transport admits through `limiter.Admit`, drops a redelivery with `seenMessages`, enters `drain` for the
life of the turn, and keeps no platform history.

## Slack

`slackTransport` runs over Socket Mode, so it needs no public endpoint. It admits `app_mention` events in channels and
`message.im` direct messages and nothing else: a bot author, an edit, a subtype and the bot itself are ignored, and a
redelivery is answered once. A reply goes to the thread the summon is in, to a new thread under a channel mention, or
inline in a direct message. Reactions map the harness glyphs to Slack names, and a glyph with no name falls back to
words. Progress is one message edited in place. Text is sent with markup off and `&`, `<`, `>` escaped, so no reply
can form a mention or a link.

**Access is its own file and schema**, `coilyco-harness.slack-access.v1`: workspaces, channels, users, a deny list and
direct messages. Without one, nothing is admitted. A bad bot token, an app token Slack rejects, or a bot workspace the
policy never names stops the process at start. `sirens-echo-slack-access-check` validates a file offline. See the
[reference policy](slack-access-policy.reference.yaml).

**Slack's API terms bar long-term storage of its data.** A turn reads its thread, or the channel before the summon,
and drops it. Display names are held in memory for an hour. No job, attachment, event queue, interrupt record or
follow-up without a mention runs on Slack.

Enable with `SIRENS_ECHO_SLACK_ENABLED`, `SLACK_BOT_TOKEN` (`xoxb-`), `SLACK_APP_TOKEN` (`xapp-`) and
`SIRENS_ECHO_SLACK_ACCESS_POLICY`. A Slack-only deployment sets `SIRENS_ECHO_DISCORD_ENABLED=false`. Both tokens are
guarded, so a reply carrying either is refused. The app is declared in a [manifest](slack-app.reference.yaml).

## Roster re-export

`SIRENS_ECHO_MCP_REEXPORT` offers the lane's own rostered tools over `/mcp` beside `turn`, so a fleet
client makes one tool call instead of paying a whole agent turn for it. Off by default. Argued at
sirens-echo#1025.

**A re-exported call is not a turn.** The lane's guards live in the turn pipeline rather than in the
tools: `runReplyChecks`, response validation, and the `IdentifierGuard` that #310 depends on. A caller
reaching `sample__find` reaches the server, and none of those run.

So this **moves a security boundary rather than adding an interface**, which is the reason for every
choice below.

**The gate.** Every re-exported call needs the deployment token in an `Authorization: Bearer` header, compared in
constant time. `turn` is unchanged and still needs none.

**An empty `SIRENS_ECHO_HTTP_TOKEN` trusts nobody**, so turning re-export on without configuring a
token offers tools that refuse every caller. That is deliberate: a half-configured deployment should
fail closed, especially on a NodePort that by design refuses nobody at the network layer.

This is the existing token actually enforced on a new path rather than the real authentication #1025
says probably wants landing first. If that is judged insufficient, the flag staying off is the safe
state.

**Naming.** Tools keep the `server__tool` name `proxyToolName` already gives them, so the collision rule guarding
the model's own tool list guards this surface too. Twelve servers offering `find` stay twelve
addressable tools rather than one nobody chose.

**Staleness.** The roster is discovered per turn, and #943 recorded it collapsing from 86 tools to 0 and back. A list
cached forever would advertise tools that stopped existing, so the advertised set refreshes on
`SIRENS_ECHO_REEXPORT_REFRESH`, one minute by default.

**A failed refresh keeps the previous list rather than emptying it.** A client whose tool list vanishes
underneath it is the worse failure, and worse here than internally: a lane's view of a server can fail
while other clients use that same server successfully, which reads as a broken tool rather than a
broken path to one.

**Cost.** A call opens a roster session, calls, and closes it. That is honest rather than efficient, and it is
the first thing to improve if this carries real traffic. Admission runs through the same `transportMCP`
budget as a turn, so a tool client cannot outspend the guilds it shares a deployment with.

**Deliberately not here:**

* **No scoping to part of the roster.** Opting in offers all of it. A deployment wanting less trims the
  roster rather than the export.
* **No per-tool authority.** One token answers for the whole surface, so write tools and read tools are
  gated identically.
* Deploy's README still says every MCP is ClusterIP-only with no lane reaching across a namespace. That
  stays true of the servers. This changes what one lane's own listener offers, and that README wants a
  matching edit if any lane turns this on.

## Posting a message

`POST /v1/message` posts `{"channel","content"}` verbatim to a Discord channel as the bot, with no
credential, no model, and no grounding or content gate. **The allowlist and the mention policy are the
whole boundary.** The model cannot call it.

* **Allowlist.** `SIRENS_ECHO_MESSAGE_CHANNEL_<SLUG>` holds a channel id. The caller's `channel` loses one
  leading `#`, is upper-cased, and `-` becomes `_`, so `gaming-arpgs` reads `..._GAMING_ARPGS`. Read at
  startup: empty reaches nothing, a non-numeric id stops the boot. Anything else is `403` naming the slug
  sent, never the variables.
* **Send.** `allowed_mentions.parse` is empty, so `@everyone` arrives as text. Send Unicode emoji, since
  Discord does not expand `:x:` in bot content. Content over `SIRENS_ECHO_REPLY_LIMIT` is `400`.
* **Refusals.** `405`, `400` for a bad or blank body, `502` for a Discord failure (status and code
  logged, no body), `503` with Discord off. `httpmessage_test.go` asserts each.
* **Not here.** Admission control, so only Discord's rate limit bounds a looping caller.

## Related

* [HTTP and MCP surfaces](sirens-echo-http.md) - the listener the private ingress and re-export hang off.
* [Tools](sirens-echo-tools.md) - the roster being re-exported.
* [Admission](sirens-echo-admission.md) - the gates the Discord transport applies.
