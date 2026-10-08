#!/usr/bin/env bash
# env-sync.sh: compare the env var NAMES each runtime needs with what is set
# in Vercel (production) and in the Modal secret. Names only; no values are
# read or printed. Read-only: it never adds, changes or removes anything.
#
#   scripts/deploy/env-sync.sh [--dry-run] [--skip-modal]
#
# The Modal check runs a ~10 s ephemeral function (modal_secret_keys.py) that
# prints the key names the secret injects; --skip-modal skips it.
# Exit status: 0 when nothing required is missing, 1 otherwise.
set -euo pipefail
# shellcheck source=scripts/deploy/common.sh
. "$(dirname "$0")/common.sh"

SKIP_MODAL=0
while [ $# -gt 0 ]; do
  parse_common_flag "$1" && { shift; continue; }
  case "$1" in
    --skip-modal) SKIP_MODAL=1 ;;
    -h|--help) sed -n '2,10p' "$0" | sed 's/^# \{0,1\}//'; exit 0 ;;
    *) die "unknown argument: $1" ;;
  esac
  shift
done

MISSING=0

# word_in WORD "LIST": true if WORD is one of the space-separated LIST.
word_in() {
  case " $2 " in
    *" $1 "*) return 0 ;;
  esac
  return 1
}

# compare LABEL "REQUIRED NAMES" "ACTUAL NAMES (newline separated)"
compare() {
  local label="$1" required="$2" actual="$3" name extra
  local miss=""
  for name in $required; do
    printf '%s\n' "$actual" | grep -qx "$name" || miss="$miss $name"
  done
  extra=""
  for name in $actual; do
    word_in "$name" "$required" || extra="$extra $name"
  done
  if [ -z "$miss" ]; then
    ok "$label: all $(echo "$required" | wc -w | tr -d ' ') required names set"
  else
    err "$label: missing:$miss"
    MISSING=1
  fi
  [ -z "$extra" ] || info "$label: also set (optional/unused):$extra"
}

vercel_names() { # vercel_names PROJECT
  VERCEL_ORG_ID="$VERCEL_ORG" vercel env ls production --project "$1" --scope "$VERCEL_SCOPE" --format json 2>/dev/null \
    | python3 -c '
import json, sys
data = json.load(sys.stdin)
envs = data.get("envs", data) if isinstance(data, dict) else data
print("\n".join(sorted({e["key"] for e in envs if "production" in (e.get("target") or [])})))'
}

if [ "$DRY_RUN" = "1" ]; then
  printf '%s[dry-run]%s vercel env ls production --project %s (names only)\n' "$C_DIM" "$C_OFF" "$VERCEL_API_PROJECT"
  printf '%s[dry-run]%s vercel env ls production --project %s (names only)\n' "$C_DIM" "$C_OFF" "$VERCEL_CONSOLE_PROJECT"
  [ "$SKIP_MODAL" = "1" ] || printf '%s[dry-run]%s modal run modal_secret_keys.py (key names of %s)\n' "$C_DIM" "$C_OFF" "$MODAL_SECRET"
  printf '%s[dry-run]%s compare with the local secrets file (names only)\n' "$C_DIM" "$C_OFF"
  exit 0
fi

require_cmds vercel python3
check_vercel_login

log "Vercel $VERCEL_API_PROJECT (production)"
compare "$VERCEL_API_PROJECT" "$REQUIRED_API_ENV" "$(vercel_names "$VERCEL_API_PROJECT")"
log "Vercel $VERCEL_CONSOLE_PROJECT (production)"
compare "$VERCEL_CONSOLE_PROJECT" "$REQUIRED_CONSOLE_ENV" "$(vercel_names "$VERCEL_CONSOLE_PROJECT")"

if [ "$SKIP_MODAL" = "0" ]; then
  require_cmds modal
  load_secrets >/dev/null
  modal_token_policy
  log "Modal secret $MODAL_SECRET"
  names="$(modal_secret_key_names)" || die "modal probe failed"
  [ -n "$names" ] || warn "probe returned no key names"
  compare "$MODAL_SECRET" "$REQUIRED_WORKER_ENV" "$names"
fi

log "local secrets file $(secrets_file)"
local_names="$(secret_names)"
need_local="DATABASE_URL_DIRECT DATABASE_URL_POOLED JWT_SECRET WORKER_TRIGGER_SECRET WORKER_URL GEMINI_API_KEY STORAGE_BACKEND S3_ENDPOINT S3_BUCKET S3_REGION S3_ACCESS_KEY_ID S3_SECRET_ACCESS_KEY"
miss=""
for name in $need_local; do
  printf '%s\n' "$local_names" | grep -qx "$name" || miss="$miss $name"
done
if [ -z "$miss" ]; then
  ok "local file has every name needed for --sync-secrets and migrations"
else
  warn "local file lacks (only matters for --sync-secrets/migrate):$miss"
fi

[ "$MISSING" = "0" ] || die "required env var names are missing (see above). Add them in Vercel/Modal; this script never writes."
ok "env names in sync"
