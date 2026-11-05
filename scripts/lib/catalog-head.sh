#!/usr/bin/env bash
# The catalogue commit the image bakes, resolved by the caller and passed in as a
# build arg. See docs/sirens-echo-compose.md.

AOS_CATALOG_URL="https://github.com/coilyco/agentic-os.git"

# aos_git_auth sends AOS_READ_TOKEN to the private catalogue only, in env, never argv.
# With no token set it does nothing, so a workstation uses its own git credentials.
aos_git_auth() {
  [ -n "${AOS_READ_TOKEN:-}" ] || return 0
  local basic count=${GIT_CONFIG_COUNT:-0} url
  basic=$(printf 'x-access-token:%s' "${AOS_READ_TOKEN}" | base64 | tr -d '\n')
  for url in "${AOS_CATALOG_URL%.git}" "${AOS_CATALOG_URL}"; do
    export "GIT_CONFIG_KEY_${count}=http.${url}.extraheader" "GIT_CONFIG_VALUE_${count}=AUTHORIZATION: basic ${basic}"
    count=$((count + 1))
  done
  export GIT_CONFIG_COUNT=$count
}

# catalog_head prints the commit a ref names right now. Branch first and tag
# second, because AOS_CATALOG_REF may be either and `git clone --branch` takes both.
catalog_head() {
  local ref="${1:?catalog_head needs a ref}"
  local candidate sha
  aos_git_auth
  for candidate in "refs/heads/${ref}" "refs/tags/${ref}"; do
    sha=$(GIT_TERMINAL_PROMPT=0 git ls-remote "${AOS_CATALOG_URL}" "${candidate}" | cut -f1)
    if [ -n "${sha}" ]; then
      printf '%s' "${sha}"
      return 0
    fi
  done
  echo "catalog-head: ${ref} names no branch or tag in ${AOS_CATALOG_URL}" >&2
  return 1
}
