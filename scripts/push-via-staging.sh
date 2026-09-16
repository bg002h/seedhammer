#!/usr/bin/env bash
# push-via-staging.sh -- the ci/staging push ritual as a command.
#
# WHY IT EXISTS. A required status check binds to a COMMIT SHA, not a branch, so
# a commit pushed straight at a protected branch carries no check when the rule
# is evaluated: GitHub reports the contexts as "expected" and the push is
# BYPASSED rather than satisfied:
#
#     remote: Bypassed rule violations for refs/heads/master:
#     remote: - 4 of 4 required status checks are expected.
#
# `strict: false` is what makes that fixable -- GitHub asks only whether the
# commit carries a passing context, not whether it is up to date. So let the SHA
# earn one on `ci/staging` first, then push the branch. This script IS the
# ritual; running it is the discipline.
#
#     scripts/push-via-staging.sh            # current branch
#     scripts/push-via-staging.sh master     # explicit
#
# IT DISCOVERS WHAT TO WAIT FOR, rather than being told. Two things went wrong
# on 2026-09-16 that a single hardcoded REQUIRED_CONTEXT cannot express:
#
#   * mnemonic-secret requires FOUR contexts. Waiting for one and pushing means
#     pushing while three are still running.
#   * the seedhammer fork's `main` is UNPROTECTED (the API returns 404). There
#     is no context to earn and no bypass is possible -- but the CI signal still
#     matters, and that repo fires THREE runs on a staging push, because
#     image.yml triggers on `push` AND on `create`. Waiting for "the" run waits
#     for whichever appeared first.
#
# So: protected -> wait for every required context. Unprotected -> wait for
# every workflow run on the SHA and require all of them green. Either way a
# FAILURE STOPS THE PUSH.
#
# FREEZE: no commits to the branch between invocation and completion. The tip is
# re-checked immediately before the final push and the run aborts if it moved --
# measured 2026-08-16, when two commits landed mid-window and reached the remote
# with no CI signal at all.
set -euo pipefail

BRANCH="${1:-$(git rev-parse --abbrev-ref HEAD)}"
TIP="$(git rev-parse HEAD)"
SLUG="$(git remote get-url origin | sed -E 's#(git@github.com:|https://github.com/)##; s#\.git$##')"
echo "== $SLUG @ $BRANCH -- staging $TIP ($(git rev-list --count "origin/$BRANCH..HEAD" 2>/dev/null || echo '?') ahead)"

if [ -n "$(git status --porcelain)" ]; then
  echo "FATAL: working tree is dirty. Empty your hands before a push window."; exit 1
fi

# `gh api` prints its 404 body to STDOUT, so `2>/dev/null` does not suppress it
# and a naive mapfile captures {"message":"Branch not protected"...} as one
# bogus "context" the script would then wait forever to see. The EXIT CODE is
# the honest signal: 0 protected, 1 not.
if PROT="$(gh api "repos/$SLUG/branches/$BRANCH/protection" 2>/dev/null)"; then
  mapfile -t CONTEXTS < <(printf '%s' "$PROT" | jq -r '.required_status_checks.contexts[]?')
else
  CONTEXTS=()
fi
if [ "${#CONTEXTS[@]}" -gt 0 ]; then
  echo "== required contexts (${#CONTEXTS[@]}): ${CONTEXTS[*]}"
else
  echo "== branch is NOT protected: no context to earn, so no bypass is possible."
  echo "==   The CI signal still gates this push -- every run on the SHA must be green."
fi

git push origin "HEAD:refs/heads/ci/staging"

# An empty `gh` result is a RACE, never a conclusion (measured: runs can take
# ~30 s to appear). Poll for existence before polling for completion.
for _ in $(seq 1 30); do
  [ "$(gh run list --repo "$SLUG" --commit "$TIP" --json databaseId -q 'length' 2>/dev/null || echo 0)" != "0" ] && break
  sleep 10
done
if [ "$(gh run list --repo "$SLUG" --commit "$TIP" --json databaseId -q 'length' 2>/dev/null || echo 0)" = "0" ]; then
  echo "FATAL: no workflow run appeared for $TIP after 5 minutes"; exit 1
fi

echo "== waiting for every run on $TIP to conclude"
for _ in $(seq 1 180); do
  PENDING="$(gh run list --repo "$SLUG" --commit "$TIP" --json status \
             -q '[.[]|select(.status!="completed")]|length' 2>/dev/null || echo 1)"
  [ "$PENDING" = "0" ] && break
  sleep 10
done
gh run list --repo "$SLUG" --commit "$TIP" --json name,conclusion \
  -q '.[]|"   \(.name) -> \(.conclusion)"'

# Job-level conclusions: a required CONTEXT is a job name, not a workflow name.
mapfile -t JOBS < <(
  gh run list --repo "$SLUG" --commit "$TIP" --json databaseId -q '.[].databaseId' \
  | while read -r id; do gh run view "$id" --repo "$SLUG" --json jobs \
      -q '.jobs[]|"\(.name)\t\(.conclusion // "pending")"'; done
)
fail=0
if [ "${#CONTEXTS[@]}" -gt 0 ]; then
  for ctx in "${CONTEXTS[@]}"; do
    got="$(printf '%s\n' "${JOBS[@]}" | awk -F'\t' -v c="$ctx" '$1==c{print $2; exit}')"
    echo "   context '$ctx' -> ${got:-MISSING}"
    [ "$got" = "success" ] || fail=1
  done
else
  for row in "${JOBS[@]}"; do
    name="${row%%$'\t'*}"; conc="${row##*$'\t'}"
    case "$conc" in
      success|skipped) ;;
      *) echo "   job '$name' -> $conc"; fail=1 ;;
    esac
  done
fi
if [ "$fail" != "0" ]; then
  echo "FATAL: CI is not green for $TIP -- NOT pushing $BRANCH."
  echo "       A red suite is a finding, not an obstacle: fix it and re-run."
  git push origin --delete ci/staging || true
  exit 1
fi

[ "$(git rev-parse HEAD)" = "$TIP" ] || {
  echo "FATAL: the tip moved during the window -- re-stage the new tip"; exit 1; }

OUT="$(git push origin "HEAD:$BRANCH" 2>&1)"; echo "$OUT"
if echo "$OUT" | grep -qi "bypassed rule violations"; then
  echo "FATAL: bypass message detected -- ci/staging left in place for forensics"; exit 1
fi
git push origin --delete ci/staging
git fetch -q origin
[ "$(git rev-parse "origin/$BRANCH")" = "$TIP" ] || {
  echo "FATAL: origin/$BRANCH is not $TIP after the push"; exit 1; }
echo "== OK: $TIP is on $BRANCH, CI green, no bypass"
