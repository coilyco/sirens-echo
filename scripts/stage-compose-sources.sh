#!/usr/bin/env bash
# Bake one verified bundle per roster role. roles.kdl is purely additive: it
# grants a role its allowlisted skills, it does not decide which roles exist.
# sirens-echo-compose expands the graph and writes the declaration agent-compose
# consumes. See docs/sirens-echo-compose.md.
set -euo pipefail

# Usage: stage-compose-sources.sh <bundle out dir> <catalogue> [catalogue...]
# A wider layer passes several catalogues; the image build passes one.
bundles=${1:?usage: stage-compose-sources.sh <bundle out dir> <catalogue>...}
shift
[ $# -gt 0 ] || { echo "stage-compose-sources: name at least one catalogue" >&2; exit 1; }
catalog_flags=()
for catalog in "$@"; do
    catalog_flags+=(--catalog "$catalog")
done
compose_dir=${SIRENS_ECHO_COMPOSE_DIR:-agent/compose}

mkdir -p "$bundles"
# Composition runs from the declaration's directory, so this must be absolute.
bundles=$(cd "$bundles" && pwd)

# A scratch HOME with a minimal config keeps this hermetic. Without the config
# the run converges a whole home tree instead of materializing a bundle.
scratch_home=$(mktemp -d)
trap 'rm -rf "$scratch_home" "$compose_dir/skills" "$compose_dir"/aos-public.kdl "$compose_dir"/request.*.kdl' EXIT
mkdir -p "$scratch_home/.agent-compose" "$scratch_home/.claude"
cat > "$scratch_home/.agent-compose/agent-compose.yaml" <<YAML
load_points:
  claude: $scratch_home/.claude/CLAUDE.md
YAML

# The image build hands in a binary already compiled by the build stage, so
# that stage needs no Go toolchain work of its own.
generator=${SIRENS_ECHO_COMPOSE_BIN:-}
if [ -z "$generator" ]; then
    generator=$scratch_home/sirens-echo-compose
    go build -o "$generator" ./cmd/sirens-echo-compose
fi

# The roster is the authority on which roles exist, not roles.kdl. That file is
# purely additive: it grants skills to a role, it does not create one.
HOME=$scratch_home agent-compose roster >/dev/null
person=$scratch_home/.agent-compose/sources/personality/person.json
if [ ! -f "$person" ]; then
    echo "stage-compose-sources: agent-compose roster wrote no person.json" >&2
    exit 1
fi
roles=$(python3 -c 'import json,sys; d=json.load(open(sys.argv[1])); print("\n".join(d.get("role_order") or sorted(d["roles"])))' "$person")
if [ -z "$roles" ]; then
    echo "stage-compose-sources: the roster declares no role" >&2
    exit 1
fi
echo "stage-compose-sources: baking $(echo "$roles" | wc -w | tr -d ' ') roster roles"

# Community person-package roles bake beside the core roster ones, each from a
# request gaining a person-source node. See docs/sirens-echo-compose.md.
person_dir=${SIRENS_ECHO_PERSON_DIR:-agent/compose/person}
community_roles=""
person_rel=""
if [ -d "$person_dir" ]; then
    person_out=$(mktemp -d "$scratch_home/person.XXXXXX")
    HOME=$scratch_home agent-compose roster --person-source "$person_dir" --out "$person_out" >/dev/null
    community_roles=$(python3 -c 'import json,sys; d=json.load(open(sys.argv[1])); print("\n".join(d.get("role_order") or sorted(d["roles"])))' "$person_out/person.json")
    if [ -z "$community_roles" ]; then
        echo "stage-compose-sources: $person_dir declares no role" >&2
        exit 1
    fi
    # A slug owned by both rosters would silently overwrite a core bundle.
    for role in $community_roles; do
        if echo "$roles" | grep -qx "$role"; then
            echo "stage-compose-sources: role $role exists in both rosters" >&2
            exit 1
        fi
    done
    person_rel=$(python3 -c 'import os,sys; print(os.path.relpath(os.path.abspath(sys.argv[1]), os.path.abspath(sys.argv[2])))' "$person_dir" "$compose_dir")
    echo "stage-compose-sources: baking $(echo "$community_roles" | wc -w | tr -d ' ') community roles from $person_dir"
fi

# Per-role seat names, because one request template bakes every roster role and
# an identity in it would rename all of them. See docs/sirens-echo-identity.md.
seat_identity() {
    case "$1" in
        sysadmin) printf '    identity name="Echo" pronouns="it"\n' ;;
        *) printf '' ;;
    esac
}

# One agent alone in a guild has no seat to defer to, so Dowel drops what
# platform defers: one boundary under v3, not two. agent-compose#304.
seat_boundary_omissions() {
    case "$1" in
        platform) printf '    boundary-omit "suggest-external-comms"\n' ;;
        *) printf '' ;;
    esac
}

# person carries a request person-source node for community-package roles, so
# agent-compose selects the package instead of the embedded Core Roster.
bake_role() {
    role=$1
    person=${2:-}
    "$generator" "${catalog_flags[@]}" --role "$role" --compose-dir "$compose_dir"
    out=$bundles/$role
    rm -rf "$out"
    mkdir -p "$out"
    identity=$(seat_identity "$role")
    omissions=$(seat_boundary_omissions "$role")
    awk -v role="$role" -v identity="$identity" -v omissions="$omissions" -v person="$person" '
        /^    role "/ {
            print "    role \"" role "\""
            if (identity != "") { print identity }
            if (omissions != "") { print omissions }
            if (person != "") { print person }
            next
        }
        { print }
    ' "$compose_dir/request.kdl" > "$compose_dir/request.$role.kdl"
    ( cd "$compose_dir" && HOME=$scratch_home agent-compose compose "request.$role.kdl" --out "$out" >/dev/null )
    # The materializer names the tree by content hash; flatten it so the role
    # slug alone selects a bundle at runtime.
    tree=$(find "$out" -mindepth 1 -maxdepth 1 -type d | head -1)
    mv "$tree"/* "$out"/ && rmdir "$tree"
    HOME=$scratch_home agent-compose verify "$out"
}

for role in $roles; do
    bake_role "$role"
done
for role in $community_roles; do
    bake_role "$role" "    person-source \"$person_rel\""
done

# A vendored core personality is a checked mirror, not a fork: each body must
# match what the embedded roster bakes. See docs/sirens-echo-person.md.
for vendored in "$person_dir"/definitions/skills/personality-*/SKILL.md; do
    [ -e "$vendored" ] || continue
    slug=$(basename "$(dirname "$vendored")")
    baked=$(find "$bundles" -path "*/content/skills/roster%3Acore/$slug/SKILL.md" | head -1)
    if [ -z "$baked" ]; then
        echo "stage-compose-sources: $slug has no core counterpart in this bake, drift unchecked"
        continue
    fi
    if ! cmp -s "$vendored" "$baked"; then
        echo "stage-compose-sources: $vendored drifted from the embedded core body ($baked)" >&2
        exit 1
    fi
done
