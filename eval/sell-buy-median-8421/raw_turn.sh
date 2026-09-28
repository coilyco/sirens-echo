#!/usr/bin/env bash
# One turn over raw JSON-RPC, printing the whole tools/call result, isError and content text included.
set -euo pipefail
U=http://kai-server:30120/mcp
H=(-H 'content-type: application/json' -H 'accept: application/json, text/event-stream')
S=$(curl -sS -D - -o /dev/null "${H[@]}" -d '{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-06-18","capabilities":{},"clientInfo":{"name":"probe-scientist","version":"0"}}}' $U | awk -F': ' 'tolower($1)=="mcp-session-id"{print $2}' | tr -d '\r')
curl -sS "${H[@]}" -H "mcp-session-id: $S" -d '{"jsonrpc":"2.0","method":"notifications/initialized"}' $U >/dev/null
ARGS=$(python3 -c 'import json,sys; print(json.dumps({"author":"probe-scientist","content":sys.argv[1]}))' "$1")
curl -sS -m 300 "${H[@]}" -H "mcp-session-id: $S" -d "{\"jsonrpc\":\"2.0\",\"id\":2,\"method\":\"tools/call\",\"params\":{\"name\":\"turn\",\"arguments\":$ARGS}}" $U | sed -n 's/^data: //p'
