#!/usr/bin/env bash
# The catalogue commit the image bakes, resolved by the caller and passed in as a
# build arg. See docs/sirens-echo-compose.md.

AOS_CATALOG_URL="https://forgejo.coilysiren.me/coilyco/agentic-os.git"

# catalog_head prints the commit a ref names right now. Branch first and tag
# second, because AOS_CATALOG_REF may be either and `git clone --branch` takes both.
catalog_head() {
  local ref="${1:?catalog_head needs a ref}"
  local candidate sha
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
