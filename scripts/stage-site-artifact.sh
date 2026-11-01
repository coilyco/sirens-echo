#!/usr/bin/env bash
# Stage site/ as the artifact coilyco/deploy publishes to S3. build-site.yml uploads
# site-artifact/ as the Actions artifact `site`, so the layout is site/ and nothing else.
# See docs/sirens-echo-deploy.md.
set -euo pipefail

# An incomplete tree would reach the bucket with --delete, so refuse it here too.
for required in site/index.html site/404.html; do
  [ -s "${required}" ] || {
    echo "stage-site-artifact: ${required} is missing or empty, refusing to stage." >&2
    exit 1
  }
done

# A fresh runner has none. A leftover would mix stale files into what deploy publishes.
[ ! -e site-artifact ] || { echo "stage-site-artifact: site-artifact/ already exists. Remove it first." >&2; exit 1; }
mkdir site-artifact
cp -R site site-artifact/site
echo "stage-site-artifact: staged $(find site-artifact/site -type f | wc -l | tr -d ' ') files"
