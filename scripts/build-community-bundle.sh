#!/usr/bin/env bash
set -euo pipefail

provider=${1:?AOS provider checkout or mirror is required}
destination=${2:?bundle destination is required}

native_path() {
  if command -v cygpath >/dev/null 2>&1; then
    cygpath -m "$1"
    return
  fi
  printf '%s\n' "$1"
}

if [ ! -e "${provider}" ]; then
  echo "AOS provider does not exist: ${provider}" >&2
  exit 1
fi

mkdir -p "${destination}"
if [ -n "$(find "${destination}" -mindepth 1 -print -quit)" ]; then
  echo "bundle destination must be empty: ${destination}" >&2
  exit 1
fi

scratch=$(mktemp -d)
trap 'rm -rf -- "${scratch}"' EXIT

git clone \
  --quiet \
  --no-hardlinks \
  "${provider}" \
  "$(native_path "${scratch}/aos-public")"
cp agent/community-compose.kdl "${scratch}/request.kdl"
agent-compose compose \
  "$(native_path "${scratch}/request.kdl")" \
  --out "$(native_path "${scratch}/bundles")"

bundles=()
while IFS= read -r bundle; do
  bundles+=("${bundle}")
done < <(
  find "${scratch}/bundles" \
    -mindepth 1 \
    -maxdepth 1 \
    -type d \
    -not -name '.stage-*' \
    -print
)
if [ "${#bundles[@]}" -ne 1 ]; then
  echo "agent-compose produced ${#bundles[@]} bundle directories, expected 1" >&2
  exit 1
fi

agent-compose verify "$(native_path "${bundles[0]}")"
cp -R "${bundles[0]}/." "${destination}/"
