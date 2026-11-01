#!/usr/bin/env bash
# Asks coilyco/deploy to publish the artifact just pushed. Without the token the
# daily deploy cron publishes :main instead. See docs/sirens-echo-deploy.md.
set -euo pipefail

if [ -z "${DEPLOY_DISPATCH_TOKEN:-}" ]; then
  echo "dispatch-deploy-publish: DEPLOY_DISPATCH_TOKEN is not set, so deploy was not asked." >&2
  echo "The daily deploy cron publishes the :main artifact, so the site updates within a day." >&2
  exit 0
fi

sha="${GITHUB_SHA:?GITHUB_SHA is required}"
GH_TOKEN="${DEPLOY_DISPATCH_TOKEN}" gh workflow run publish-sirens-echo-site.yml \
  --repo coilyco/deploy -f sha="${sha}" || {
  echo "dispatch-deploy-publish: the dispatch to coilyco/deploy failed." >&2
  exit 1
}
echo "dispatch-deploy-publish: asked coilyco/deploy to publish ${sha}"
