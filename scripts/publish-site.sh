#!/usr/bin/env bash
# Publish site/ to the discord.coilysiren.me bucket. See docs/sirens-echo-deploy.md.
set -euo pipefail

registry="forgejo.coilysiren.me"
ci_image="${registry}/coilyco/agentic-os:release"
bucket="coilysiren-discord-site"
no_proxy_hosts="127.0.0.1,localhost,forgejo.coilysiren.me,forgejo.forgejo.svc.cluster.local,.svc,.cluster.local"

for name in PROJECT_SITES_PUBLISH_AWS_ACCESS_KEY_ID PROJECT_SITES_PUBLISH_AWS_SECRET_ACCESS_KEY \
  REGISTRY_TOKEN FORGEJO_EGRESS_PROXY; do
  if [ -z "${!name:-}" ]; then
    echo "publish-site: ${name} is not set. This runs on the coilyco-gaming deploy runner." >&2
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

docker_config="$(mktemp -d)"
trap 'rm -rf "${docker_config}"' EXIT
chmod 700 "${docker_config}"
export DOCKER_CONFIG="${docker_config}"
printf '%s' "${REGISTRY_TOKEN}" \
  | docker login "${registry}" --username coilyco-ops --password-stdin
docker pull --quiet "${ci_image}"

# Bare --env NAME keeps the key out of the runner's argv.
export AWS_ACCESS_KEY_ID="${PROJECT_SITES_PUBLISH_AWS_ACCESS_KEY_ID}"
export AWS_SECRET_ACCESS_KEY="${PROJECT_SITES_PUBLISH_AWS_SECRET_ACCESS_KEY}"
export AWS_DEFAULT_REGION="us-east-1"

# max-age=0 stands in for an invalidation, which would need the distribution id.
tar -C site -cf - . \
  | docker run --rm -i \
    --env AWS_ACCESS_KEY_ID \
    --env AWS_SECRET_ACCESS_KEY \
    --env AWS_DEFAULT_REGION \
    --env HTTP_PROXY="${FORGEJO_EGRESS_PROXY}" \
    --env HTTPS_PROXY="${FORGEJO_EGRESS_PROXY}" \
    --env NO_PROXY="${no_proxy_hosts}" \
    --env http_proxy="${FORGEJO_EGRESS_PROXY}" \
    --env https_proxy="${FORGEJO_EGRESS_PROXY}" \
    --env no_proxy="${no_proxy_hosts}" \
    --entrypoint bash \
    "${ci_image}" -c "
      set -euo pipefail
      mkdir -p /tmp/site && tar -xf - -C /tmp/site
      aws s3 sync /tmp/site/ s3://${bucket}/ --delete --no-progress \
        --cache-control 'public,max-age=0,must-revalidate'
      served=\$(aws s3api head-object --bucket ${bucket} --key index.html --query ContentType --output text)
      case \"\${served}\" in
        text/html*) echo \"publish-site: index.html serves as \${served}\" ;;
        *) echo \"publish-site: index.html serves as \${served}, not text/html\" >&2; exit 1 ;;
      esac
    "

echo "publish-site: published site/ to https://discord.coilysiren.me/"
