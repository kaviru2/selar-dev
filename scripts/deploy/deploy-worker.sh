#!/usr/bin/env bash
# deploy-worker.sh: deploy the Python worker (services/selar-worker) to Modal.
#
#   scripts/deploy/deploy-worker.sh [--dry-run] [--yes] [--sync-secrets]
#                                   [--allow-key-removal]
#
# By default the Modal secret 'selar-worker-secrets' is left untouched.
# --sync-secrets overwrites it from the local secrets file first
# (DATABASE_URL <- DATABASE_URL_DIRECT, plus the worker keys listed in
# common.sh). Values go through a 0600 temp file that is deleted afterwards;
# only key names are printed. The sync replaces the whole secret, so it refuses
# when a key that exists in Modal is missing locally, unless
# --allow-key-removal. `modal deploy` takes about 5 minutes.
set -euo pipefail
# shellcheck source=scripts/deploy/common.sh
. "$(dirname "$0")/common.sh"

SYNC_SECRETS=0
ALLOW_KEY_REMOVAL=0
while [ $# -gt 0 ]; do
  parse_common_flag "$1" && { shift; continue; }
  case "$1" in
    --sync-secrets) SYNC_SECRETS=1 ;;
    --allow-key-removal) ALLOW_KEY_REMOVAL=1 ;;
    -h|--help) sed -n '2,14p' "$0" | sed 's/^# \{0,1\}//'; exit 0 ;;
    *) die "unknown argument: $1" ;;
  esac
  shift
done

log "worker: preflight"
require_cmds modal git curl
load_secrets
modal_token_policy
check_modal_login
modal secret list --json 2>/dev/null | grep -q "\"name\": \"$MODAL_SECRET\"" \
  || die "Modal secret $MODAL_SECRET does not exist (create it, or run with --sync-secrets)"
ok "Modal secret $MODAL_SECRET exists"
WORKER_HEALTH="${SELAR_WORKER_URL:-${WORKER_URL:-}}"
[ -n "$WORKER_HEALTH" ] || warn "WORKER_URL not set; post-deploy health check will be skipped"

CLEANUP_PATHS=""
cleanup() {
  local p
  for p in $CLEANUP_PATHS; do rm -rf "$p"; done
}
trap cleanup EXIT

sync_secrets() {
  local tmp name names="DATABASE_URL"
  require_vars DATABASE_URL_DIRECT
  is_pooled_db_url "$DATABASE_URL_DIRECT" && die "DATABASE_URL_DIRECT is a pooled URL; the worker needs the direct one"
  for name in $REQUIRED_WORKER_ENV; do
    [ "$name" = "DATABASE_URL" ] && continue
    require_vars "$name"
    names="$names $name"
  done
  for name in $OPTIONAL_WORKER_ENV; do
    [ -n "${!name:-}" ] && names="$names $name"
  done
  log "sync Modal secret $MODAL_SECRET (keys: $names)"
  local current dropped=""
  current="$(modal_secret_key_names)" || die "could not read the current key names of $MODAL_SECRET"
  for name in $current; do
    case " $names " in *" $name "*) ;; *) dropped="$dropped $name" ;; esac
  done
  if [ -n "$dropped" ]; then
    if [ "$ALLOW_KEY_REMOVAL" = "1" ]; then
      warn "sync will REMOVE these keys from $MODAL_SECRET:$dropped"
    elif [ "$DRY_RUN" = "1" ]; then
      warn "sync would remove:$dropped (a real run refuses without --allow-key-removal; add them to $(secrets_file) instead)"
    else
      die "sync would remove keys that exist in Modal:$dropped. Add them to $(secrets_file) or pass --allow-key-removal"
    fi
  else
    ok "no existing keys would be removed"
  fi
  if [ "$DRY_RUN" = "1" ]; then
    printf '%s[dry-run]%s modal secret create %s --from-dotenv <0600 temp file> --force\n' "$C_DIM" "$C_OFF" "$MODAL_SECRET"
    return 0
  fi
  confirm_production "Modal secret overwrite ($MODAL_SECRET)"
  tmp="$(mktemp "${TMPDIR:-/tmp}/selar-worker-secret.XXXXXX")"
  CLEANUP_PATHS="$CLEANUP_PATHS $tmp"
  chmod 600 "$tmp"
  {
    printf 'DATABASE_URL=%s\n' "$DATABASE_URL_DIRECT"
    for name in $names; do
      [ "$name" = "DATABASE_URL" ] && continue
      printf '%s=%s\n' "$name" "${!name}"
    done
  } > "$tmp"
  modal secret create "$MODAL_SECRET" --from-dotenv "$tmp" --force >/dev/null \
    || die "modal secret create failed"
  rm -f "$tmp"
  ok "Modal secret updated (values hidden)"
}

if [ "$SYNC_SECRETS" = "1" ]; then
  sync_secrets
else
  info "Modal secret left unchanged (pass --sync-secrets to overwrite it from $(secrets_file))"
fi

if [ "$DRY_RUN" = "1" ]; then
  printf '%s[dry-run]%s git archive HEAD services/selar-worker -> temp dir\n' "$C_DIM" "$C_OFF"
  printf '%s[dry-run]%s modal deploy modal_app.py --name %s --tag %s\n' "$C_DIM" "$C_OFF" "$MODAL_APP" "$(git_short_sha)"
  [ -n "$WORKER_HEALTH" ] && printf '%s[dry-run]%s wait for %s/health -> 200\n' "$C_DIM" "$C_OFF" "<WORKER_URL>"
  exit 0
fi

confirm_production "worker ($MODAL_APP)"
TREE="$(export_release_tree services/selar-worker)"
CLEANUP_PATHS="$CLEANUP_PATHS ${TREE%/services/selar-worker}"
log "worker: modal deploy ($(git_short_sha)); this takes a few minutes"
(cd "$TREE" && modal deploy modal_app.py --name "$MODAL_APP" --tag "$(git_short_sha)") \
  || die "modal deploy failed (the previous version keeps serving; see ROLLBACK.md)"

if [ -n "$WORKER_HEALTH" ]; then
  log "worker: health"
  wait_for_status "${WORKER_HEALTH%/}/health" 200 12 || die "worker /health failed; consider: modal app rollback $MODAL_APP"
  ok "worker healthy"
fi
