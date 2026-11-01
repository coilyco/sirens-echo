#!/usr/bin/env bash
# Builds the site artifact and pushes it to GHCR. SITE_ARTIFACT_PUSH=0 builds only.
# See docs/sirens-echo-deploy.md.
set -euo pipefail

image="ghcr.io/coilyco/sirens-echo-site"

# An incomplete tree would reach the bucket with --delete, so refuse it here too.
for required in site/index.html site/404.html; do
  [ -s "${required}" ] || {
    echo "push-site-artifact: ${required} is missing or empty, refusing to build." >&2
    exit 1
  }
done

sha="${GITHUB_SHA:-$(git rev-parse HEAD)}"
case "${sha}" in
  *[!0-9a-f]*|"")
    echo "push-site-artifact: the source sha is not lowercase hexadecimal." >&2
    exit 1
    ;;
esac
if [ "${#sha}" -ne 40 ]; then
  echo "push-site-artifact: the source sha must be 40 characters." >&2
  exit 1
fi

# FROM scratch, so the filesystem is exactly /site/ and nothing else.
echo "push-site-artifact: building ${image}:${sha}"
printf 'FROM scratch\nCOPY . /site/\n' | docker build -f - \
  --label org.opencontainers.image.source=https://github.com/coilyco/sirens-echo \
  -t "${image}:${sha}" -t "${image}:main" site

if [ "${SITE_ARTIFACT_PUSH:-1}" = "0" ]; then
  echo "push-site-artifact: SITE_ARTIFACT_PUSH=0, built and not pushed"
  exit 0
fi

: "${GITHUB_TOKEN:?GITHUB_TOKEN is required to push}"
docker_config="$(mktemp -d "${TMPDIR:-/tmp}/push-site-artifact.XXXXXX")"
trap 'rm -rf "${docker_config}"' EXIT
chmod 700 "${docker_config}"
export DOCKER_CONFIG="${docker_config}"
printf '%s' "${GITHUB_TOKEN}" \
  | docker login ghcr.io --username "${GITHUB_ACTOR:-coilyco-ops}" --password-stdin

docker push "${image}:${sha}"
docker push "${image}:main"
docker manifest inspect "${image}:${sha}" >/dev/null
echo "push-site-artifact: pushed ${image}:${sha} and :main"
