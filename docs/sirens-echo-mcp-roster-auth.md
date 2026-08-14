# Authenticated MCP endpoints

Part of [the MCP roster](sirens-echo-mcp-roster.md). A hosted MCP server that
wants a credential gets it through `headers`, which a URL transport takes and a
`stdio` entry does not.

```yaml
mcpServers:
  vendor:
    url: https://mcp.vendor.example/mcp
    headers: {x-api-key: "${SIRENS_ECHO_VENDOR_API_KEY}"}
```

Values resolve `${VAR}` from Echo's environment the way every other roster
string does, so the credential reaches the entry without being written into it.
Deployment supplies the variable from its own secret, and the roster file stays
reviewable.

## Why not the query string

Most vendors also accept a key as a URL parameter. Echo does not treat that as
supported, for two reasons.

The identifier guard is the first. `IdentifierGuard.addEndpoint` returns early
unless the URL carries an explicit port, and even then it guards the host alone.
A portless HTTPS endpoint therefore contributes nothing to the forbidden set, so
a key in its query string is not among the values a reply is checked against. A
header value is instead guarded through `addOpaque`, the same path the Discord
token takes, so a credential delivered this way cannot appear in a reply.

The second is reach. A query string travels as part of the URL, which is the
field most likely to be carried intact into a log line, a span attribute, or an
upstream error message. A header is not attached to the request target and does
not ride along the same way.

## Validation

A header name must be RFC 7230 token characters. The pattern is deliberately
permissive about case, because the vendor picks the name and `x-api-key` is as
common as a capitalised spelling.

An empty value is rejected against the named server. Without that check an unset
variable would expand to empty and the call would reach the vendor anonymously,
surfacing as the vendor's authentication error rather than as the roster mistake
it is. This is the same failure shape an unset endpoint already has.

## One client, per-entry headers

The roster shares one `http.Client`. An entry that declares headers gets a
shallow copy carrying a wrapping `RoundTripper`, so the entries that declared
none keep the shared client untouched. The wrapper clones each request before
writing, because a `RoundTripper` must not modify the request it is handed and
the MCP SDK reuses one across a retry.
