# Private HTTP entrypoint

The same `runTurn` path Discord uses is available through `POST /v1/turn` on the process's private HTTP
listener, and a deployment can disable Discord and use this as its only ingress. The endpoint accepts a
JSON object with `author`, `content`, an optional `request_id`, and optional bounded `history`, and
returns the validated reply without sending a Discord message. **It bypasses only Discord's channel,
mention, and duplicate gates**: admission, Agent Proxy, MCP tool calls, response validation, grounding,
and guarded Forgejo issue handling are unchanged. The listener also serves `POST /v1/message`, a
verbatim post to an allowlisted channel. See [transports](sirens-echo-transports.md#posting-a-message).

The same turn is served over MCP at `/mcp` on the same listener, as a single `turn` tool taking
`author`, `content`, and optional `history`, so a fleet client reaches Echo natively instead of learning
this JSON contract. **`turn` bypasses nothing there either**, and its turns are labelled `mcp` in
telemetry. One surface there is an exception and is off by default:
[roster re-export](sirens-echo-transports.md#roster-re-export).
Admission keys off `X-Sirens-Caller` when a client sends one and the declared MCP client name otherwise,
so a client can still separate its own callers, and a caller-fixable problem comes back as an error
result rather than a protocol error so the calling model can correct itself. **Rostering Echo into its
own roster is not guarded against**: turns are serialized, so a self-call waits on the slot its caller
already holds and fails on the queue timeout rather than recursing.

**Reachability is decided at the network layer rather than by the process.** The k3s deployment binds
`0.0.0.0:8080` and publishes no Ingress, certificate, DNS record, or public resource. **Two paths reach
it, and a lane has one or both.** Where a lane runs Echo's Tailscale sidecar, callers arrive on its
MagicDNS name. Where the deployment binds a NodePort on kai-server they arrive there: `sirens-echo`
30120 and `sirens-deep` 30121, **one port per lane covering both surfaces**,
because `HTTPHandler` hangs `/mcp` and the `/v1` paths off one mux on one listener. The ClusterIP is
retained either way, so the in-namespace path is unchanged.

**A lane may run `tailnet.enabled: false`**, in which case it has no sidecar, no MagicDNS name, and
the NodePort is its only path. **The boundary is the same either way**: the NodePort is LAN and
tailnet only, the home router forwards nothing to it, so reaching `/v1/turn` still requires being an

```sh
curl -sS http://sirens-echo:8080/v1/turn -H 'Content-Type: application/json' \
  -H 'X-Sirens-Caller: manual-test' -d '{"author":"manual test","content":"Eco server status?"}'
```

`GET /healthz` checks local listener liveness and `GET /readyz` checks the configured route through
Agent Proxy without inference. Both emit bounded metrics and no logs or spans. `POST /v1/turn` remains
traced, and its server span extracts W3C context and parents the shared Community turn.

## The rejection contract

**Every rejection below is a caller error answered with a `4xx`, so a malformed request is never counted
against the service's own error rate.** `internal/community/http_test.go` asserts each row.

* any method but `POST` - `405` with `Allow: POST`; `POST /healthz` likewise; an unknown path is `404`.
* body that is not a JSON object - `400` `request body must be a JSON object`.
* a field this contract does not define - `400` naming the offending field.
* absent, empty, or blank `content` with no `prompt` - `400` `content is required`.
* `author` over 256 runes or `content` over 16000 runes - `400` `author or content is too long`.
* `history` longer than `max_context_messages` - `400` `history exceeds the configured context limit`.
* body over 64 KiB - `400` `request body exceeds the 65536 byte limit`.
* `prompt` with an empty server or name - `400` `prompt requires a server and a name`; one naming an
  unrostered server is `400` naming the server the caller supplied.

The caps count **runes rather than bytes**, so a multibyte author is not charged for bytes it did not
spend, and the `history` limit is inclusive so a caller filling the configured context exactly is
admitted. An unrostered `prompt` server is named back because the caller supplied it, while **a
transport failure stays generic so a resolution error cannot carry an endpoint, host, or port**. The
body cap and the rune caps are separate limits and say so separately, since a caller can only act on the
one they broke. An oversize body is refused rather than routed to the virtual-file upload path, Kai's
decision on issue 157, waiting on that path existing.

Two behaviours are tolerated and pinned by characterization tests, **so the issue that fixes one has a
test to flip rather than delete**. Decoding is not strict, so an unknown field is accepted in silence
rather than refused (issue 173). And `X-Sirens-Caller` splits the per-user tier alone, so one HTTP
caller can shed another's turn through the shared pending counter, documented rather than changed
because **the header is caller-asserted, so isolating a second tier on it would mint budgets rather than
bound them** (issue 182).

**Every `429` carries `Retry-After`, including a pending-cap shed**, whose path charges its own
one-second bucket so the advertised wait is a real window rather than a constant.
`TestQueueDenialCarriesRetryAfter` asserts both halves (issue 181).

## A trusted caller

`/v1/turn` could not tell one caller from another. Authenticating the caller is the first half of fixing
that; sessions and the prompt's principal assertion are separate, and the order matters. Echo is reached over the
tailnet or the LAN behind it and has never been on the public internet, which does not make
authentication unimportant, it makes the threat model **which tailnet peer** rather than **anyone at
all**, a bounded problem.

`SIRENS_ECHO_HTTP_TOKEN` supplies the token and **unset trusts nobody**, exactly what this endpoint did
before the token existed, so enabling identity is a deployment decision and landing the code changes
nothing on its own. A request is trusted when it presents `Authorization: Bearer <token>` matching the
configured value, compared in constant time, because **a check that leaks its own answer through timing
looks like a control and is not one**.

**`X-Sirens-Caller` is the trap in this feature.** There is a header named like an identity, carrying a
caller-supplied name, already flowing into the turn's requester, and the smallest possible version of
"add identity" is to trust it, which is treating an unauthenticated string as a principal. It would pass
every test anyone would think to write, because the plumbing works and the value arrives where it is
expected. So the trusted input is a different input, and the self-asserted one keeps its old meaning as
a rate-limit key.

**Sessions come after, not before.** A session is a handle to retained conversation, and an endpoint
that accepts a session id from an unauthenticated caller and returns that session's history discloses
conversations to whoever guesses an id. `history_count: 0` is therefore not merely a missing feature, it
is the reason this endpoint is currently safe to expose without authentication.

Trust does nothing today beyond a span attribute recording whether the caller authenticated, which is
deliberate: **the value of the change is that the distinction exists and is recorded**, and every
consumer of it is a separate decision. The prompt still asserts that an HTTP caller is not the
principal, and making that conditional means rebuilding the system prompt per request, since it is built
once at startup.
