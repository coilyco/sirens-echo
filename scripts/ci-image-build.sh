#!/usr/bin/env bash
# Builds the Sirens Echo image without publishing, so a Dockerfile or build
# script fault fails a pull request instead of the merge. See
# docs/sirens-echo-deploy.md.
set -euo pipefail

script_dir=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)
# shellcheck source=scripts/lib/catalog-head.sh
. "${script_dir}/lib/catalog-head.sh"

catalog_ref="${AOS_CATALOG_REF:-main}"
catalog_sha=$(catalog_head "${catalog_ref}")
echo "baking catalogue ${catalog_ref} at ${catalog_sha}"

# No tag anyone can push and no --push. This proves the build, nothing else.
# The publish job remains the only thing that ships an image.
: "${AGENT_COMPOSE_TOKEN:?the image fetches the private agent-compose release: set AGENT_COMPOSE_TOKEN, locally from gh auth token}"

docker build \
  --secret id=agent_compose_token,env=AGENT_COMPOSE_TOKEN \
  --build-arg AOS_CATALOG_REF="${catalog_ref}" \
  --build-arg AOS_CATALOG_HEAD="${catalog_sha}" \
  -t sirens-echo:pr-check .
echo "image build succeeded"
