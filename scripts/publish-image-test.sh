#!/usr/bin/env bash
# Drives publish-image.sh against a stub docker and git, so the retry rule is
# checked without a daemon or a registry. See docs/sirens-echo-deploy.md.
set -euo pipefail

script_dir=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)
work=$(mktemp -d "${TMPDIR:-/tmp}/publish-image-test.XXXXXX")
trap 'rm -rf "${work}"' EXIT
mkdir "${work}/bin"

# The stub docker fails builds as FIRST_BUILD_OUTPUT says, then succeeds, and
# logs every build's nonce so the test can see each attempt got its own.
cat >"${work}/bin/docker" <<'STUB'
#!/usr/bin/env bash
case "$1" in
  build)
    n=$(($(cat "${STUB_DIR}/builds" 2>/dev/null || echo 0) + 1))
    echo "$n" >"${STUB_DIR}/builds"
    for arg in "$@"; do
      case "$arg" in SIRENS_ECHO_BUILD_NONCE=*) echo "${arg#*=}" >>"${STUB_DIR}/nonces" ;; esac
    done
    if [ "$n" -eq 1 ] && [ -n "${FIRST_BUILD_OUTPUT:-}" ]; then
      echo "${FIRST_BUILD_OUTPUT}" >&2
      exit 1
    fi
    ;;
  push) echo pushed >>"${STUB_DIR}/pushes" ;;
esac
STUB
cat >"${work}/bin/git" <<'STUB'
#!/usr/bin/env bash
printf 'aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa\trefs/heads/main\n'
STUB
chmod +x "${work}/bin/docker" "${work}/bin/git"

run() {
  rm -f "${work}/builds" "${work}/nonces" "${work}/pushes"
  PATH="${work}/bin:${PATH}" STUB_DIR="${work}" REGISTRY_TOKEN=t FORGEJO_EGRESS_PROXY=http://p \
    GITHUB_SHA=bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb GITHUB_RUN_ID=7 FIRST_BUILD_OUTPUT="$1" \
    bash "${script_dir}/publish-image.sh" >"${work}/out" 2>&1
}

# A lost stage image retries once with a fresh nonce and then publishes.
run 'invalid from flag value compose: No such image: sha256:f13b'
[ "$(cat "${work}/builds")" = 2 ] || { echo "lost stage image: want 2 builds" >&2; exit 1; }
[ "$(sort -u "${work}/nonces" | wc -l | tr -d ' ')" = 2 ] || { echo "retry reused its nonce" >&2; exit 1; }
[ -f "${work}/pushes" ] || { echo "retry never pushed" >&2; exit 1; }

# Any other build failure fails at once, with no second attempt and no push.
if run 'go: build failed'; then echo "a real build fault was swallowed" >&2; exit 1; fi
[ "$(cat "${work}/builds")" = 1 ] || { echo "real fault: want 1 build" >&2; exit 1; }
[ ! -f "${work}/pushes" ] || { echo "a failed build pushed" >&2; exit 1; }

echo "publish-image retry rule holds"
