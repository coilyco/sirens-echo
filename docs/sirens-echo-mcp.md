# The MCP roster

The roster names the MCP servers a deployment gives Echo, and how to reach each one. **Echo validates
that an entry carries the fields its transport needs and none belonging to another, never which server
is acceptable, and no server name, endpoint, or command appears in this repository.** The file uses the
`mcpServers` shape shared with mcporter, Claude Code, and Codex, so a server registered for a harness is
expressible here unchanged.

The shared shape carries no transport discriminator, so **a `command` means stdio and an endpoint means
HTTP**, and `transport` is the one key Echo adds, choosing between `streamable` (current, the default)
and `sse` (2024-11-05). A URL transport takes a `baseUrl`, or `url` as its alias, an optional `headers`
map, and no `command`, `args`, or `env`; a `stdio` entry takes a `command`, optional `args`, an optional
`env` map, and no endpoint or headers.

```yaml
mcpServers:
  eco: {baseUrl: https://eco-mcp:9000/mcp}
  teable: {baseUrl: "${SIRENS_ECHO_TEABLE_MCP_URL}"}
  local: {command: /usr/bin/some-mcp, args: [--read-only], env: {SOME_TOKEN: "${SOME_TOKEN}"}}
```

`SIRENS_ECHO_MCP_ROSTER` names the file and **an unset variable is a valid no-tool boundary**. Any
string field resolves `${VAR}` from Echo's environment, so a secret reaches an entry without being
written into it, and **an unset one fails validation**. JSON is a YAML subset, so a JSON roster parses
unchanged, keys stay camelCase, and keys Echo does not read (`description`, `imports`, an `x-`
extension) are ignored rather than rejected.

**A `stdio` entry names a command Echo executes, so whoever can write the roster can run a process
inside Echo's pod.** That is the intended model, matching the layer that already chooses Echo's image,
arguments, and mounts, and it means the roster source deserves the write protection the pod spec gets. A
child receives exactly the `env` map, never Echo's own environment, and exits when Echo does.

**Connections are held across turns**, so a turn borrows a supervised session and returns it, paying no
handshake and no rediscovery. A tool listing is cached until something invalidates it, a transport
carrying server-initiated messages invalidating on `tools/list_changed`; streamable cannot while its
standalone SSE stream stays disabled, **so those listings expire after an hour**, which the model can
end early with `harness__refresh_tools`. A failed connection retries with backoff between five seconds
and two minutes, and discovery traffic carries the calling turn's trace context even though the
connection outlives that turn. Connection is per server, so one that cannot connect or list contributes
no tools and the turn continues on the rest; **only an entirely unreachable roster stops the turn**.

## Authenticated endpoints

A hosted server that wants a credential gets it through `headers`, which a URL transport takes and a
`stdio` entry does not, resolving `${VAR}` like every other roster string.

**Most vendors also accept a key as a URL parameter, and Echo does not treat that as supported.**
`IdentifierGuard.addEndpoint` returns early unless the URL carries an explicit port, and even then
guards the host alone, so **a key in a portless endpoint's query string is not among the values a reply
is checked against**, while a header value is guarded through `addOpaque`, the path the Discord token
takes. A query string also travels as part of the URL, the field most likely to be carried intact into a
log line, a span attribute, or an upstream error message.

A header name must be RFC 7230 token characters, permissive about case because the vendor picks the
name. **An empty value is rejected against the named server**, because otherwise an unset variable would
expand to empty and the call would reach the vendor anonymously, surfacing as the vendor's
authentication error rather than the roster mistake it is. An entry declaring headers gets a shallow
copy of the shared `http.Client` with a wrapping `RoundTripper`, and **the wrapper clones each request
before writing**, because a `RoundTripper` must not modify the request it is handed.

## What each surface is for

The model receives every tool's schema on every turn but not **which server to reach for**, the question
members actually pose. `InitializeResult.Instructions` answers it, read off the handshake the harness already performs and
rendered beside the roster rather than generated into skill files at build time, **because that
generator failed open**: an unreachable server produced a file saying no schema was available.
**Absence is nothing**, so a server publishing no
instructions produces no entry, and **it describes, it does not authorise**, which the message says in
the same breath as the text. `SIRENS_ECHO_SERVER_GUIDANCE_BYTES` caps one entry: **a bound
supplied by the thing bounded is not a bound**.

## Resources

A server can hand Echo reference material directly, rather than asking through a tool call and paying
a round trip plus the tool-result bound. **The specification calls resources application-driven**,
naming automatic context inclusion as one pattern, which is why Echo pulls a resource in without the
model asking and does not do the same for prompts.

Only a resource whose `annotations.audience` names `assistant` is included, so **inclusion is a
deliberate server signal rather than an Echo assumption**. Qualifying resources are ordered by
`annotations.priority`, highest first, URI as tiebreak, and **ordering matters because the bounds cut
from the bottom**: at most eight documents and 8KB **per connected server, not per turn**, so an
eleven-server roster tops out at 88 and 90KB (#858). One over budget is truncated on a rune boundary and
marked, and an unreadable one is skipped, never failing the turn. **Included documents appear in a system message below the local policy, labelled as data to
answer from and never as instructions to follow**, because letting third-party content read as
instruction would give any connected server a way to redirect the turn.

**Prompts are selected, never injected**, being user-controlled: an HTTP caller names one with
`{"prompt": {"server": "eco", "name": "rules"}}`, and `content` is optional then because the prompt is
the request. The server's messages keep their user and assistant roles and **enter the turn as
conversation, not as system instruction**, and with no caller content the prompt's final user message
becomes the request. Required arguments are checked against `prompts/list` before the fetch, and **a
transport failure returns a generic message because its text could carry an endpoint**. Discord has no
selection surface yet.

## Call telemetry

`mcp.tool.call` carries `mcp.tool.outcome` and `mcp.tool.result_bytes`, so a reader holding one trace
can tell a call that returned rows from one that returned none, the outcome being the same three-state
`ToolOutcome` the disclosure footer renders. **Without them an investigation into a reply asserting
absence stops one step short of what the tool returned.** `mcp.tool.limit_bytes` and
`mcp.tool.truncated` sit on the same span, **on every call rather than only the truncated ones**, so a
filter on `truncated=false` answers how often the cap does not bind. `limit_bytes` is the cap rather
than the bytes delivered, the two differing by the truncation and spill notices appended afterwards.

**A session is not a request.** The MCP transport's client spans are named `mcp.session` plus the verb
rather than `HTTP POST`, because a connection that exists to stay open has a lifetime rather than a
latency, and **percentiles group on the span name, so two long sessions would set a whole service's
`HTTP POST` p99**. Expect those sessions to be long: they are no longer cut at the old whole-request
timeout.
