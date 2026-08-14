# MCP roster

The roster names the MCP servers a deployment gives Echo, and how to reach each
one. Deployment chooses which servers run, the way an operator chooses their own
MCP servers. Echo validates that an entry carries the fields its transport needs
and none belonging to another, never which server is acceptable. No server name,
endpoint, or command appears anywhere in this repository.

The file uses the `mcpServers` shape shared with mcporter, Claude Code, and
Codex rather than a format of Echo's own, so a server registered for a harness
is expressible here unchanged.

## Transports

The shared shape carries no transport discriminator, so a `command` means stdio
and an endpoint means HTTP. `transport` is the one key Echo adds, needed only to
choose between the two HTTP transports.

- `streamable` - the current HTTP transport, and the default. Takes a URL.
- `sse` - the 2024-11-05 HTTP and SSE transport. Takes a URL.
- `stdio` - a child process speaking newline-delimited JSON over its pipes.

A URL transport takes a `baseUrl`, or `url` as its alias, an optional `headers`
map, and no `command`, `args`, or `env`. A `stdio` entry takes a `command`,
optional `args`, an optional `env` map, and no endpoint or headers.

`SIRENS_ECHO_MCP_ROSTER` names the file, and an unset variable is a valid
no-tool boundary. Any string field resolves `${VAR}` from Echo's environment, so
a secret reaches an entry without being written into it, and an unset one fails
validation. A credential goes in [`headers`](sirens-echo-mcp-roster-auth.md).

```yaml
mcpServers:
  eco:
    baseUrl: https://eco-mcp:9000/mcp
  forgejo:
    baseUrl: ${SIRENS_ECHO_FORGEJO_MCP_URL}
  local:
    command: /usr/bin/some-mcp
    args: [--read-only]
    env: {SOME_TOKEN: "${SOME_TOKEN}"}
```

JSON is a YAML subset, so a roster still written as JSON parses unchanged and a
lane converts on its own schedule. Keys stay camelCase: the shape is the shared
one. Keys Echo does not read, such as `description`, `imports`, or an `x-`
extension, belong to another tool and are ignored rather than rejected.

## Trust and stdio

A `stdio` entry names a command Echo executes, so whoever can write the roster
can run a process inside Echo's pod. That is the intended model, matching the
layer that already chooses Echo's image, arguments, and mounts, and it means the
roster source deserves the write protection the pod spec gets. A child receives
exactly the `env` map, never Echo's own environment, and exits when Echo does.

## Connection lifetime

Connections are held across turns, so a turn pays no handshake and no
rediscovery. A turn borrows a supervised session and returns it, and only
shutdown closes one.

A tool listing is cached until something invalidates it. A transport carrying
server-initiated messages invalidates on `tools/list_changed`. Streamable cannot
while its standalone SSE stream stays disabled, so those listings expire after
an hour. A failed connection retries with backoff between five seconds and two
minutes. Discovery traffic carries the calling turn's trace context even though
the connection outlives that turn. The model can end that hour early with a
[harness tool](sirens-echo-harness-tools.md).

## Failure

Connection is per server. One that cannot connect or list contributes no tools
and the turn continues on the rest, named to the model so it reports the gap.
Only an entirely unreachable roster stops the turn.

## See also

See [configuration](sirens-echo-config.md), [runtime MCP tools](sirens-echo-tools.md),
[harness tools](sirens-echo-harness-tools.md), [MCP resources](sirens-echo-mcp-resources.md).
