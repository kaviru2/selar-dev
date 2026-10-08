#!/usr/bin/env bash
# deploy-all.sh: one-command production release of the whole SELAR stack.
#
#   scripts/deploy/deploy-all.sh [--dry-run] [--yes] [--skip-tests]
#                                [--sync-secrets] [--only=LIST] [--auth-smoke]
#
# Order (each step stops the release on failure):
#   1. preflight: CLIs, Vercel/Modal logins, secrets file, env var names
#   2. git: clean main, equal to origin/main (plus a CI status check)
#   3. quick tests (go vet+test, worker pytest, console vitest) unless --skip-tests
#   4. migrate     (status -> apply -> status, direct URL)
#   5. worker      (Modal; secret untouched unless --sync-secrets)
#   6. api         (Vercel production)
#   7. console     (Vercel production)
#   8. smoke       (read-only; --auth-smoke adds the throwaway-account flow)
#
# --only=LIST limits steps 4-7, e.g. --only=api,console (migrate is still
# checked read-only first so an API never ships ahead of its schema).
# --dry-run runs every read-only check and prints every deploy command.
set -euo pipefail
# shellcheck source=scripts/deploy/common.sh
. "$(dirname "$0")/common.sh"

SKIP_TESTS=0
SYNC=""
AUTH_SMOKE=""
ONLY="migrate,worker,api,console"
PASS_FLAGS=""
while [ $# -gt 0 ]; do
  if parse_common_flag "$1"; then
    PASS_FLAGS="$PASS_FLAGS $1"; shift; continue
  fi
  case "$1" in
    --skip-tests) SKIP_TESTS=1 ;;
    --sync-secrets) SYNC="--sync-secrets" ;;
    --auth-smoke) AUTH_SMOKE="--auth-flow" ;;
    --only=*) ONLY="${1#--only=}" ;;
    -h|--help) sed -n '2,21p' "$0" | sed 's/^# \{0,1\}//'; exit 0 ;;
    *) die "unknown argument: $1" ;;
  esac
  shift
done
case ",$ONLY," in *,migrate,*|*,worker,*|*,api,*|*,console,*) ;; *) die "--only needs one of migrate,worker,api,console" ;; esac
wants() { case ",$ONLY," in *",$1,"*) return 0 ;; *) return 1 ;; esac; }

START=$(date +%s)
step() { printf '\n%s######## %s%s\n' "$C_BLU" "$*" "$C_OFF"; }

if [ "$DRY_RUN" = "1" ]; then
  log "DRY RUN: read-only checks run for real; nothing is deployed or changed"
fi

step "1/8 preflight"
need="git curl python3"
wants migrate && need="$need go"
wants worker && need="$need modal"
{ wants api || wants console; } && need="$need vercel"
# shellcheck disable=SC2086
require_cmds $need
ok "CLIs present: $need"
load_secrets
modal_token_policy
{ wants api || wants console; } && check_vercel_login
wants worker && check_modal_login
if [ "${SELAR_SKIP_ENV_SYNC:-0}" = "1" ]; then
  warn "env name check skipped (SELAR_SKIP_ENV_SYNC=1)"
else
  # Names-only, read-only comparison; the Modal probe is slow, so it runs only
  # when the worker is part of this release.
  # Read-only, so it runs for real even in --dry-run.
  if wants worker; then DRY_RUN=0 "$DEPLOY_DIR/env-sync.sh"; else DRY_RUN=0 "$DEPLOY_DIR/env-sync.sh" --skip-modal; fi
fi

step "2/8 git state"
check_git_release_state
check_ci_status

step "3/8 tests"
if [ "$SKIP_TESTS" = "1" ]; then
  warn "tests skipped (--skip-tests)"
else
  setup_go_cache
  # Tests must never reach a real database: drop every DB URL from their env
  # (DB-backed tests then skip; CI runs them against a throwaway Postgres).
  TEST_ENV="env -u DATABASE_URL -u TEST_DATABASE_URL -u MIGRATION_DATABASE_URL -u DATABASE_URL_DIRECT -u DATABASE_URL_POOLED"
  log "go vet + go test (API, short)"
  # shellcheck disable=SC2086
  (cd "$REPO_ROOT/services/selar-api" && $TEST_ENV go vet ./... && $TEST_ENV go test -short -count=1 ./...)
  if command -v pnpm >/dev/null 2>&1 && [ -d "$REPO_ROOT/services/selar-console/node_modules" ]; then
    log "console vitest"
    # shellcheck disable=SC2086
    (cd "$REPO_ROOT/services/selar-console" && $TEST_ENV pnpm -s test)
  else
    warn "console tests skipped (run pnpm install in services/selar-console first)"
  fi
  if python3 -c 'import pytest, fastapi, asyncpg' >/dev/null 2>&1; then
    log "worker pytest (no database)"
    # shellcheck disable=SC2086
    (cd "$REPO_ROOT/services/selar-worker" && $TEST_ENV python3 -m pytest -q -x)
  else
    warn "worker tests skipped (pip install -r services/selar-worker/requirements.txt pytest)"
  fi
  ok "tests passed"
fi

# One confirmation for the whole release; the step scripts then run unattended.
if [ "$DRY_RUN" != "1" ] && [ "${SELAR_YES:-0}" != "1" ]; then
  confirm_production "steps [$ONLY]"
  SELAR_YES=1; export SELAR_YES; PASS_FLAGS="$PASS_FLAGS --yes"
fi

step "4/8 migrate"
# shellcheck disable=SC2086  # PASS_FLAGS is a word list of simple flags
if wants migrate; then
  "$DEPLOY_DIR/migrate.sh" $PASS_FLAGS
else
  "$DEPLOY_DIR/migrate.sh" --status-only $PASS_FLAGS
fi

step "5/8 worker"
# shellcheck disable=SC2086
if wants worker; then "$DEPLOY_DIR/deploy-worker.sh" $PASS_FLAGS $SYNC; else info "skipped (--only)"; fi

step "6/8 api"
# shellcheck disable=SC2086
if wants api; then "$DEPLOY_DIR/deploy-api.sh" $PASS_FLAGS; else info "skipped (--only)"; fi

step "7/8 console"
# shellcheck disable=SC2086
if wants console; then "$DEPLOY_DIR/deploy-console.sh" $PASS_FLAGS; else info "skipped (--only)"; fi

step "8/8 smoke"
if [ "$DRY_RUN" = "1" ]; then
  # The read-only checks run for real against current production; the
  # throwaway-account flow is only printed.
  DRY_RUN=0 "$DEPLOY_DIR/smoke.sh"
  [ -z "$AUTH_SMOKE" ] || "$DEPLOY_DIR/smoke.sh" --dry-run --auth-flow | grep 'dry-run'
else
  # shellcheck disable=SC2086
  "$DEPLOY_DIR/smoke.sh" $AUTH_SMOKE
fi

printf '\n'
if [ "$DRY_RUN" = "1" ]; then
  ok "dry run complete in $(( $(date +%s) - START ))s; nothing was deployed"
else
  ok "release $(git_short_sha) complete in $(( $(date +%s) - START ))s"
  info "rollback notes: scripts/deploy/ROLLBACK.md"
fi
