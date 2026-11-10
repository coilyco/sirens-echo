#!/usr/bin/env bash
# Decode the base64 App key SSM stores into the PEM create-github-app-token takes,
# and write it to the step output. Every PEM line is masked first, because the
# decoded text is not a secret GitHub knows about and would print in a log.
set -euo pipefail

: "${APP_KEY_BASE64:?the private key input is empty}"
: "${GITHUB_OUTPUT:?GITHUB_OUTPUT is unset}"

pem=$(printf '%s' "$APP_KEY_BASE64" | base64 -d)
case "$pem" in
  *"-----BEGIN "*"PRIVATE KEY-----"*) ;;
  *)
    echo "::error::the private key input does not decode to a PEM private key" >&2
    exit 1
    ;;
esac

while IFS= read -r line; do
  [ -n "$line" ] && echo "::add-mask::$line"
done <<<"$pem"

delimiter="pem_$(od -An -N8 -tx1 /dev/urandom | tr -d ' \n')"
{
  echo "pem<<${delimiter}"
  echo "$pem"
  echo "${delimiter}"
} >>"$GITHUB_OUTPUT"
