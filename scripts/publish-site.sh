#!/usr/bin/env bash
# Publish site/ to the discord.coilysiren.me bucket. See docs/sirens-echo-deploy.md.
set -euo pipefail

bucket="coilysiren-discord-site"

for name in PROJECT_SITES_PUBLISH_AWS_ACCESS_KEY_ID PROJECT_SITES_PUBLISH_AWS_SECRET_ACCESS_KEY; do
  if [ -z "${!name:-}" ]; then
    echo "publish-site: ${name} is not set. It is a repository secret." >&2
    exit 1
  fi
done

# sync --delete from an incomplete tree empties the live site.
for required in site/index.html site/404.html; do
  [ -s "${required}" ] || {
    echo "publish-site: ${required} is missing or empty, refusing to sync." >&2
    exit 1
  }
done

export AWS_ACCESS_KEY_ID="${PROJECT_SITES_PUBLISH_AWS_ACCESS_KEY_ID}"
export AWS_SECRET_ACCESS_KEY="${PROJECT_SITES_PUBLISH_AWS_SECRET_ACCESS_KEY}"
export AWS_DEFAULT_REGION="us-east-1"

# max-age=0 stands in for an invalidation, which would need the distribution id.
aws s3 sync site/ "s3://${bucket}/" --delete --no-progress \
  --cache-control 'public,max-age=0,must-revalidate'
served=$(aws s3api head-object --bucket "${bucket}" --key index.html --query ContentType --output text)
case "${served}" in
  text/html*) echo "publish-site: index.html serves as ${served}" ;;
  *) echo "publish-site: index.html serves as ${served}, not text/html" >&2; exit 1 ;;
esac

echo "publish-site: published site/ to https://discord.coilysiren.me/"
