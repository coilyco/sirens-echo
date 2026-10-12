#!/usr/bin/env bash
set -euo pipefail

script_dir=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)
# shellcheck source=scripts/lib/catalog-head.sh
. "${script_dir}/lib/catalog-head.sh"

registry="forgejo.coilysiren.me"
image_name="coilyco/sirens-echo"

if [ -z "${REGISTRY_TOKEN:-}" ]; then
  echo "REGISTRY_TOKEN is required for the trusted image-publish lane." >&2
  exit 1
fi
if [ -z "${FORGEJO_EGRESS_PROXY:-}" ]; then
  echo "FORGEJO_EGRESS_PROXY is required for the Sirens Echo dependency build." >&2
  exit 1
fi

sha="${GITHUB_SHA:-$(git rev-parse HEAD)}"
case "${sha}" in
  *[!0-9a-f]*|"")
    echo "sirens-echo source sha is not a lowercase hexadecimal commit id." >&2
    exit 1
    ;;
esac
if [ "${#sha}" -ne 40 ]; then
  echo "sirens-echo source sha must be a full 40-character commit id." >&2
  exit 1
fi

image="${registry}/${image_name}:${sha}"
docker_config="$(mktemp -d "${TMPDIR:-/tmp}/publish-docker.XXXXXX")"
trap 'rm -rf "${docker_config}"' EXIT
chmod 700 "${docker_config}"
export DOCKER_CONFIG="${docker_config}"

printf '%s' "${REGISTRY_TOKEN}" \
  | docker login "${registry}" --username coilyco-ops --password-stdin

catalog_ref="${AOS_CATALOG_REF:-main}"
catalog_sha=$(catalog_head "${catalog_ref}")

echo "==> building ${image} against catalogue ${catalog_ref} at ${catalog_sha}"
# A shared daemon makes same-content stage images one image across runs, so the
# nonce gives each run its own. See docs/sirens-echo-deploy.md.
build_log="$(mktemp "${TMPDIR:-/tmp}/publish-build.XXXXXX")"
trap 'rm -rf "${docker_config}" "${build_log}"' EXIT
build_image() {
  local nonce
  nonce="${GITHUB_RUN_ID:-local}-$(date +%s)-$$-${1}"
  docker build \
    --pull \
    --build-arg HTTP_PROXY="${FORGEJO_EGRESS_PROXY}" \
    --build-arg HTTPS_PROXY="${FORGEJO_EGRESS_PROXY}" \
    --build-arg SIRENS_ECHO_REVISION="${sha}" \
    --build-arg SIRENS_ECHO_BUILD_NONCE="${nonce}" \
    --build-arg AOS_CATALOG_REF="${catalog_ref}" \
    --build-arg AOS_CATALOG_HEAD="${catalog_sha}" \
    -t "${image}" \
    . 2>&1 | tee "${build_log}"
}
# One retry, and only for the daemon losing a stage image mid-build. Any other
# failure is a real build fault and must fail the first time.
if ! build_image 1; then
  if grep -q 'No such image: sha256:' "${build_log}"; then
    echo "==> the daemon lost a stage image mid-build, retrying once with fresh stage ids" >&2
    build_image 2
  else
    exit 1
  fi
fi

echo "==> publishing ${image}"
docker push "${image}"

docker manifest inspect "${image}" >/dev/null
echo "verified immutable manifest ${image}"
