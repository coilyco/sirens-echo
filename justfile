# Per-repo task manifest. Run `just` (or `just --list`) to see every verb.
#
# Recipes take trailing arguments directly: `just test ./internal/...`, where
# the retired form was `ward exec test -- ./internal/...`.
#
# Multi-step verbs route through scripts/task.sh, which also installs the
# commit hook on the way past and stamps SIRENS_ECHO_RUNNER so a dataset can
# name the checkout that produced it. Verbs naming their tool directly here do
# neither, which is the same split the previous manifest had.

set positional-arguments

# Default target: list every available recipe.
default:
    @just --list --unsorted

# Install the pre-commit hooks into this checkout. Run on a fresh clone or after .pre-commit-config.yaml changes.
setup *ARGS:
    @bash scripts/task.sh setup "$@"

# Run every check a push must pass, in order, with pre-commit last.
gate *ARGS:
    @bash scripts/task.sh gate "$@"

# Compile Sirens Echo and the Echo evaluation gate.
build *ARGS:
    @bash scripts/task.sh build "$@"

# Load every tracked definition and verify its selected response policy.
policy-check *ARGS:
    @bash scripts/task.sh policy-check "$@"

# Drive the coalescing lane with a synthetic feed. No proxy and no token needed.
smoke *ARGS:
    @go run ./cmd/sirens-echo-bridge -smoke 60s "$@"

# Build the production container image locally.
image *ARGS:
    @bash scripts/build-image-local.sh "$@"

# Parse the trusted Forgejo OCI publisher shell contract and run its retry test.
image-publish-check *ARGS:
    @bash -n scripts/publish-image.sh "$@"
    @bash scripts/publish-image-test.sh

# Stage the reviewed composed sources and materialize one bundle per role.
compose-bundles *ARGS:
    @bash scripts/task.sh compose-bundles "$@"

# Rewrite the tracked rendered-prompt snapshots for every definition.
prompt-dump *ARGS:
    @go run ./cmd/sirens-echo-prompt "$@"

# Fail when a rendered-prompt snapshot no longer matches its sources.
prompt-check *ARGS:
    @go run ./cmd/sirens-echo-prompt --check "$@"

# Rewrite the tracked reference of every number a deployment may set.
knobs *ARGS:
    @go run ./cmd/sirens-echo-knobs "$@"

# Fail when the tracked knob reference no longer matches the table.
knobs-check *ARGS:
    @go run ./cmd/sirens-echo-knobs --check "$@"

# Rewrite the tracked reference of every feature a deployment may turn on or off.
flags *ARGS:
    @go run ./cmd/sirens-echo-flags "$@"

# Fail when the tracked feature reference no longer matches the table.
flags-check *ARGS:
    @go run ./cmd/sirens-echo-flags --check "$@"

# Regenerate the agent's own-authority skill from the deployed guardfile.
guardfile-skill *ARGS:
    @go run ./cmd/sirens-echo-guardfile --guardfile ../../coilyco-bridge/deploy/services/sirens-echo/forgejo-mcp.mcp.kdl "$@"

# Fail when the own-authority skill is stale against the deployed guardfile.
guardfile-skill-check *ARGS:
    @go run ./cmd/sirens-echo-guardfile --check --guardfile ../../coilyco-bridge/deploy/services/sirens-echo/forgejo-mcp.mcp.kdl "$@"

# Load already-baked bundles and fail on one that did not compose.
role-load *ARGS:
    @go run ./cmd/sirens-echo-prompt --bundles agent/bundles "$@"

# Bake the bundles and check every role composes, in one step. What CI runs.
role-check *ARGS:
    @bash scripts/task.sh role-check "$@"

# go vet across the tree. Routed through the script so a hookless checkout installs the commit gate on the way past.
vet *ARGS:
    @bash scripts/task.sh vet "$@"

# Run the unit test suite. Routed through the script so a hookless checkout installs the commit gate on the way past.
test *ARGS:
    @bash scripts/task.sh test "$@"

# Count tool-call markup across committed run records. Gates nothing.
evidence-scan *ARGS:
    @bash scripts/task.sh evidence-scan "$@"

# Verify every test that skips is on the reviewed skip allowlist.
test-skips *ARGS:
    @bash scripts/task.sh test-skips "$@"

# go mod tidy. Routed through the script so a hookless checkout installs the commit gate on the way past.
tidy *ARGS:
    @bash scripts/task.sh tidy "$@"

# Measure the Sirens Echo intermittent-behavior rates. Gates nothing.
rate-echo *ARGS:
    @bash scripts/task.sh rate-echo "$@"

# Measure the Sirens Deep intermittent-behavior rates. Gates nothing.
rate-deep *ARGS:
    @bash scripts/task.sh rate-deep "$@"

# Measure data-borne injection against a tool fixture. Gates nothing.
rate-fixture-deep *ARGS:
    @bash scripts/task.sh rate-fixture-deep "$@"

# Measure the filing rule against a tracker fixture. Gates nothing.
rate-fixture-tracker *ARGS:
    @bash scripts/task.sh rate-fixture-tracker "$@"

# Measure the filing rule's dedupe branch against a found issue. Gates nothing.
rate-fixture-tracker-match *ARGS:
    @bash scripts/task.sh rate-fixture-tracker-match "$@"

# Format Go source.
format *ARGS:
    @bash scripts/task.sh format "$@"

# Run Sirens Echo. Requires Discord, channel, model, and MCP env.
run-echo *ARGS:
    @bash scripts/task.sh run-echo "$@"

# Run the non-mutating Sirens Echo cases through Agent Proxy.
eval-echo *ARGS:
    @bash scripts/task.sh eval-echo "$@"

# Run the non-mutating Sirens Deep cases through Agent Proxy.
eval-deep *ARGS:
    @bash scripts/task.sh eval-deep "$@"

# Emit the Sirens Deep human-graded board dataset. Gates nothing.
board-deep *ARGS:
    @bash scripts/task.sh board-deep "$@"

# Run the complete repository pre-commit suite.
pre-commit-all *ARGS:
    @pre-commit run --all-files "$@"

# Print the paired challenge list the board derives from eval/attributes.yaml.
attributes *ARGS:
    @bash scripts/attributes.sh list "$@"

# Fail when a declared attribute no longer resolves against the source it names.
attributes-check *ARGS:
    @bash scripts/attributes.sh check "$@"

# Grade a board dataset by hand with the shared aos-eval. One case per screen, saved after every decision.
grade DATASET *ARGS:
    @bash scripts/task.sh grade "{{DATASET}}" {{ARGS}}

# Rank a graded board's deductions into a failure taxonomy. Takes the dataset and its annotations.
taxonomy DATASET ANNOTATIONS *ARGS:
    @bash scripts/task.sh taxonomy "{{DATASET}}" "{{ANNOTATIONS}}" {{ARGS}}

# Check a board dataset against this deployment's aos-eval profile without grading it.
grade-check DATASET *ARGS:
    @bash scripts/task.sh grade-check "{{DATASET}}" {{ARGS}}
