#!/usr/bin/env bash
# authority-case-check: the logic behind .github/workflows/authority-case-check.yml, one
# subcommand per workflow step, so api/test_authority_case_check.py can drive every branch.
#
# Same semantics as the council repository's authority-case-check workflow: the PR body's
# AuthorityCase (or legacy cc-task) reference is parsed and validated, and protected-path changes
# are named. During migration nothing fails: a missing reference and a protected path are warnings.
#
#   parse     PR_BODY in; case_id, slice_id, pre_methodology and cc_task appended to $GITHUB_OUTPUT
#   validate  CASE_ID, SLICE_ID, PRE_METHODOLOGY and CC_TASK in
#   paths     PR_BASE_SHA in; the diff from it to HEAD in the current repository
set -euo pipefail

parse() {
  local body="${PR_BODY:-}" case_id slice_id cc_task pre_methodology=false
  # grep -m1 and here-strings, never a pipe into head: under pipefail a long PR body would
  # otherwise end the step with SIGPIPE.
  case_id=$(grep -m1 -oiE 'CASE-[A-Z0-9-]+' <<<"$body" || true)
  case_id=${case_id%%$'\n'*}
  slice_id=$(grep -m1 -oiE 'SLICE-[A-Z0-9-]+' <<<"$body" || true)
  slice_id=${slice_id%%$'\n'*}
  if grep -qiE 'pre-methodology|pre_methodology' <<<"$body"; then
    pre_methodology=true
  fi
  # The match is case-insensitive, so the prefix strip is too (`CC-TASK: x` yields x).
  cc_task=$(grep -m1 -oiE 'cc-task:[[:space:]]*`?[a-z0-9-]+' <<<"$body" | sed -E 's/^cc-task:[[:space:]]*`?//I' || true)
  cc_task=${cc_task%%$'\n'*}
  {
    echo "case_id=$case_id"
    echo "slice_id=$slice_id"
    echo "pre_methodology=$pre_methodology"
    echo "cc_task=$cc_task"
  } >>"${GITHUB_OUTPUT:?GITHUB_OUTPUT is unset}"
  echo "Parsed: case=$case_id slice=$slice_id pre_methodology=$pre_methodology cc_task=$cc_task"
}

validate() {
  if [ "${PRE_METHODOLOGY:-}" = "true" ]; then
    echo "PASS — pre-methodology PR (migration compat)."
    return 0
  fi
  if [ -n "${CC_TASK:-}" ] && [ -z "${CASE_ID:-}" ]; then
    echo "PASS — legacy cc-task reference '$CC_TASK' (migration compat)."
    return 0
  fi
  if [ -z "${CASE_ID:-}" ] && [ -z "${CC_TASK:-}" ]; then
    echo "::warning::No AuthorityCase or cc-task reference found in PR body. After migration, this will be required."
    return 0
  fi
  echo "PASS — AuthorityCase reference: $CASE_ID / ${SLICE_ID:-}"
}

paths() {
  local base="${PR_BASE_SHA:-}" changed file protected_hit=false protected_files=""
  if [ -z "$base" ]; then
    echo "No base SHA — skipping path check."
    return 0
  fi
  git fetch --quiet origin "$base" 2>/dev/null || true
  changed=$(git diff --name-only "$base"...HEAD 2>/dev/null || echo "")
  if [ -z "$changed" ]; then
    echo "No changed files detected."
    return 0
  fi
  # Reins' governance surfaces: its governance and security policy, its CI gates and their
  # scripts, its licence terms, the estate's one credential resolver, and authority and custody.
  protect() {
    protected_hit=true
    protected_files="${protected_files}  - ${1}"$'\n'
  }
  while IFS= read -r file; do
    case "$file" in
      GOVERNANCE.md | SECURITY.md | CODEOWNERS | .github/CODEOWNERS | LICENSE | NOTICE.md) protect "$file" ;;
      .github/workflows/* | .github/scripts/*) protect "$file" ;;
      api/hapax_secret.py | scripts/hapax-secret | api/reins_authorization.py) protect "$file" ;;
      api/reins_claim_custody.py) protect "$file" ;;
    esac
  done <<<"$changed"
  if [ "$protected_hit" = true ]; then
    echo "::warning::Protected governance paths changed — requires T2/T3 AuthorityCase evidence:"
    echo "$protected_files"
  fi
  echo "Changed files: $(echo "$changed" | wc -l), protected paths touched: $protected_hit"
}

case "${1:-}" in
  parse | validate | paths) "$1" ;;
  *)
    echo "authority-case-check.sh: unknown command '${1:-}'. Next action: call it with parse, validate or paths." >&2
    exit 2
    ;;
esac
