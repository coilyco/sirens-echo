#!/usr/bin/env bash
# Fails a main push that shipped no image, so the pipeline stopping looks
# different from the tests being flaky. See docs/FEATURES.md.
#
# publish-echo-image reports success only once an image reaches GHCR.
# Every other result leaves main without one, and each arrives silently:
# skipped when test failed so the publish never ran, cancelled when a later
# push superseded it, failure when the publish itself broke. A caller reading
# the run list cannot tell those from "not applicable" without this check.
#
# This reports the consequence. It does not cure any of the three causes.
set -euo pipefail

result="${PUBLISH_RESULT:-}"
sha="${GITHUB_SHA:-unknown}"

if [ "$result" = "success" ]; then
  echo "publish-echo-image succeeded, so ${sha} has an image in GHCR"
  exit 0
fi

case "$result" in
  skipped)
    echo "publish-echo-image was SKIPPED, so ${sha} has no image." >&2
    echo "The test job did not pass, so the publish never ran. The green" >&2
    echo "image-build job built an image and threw it away." >&2
    ;;
  cancelled)
    echo "publish-echo-image was CANCELLED, so ${sha} has no image." >&2
    echo "A later push superseded this one before it published." >&2
    ;;
  failure)
    echo "publish-echo-image FAILED, so ${sha} has no image." >&2
    echo "Read that job's log before assuming it was transient." >&2
    ;;
  "")
    echo "publish-echo-image reported no result at all, so treat ${sha}" >&2
    echo "as having no image until the registry says otherwise." >&2
    echo "" >&2
    echo "If EVERY main run reaches this branch, suspect the check rather" >&2
    echo "than the publish: the workflow may not be passing needs.<job>.result," >&2
    echo "in which case PUBLISH_RESULT is empty on success too. Confirm" >&2
    echo "against the registry before trusting a run of these." >&2
    ;;
  *)
    echo "publish-echo-image reported '${result}', which is not success," >&2
    echo "so treat ${sha} as having no image." >&2
    ;;
esac

echo "" >&2
echo "Do not pin ${sha} for rollout. Pin the newest commit whose publish" >&2
echo "job succeeded. Deploy pulls with pullPolicy Always onto a Recreate" >&2
echo "strategy, so a tag that does not exist takes the workload down" >&2
echo "rather than failing the rollout safely." >&2
exit 1
