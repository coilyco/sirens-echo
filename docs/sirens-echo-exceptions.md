# Sirens Echo exception taxonomy

SigNoZ groups exceptions by `service.name` and `exception.type`. Echo records
every failure through one typed, source-controlled catalog so grouping and
alert filters remain stable.

## Field contract

- Grouping type - `exception.type` is one of 23 unique `sirens_echo.*` values.
- Human wording - `exception.message` and the span status description use the
  same fixed sentence from the catalog.
- Operational tags - `error.stage` has nine possible values and
  `error.outcome` is a fixed outcome owned by the selected catalog entry.
- Span projection - `error.type`, `error.stage`, and `error.outcome` repeat the
  same bounded values for trace search and alert filters.
- Stack traces - Echo emits no `exception.stacktrace`, preventing source or
  filesystem paths from entering retained exception data.
- Redaction fallback - an invalid or unclassified code becomes
  `sirens_echo.telemetry.unclassified` without retaining the supplied value.

## Cardinality

Twenty-two types represent operational paths across the `turn`, `history`,
`validation`, `forgejo`, `reply`, `mcp`, `model`, and `http` stages. The
twenty-third type is the `telemetry` fallback. This is the catalog's hard
grouping bound. Adding a failure path requires an explicit catalog entry and a
reviewed increase to that bound.

## Redaction boundary

The recording API accepts only the catalog's numeric code type. Upstream error
text, request identifiers, member data, URLs and paths, credentials, and
payloads cannot be passed into the exception event.

Regression coverage exercises every catalog entry. It checks the event, span
attributes, status description, stack-trace omission, cardinality, and safe
fallback. Representative upstream, request, user, path, credential, and payload
values must not enter retained exception fields.

See [the observability guide](sirens-echo-observability.md) for the surrounding
trace, log, metric, and SigNoZ contracts.
