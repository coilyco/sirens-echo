#!/usr/bin/env bash
set -euo pipefail

# Provenance nobody has to remember: a dataset that cannot name its checkout
# cannot be compared. See docs/sirens-echo-rate-provenance.md.
if [ -z "${SIRENS_ECHO_RUNNER:-}" ]; then
  SIRENS_ECHO_RUNNER=$(git rev-parse --short HEAD 2>/dev/null || echo "")
  export SIRENS_ECHO_RUNNER
fi

# Fires without being remembered. Resolved rather than spelled, because a linked
# worktree has .git as a file and a -d test would skip it. See sirens-echo#307.
hook_path=$(git rev-parse --git-path hooks/pre-commit 2>/dev/null || true)
if [ -n "$hook_path" ] && [ ! -e "$hook_path" ] && command -v pre-commit >/dev/null 2>&1; then
  pre-commit install --install-hooks >/dev/null 2>&1 || true
fi

case "${1:-}" in
  setup)
    # What the block above does silently, done loudly and to a verdict.
    # Reinstalls over an existing hook, so an edited config lands. See the gate.
    if ! command -v pre-commit >/dev/null 2>&1; then
      echo "setup: pre-commit is not on PATH, so the commit gate cannot be installed" >&2
      exit 1
    fi
    pre-commit install --install-hooks
    ;;
  gate)
    # Nothing read the declared lane, so an agent could violate it with every
    # check green. In AGENTS.md frontmatter, not ward.yaml. See sirens-echo#329.
    declared=$(sed -n '2,/^---$/s/^  workflow: *//p' AGENTS.md | head -1)
    branch=$(git symbolic-ref --short HEAD 2>/dev/null || echo "")
    if [ "$declared" = "pull-request-and-merge" ] && [ "$branch" = "main" ]; then
      echo "gate: this repository is on the $declared lane, so main is not a" >&2
      echo "  branch to push. Create one, then open a pull request:" >&2
      echo "    git switch -c <owner>/<topic>" >&2
      exit 1
    fi
    # One habit instead of six. Four red mains in one evening were all caught by
    # pre-commit and missed by the verbs an engineer runs. See issue 305.
    gate_step() {
      local name=$1; shift
      # Per run, not a fixed path. Four seats share this host and a concurrent
      # gate corrupted this file into one a reader could not use.
      local log
      log=$(mktemp "${TMPDIR:-/tmp}/ward-gate.XXXXXX")
      printf '%-14s ' "$name"
      if "$@" >"$log" 2>&1; then
        echo PASS
        rm -f "$log"
      else
        echo FAIL
        # -a because one NUL turns the whole diagnostic into "Binary file
        # matches", which greps clean and leaves nothing to read.
        grep -a -ivE 'Passed|Skipped' "$log" >&2 || tail -20 "$log" >&2
        rm -f "$log"
        exit 1
      fi
    }
    # Dispatched through this script rather than ward, so the gate still runs
    # while its own definition is uncommitted. See docs/sirens-echo-gate.md.
    for verb in build policy-check vet test test-skips; do
      gate_step "$verb" bash "$0" "$verb"
    done
    # Several hooks enumerate git's own file list rather than the one they are
    # handed, so a file that has never been added is invisible. See issue 343.
    gate_marked=()
    while IFS= read -r gate_file; do
      gate_marked+=("$gate_file")
    done < <(git ls-files -o --exclude-standard)
    gate_unmark() {
      [ ${#gate_marked[@]} -gt 0 ] || return 0
      git rm --cached --quiet -- "${gate_marked[@]}" >/dev/null 2>&1 || true
    }
    if [ ${#gate_marked[@]} -gt 0 ]; then
      # Intent only: no content is staged, and the index is restored however
      # this run exits. See docs/sirens-echo-gate.md.
      git add --intent-to-add -- "${gate_marked[@]}"
      trap gate_unmark EXIT
    fi
    # Last, and on the final tree, because these hooks rewrite files.
    gate_step pre-commit pre-commit run --all-files
    gate_unmark
    trap - EXIT
    echo "gate: the tree is ready to push"
    ;;
  build)
    mkdir -p bin
    go build -o bin/sirens-echo ./cmd/sirens-echo
    go build -o bin/sirens-echo-policy-check ./cmd/sirens-echo-policy-check
    go build -o bin/sirens-echo-eval ./cmd/sirens-echo-eval
    ;;
  compose-bundles)
    # Local runs need a catalogue checkout; the image build clones a pinned ref.
    catalog=${AOS_CATALOG:-$HOME/projects/coilyco-flight-deck/agentic-os}
    if [ ! -d "$catalog/.agents/composed" ]; then
      echo "compose-bundles: set AOS_CATALOG to an agentic-os checkout" >&2
      exit 1
    fi
    bash scripts/stage-compose-sources.sh agent/bundles "$catalog"
    ;;
  vet)
    go vet ./...
    ;;
  test)
    go test ./...
    ;;
  tidy)
    go mod tidy
    ;;
  policy-check)
    go run ./cmd/sirens-echo-policy-check
    ;;
  evidence-scan)
    go run ./cmd/sirens-echo-evidence
    ;;
  test-skips)
    # A skip and a pass share an exit code and the word ok, so a guard can stop
    # running for months. See docs/sirens-echo-test-skips.md.
    allow=.ward/test-skips.allow
    fired=$(go test -v ./... 2>&1 |
      sed -n 's/^ *--- SKIP: \([A-Za-z0-9_]*\).*/\1/p' | sort -u)
    expected=$(sed -e 's/#.*//' -e 's/[[:space:]]//g' "$allow" |
      grep -v '^$' | sort -u)
    unexpected=$(comm -23 <(printf '%s\n' "$fired" | grep -v '^$') \
      <(printf '%s\n' "$expected" | grep -v '^$'))
    stale=$(comm -13 <(printf '%s\n' "$fired" | grep -v '^$') \
      <(printf '%s\n' "$expected" | grep -v '^$'))
    status=0
    if [ -n "$unexpected" ]; then
      echo "test-skips: these tests skipped and are not reviewed:" >&2
      printf '  %s\n' $unexpected >&2
      echo "Fix the test, or add it to $allow with the reason." >&2
      status=1
    fi
    # A stale entry is the same defect pointed the other way: it reads as a
    # known exception and nobody deletes it.
    if [ -n "$stale" ]; then
      echo "test-skips: these are allowlisted but no longer skip:" >&2
      printf '  %s\n' $stale >&2
      echo "Delete them from $allow." >&2
      status=1
    fi
    [ "$status" -eq 0 ] && echo "test-skips: reviewed skip set matches"
    exit "$status"
    ;;
  run-echo)
    go run ./cmd/sirens-echo
    ;;
  eval-echo)
    go run ./cmd/sirens-echo-eval
    ;;
  eval-deep)
    SIRENS_ECHO_DEFINITION=agent/sirens-deep.yaml \
      SIRENS_ECHO_EVALUATION_PACK=agent/evaluation-deep.yaml \
      go run ./cmd/sirens-echo-eval
    ;;
  board-deep)
    # Emits an annotation dataset on stdout and reports no verdict. Redirect it
    # to evaluations/ before grading, because the dataset is the evidence.
    SIRENS_ECHO_DEFINITION=agent/sirens-deep.yaml \
      SIRENS_ECHO_EVALUATION_PACK=agent/board-deep.yaml \
      go run ./cmd/sirens-echo-eval
    ;;
  rate-echo)
    # Emits a measurement dataset on stdout. Redirect it to evaluations/ before
    # reading, because every reply in it is the evidence a failure was real.
    SIRENS_ECHO_EVALUATION_PACK=agent/rate-echo.yaml \
      go run ./cmd/sirens-echo-eval
    ;;
  rate-deep)
    # Emits a measurement dataset on stdout. Redirect it to evaluations/ before
    # reading, because every reply in it is the evidence a failure was real.
    SIRENS_ECHO_DEFINITION=agent/sirens-deep.yaml \
      SIRENS_ECHO_EVALUATION_PACK=agent/rate-deep.yaml \
      go run ./cmd/sirens-echo-eval
    ;;
  rate-fixture-deep)
    # The data-borne injection pack. SIRENS_ECHO_TOOL_FIXTURE is exclusive with
    # the MCP roster, so this runs separately from rate-deep.
    SIRENS_ECHO_DEFINITION=agent/sirens-deep.yaml \
      SIRENS_ECHO_EVALUATION_PACK=agent/rate-fixture-deep.yaml \
      SIRENS_ECHO_TOOL_FIXTURE=agent/tool-fixture-injection.yaml \
      go run ./cmd/sirens-echo-eval
    ;;
  rate-fixture-tracker)
    # The filing-rule pack. SIRENS_ECHO_TOOL_FIXTURE is exclusive with the MCP
    # roster, so this runs separately from rate-echo.
    SIRENS_ECHO_EVALUATION_PACK=agent/rate-fixture-tracker.yaml \
      SIRENS_ECHO_TOOL_FIXTURE=agent/tool-fixture-tracker.yaml \
      go run ./cmd/sirens-echo-eval
    ;;
  rate-fixture-tracker-match)
    # The deduplication branch. Same prompt as rate-fixture-tracker, against a
    # search that finds an existing issue.
    SIRENS_ECHO_EVALUATION_PACK=agent/rate-fixture-tracker-match.yaml \
      SIRENS_ECHO_TOOL_FIXTURE=agent/tool-fixture-tracker-match.yaml \
      go run ./cmd/sirens-echo-eval
    ;;
  format)
    find cmd internal -type f -name '*.go' -exec gofmt -w {} +
    ;;
  *)
    echo "unknown Ward action: ${1:-}" >&2
    exit 2
    ;;
esac
