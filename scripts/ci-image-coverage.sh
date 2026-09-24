#!/usr/bin/env bash
# Reports whether main's tip is deployable, from outside any push's run.
# See docs/FEATURES.md.
#
# The in-run publish-observed job cannot see a cancelled publish, because a
# cancelled run cancels the observer with it. This asks the package registry
# instead, on a schedule, which is the one vantage point a torn-down run
# cannot reach.
#
# It gates on the tip rather than on every commit. A cancelled publish for an
# intermediate commit costs nothing once a later push publishes, but a tip with
# no image means there is nothing to roll out, which is the state worth waking
# someone for.
set -euo pipefail

FORGE="${FORGEJO_BASE:-https://forgejo.coilysiren.me}"
OWNER="${IMAGE_OWNER:-coilyco}"
PACKAGE="${IMAGE_PACKAGE:-sirens-echo}"
# A just-pushed commit is still building, so it is not yet evidence of loss.
GRACE_SECONDS="${IMAGE_GRACE_SECONDS:-1200}"
DEPTH="${IMAGE_HISTORY_DEPTH:-25}"
# A CI checkout has no origin/main remote-tracking ref, so the caller names one.
REF="${IMAGE_HISTORY_REF:-origin/main}"

tags=$(mktemp)
trap 'rm -f "$tags"' EXIT

if ! curl -sfS "${FORGE}/api/v1/packages/${OWNER}?type=container&limit=200" \
  | jq -r --arg name "$PACKAGE" '.[] | select(.name==$name) | .version' \
  | sort >"$tags"; then
  echo "could not read the package registry, so image coverage is unknown" >&2
  echo "this is a check failure rather than a publish failure" >&2
  exit 1
fi

published=$(wc -l <"$tags" | tr -d ' ')
echo "registry lists ${published} published ${PACKAGE} tags"
if [ "$published" -eq 0 ]; then
  echo "no tags at all, which means the query is wrong or the registry is empty" >&2
  exit 1
fi

now=$(date +%s)
missing=0
total=0

echo ""
echo "state  commit    age    subject"
while read -r sha committed; do
  total=$((total + 1))
  age=$((now - committed))
  if grep -qx "$sha" "$tags"; then
    state="ok   "
  elif [ "$age" -lt "$GRACE_SECONDS" ]; then
    state="build"
  else
    state="MISS "
    missing=$((missing + 1))
  fi
  printf '%s  %s  %4dm  %s\n' \
    "$state" "${sha:0:8}" "$((age / 60))" "$(git log -1 --format=%s "$sha" | cut -c1-48)"
done < <(git log "$REF" --format='%H %ct' -"${DEPTH}")

echo ""
echo "${missing} of ${total} recent main commits have no image"

newest=$(git log "$REF" -1 --format='%H')
newest_age=$(($(date +%s) - $(git log -1 --format=%ct "$newest")))

if grep -qx "$newest" "$tags"; then
  echo "main's tip ${newest:0:8} has an image, so there is something to roll out"
  exit 0
fi
if [ "$newest_age" -lt "$GRACE_SECONDS" ]; then
  echo "main's tip ${newest:0:8} is $((newest_age / 60))m old and may still be publishing"
  exit 0
fi

echo "" >&2
echo "main's tip ${newest:0:8} has no image and is $((newest_age / 60))m old." >&2
echo "Nothing newer can be rolled out. Pin the newest commit marked ok above." >&2
echo "A publish is skipped when tests fail, cancelled when a later push" >&2
echo "supersedes it, and failed when the publish itself breaks." >&2
exit 1
