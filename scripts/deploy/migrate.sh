#!/usr/bin/env bash
# migrate.sh: apply SELAR's embedded SQL migrations to the production database.
#
#   scripts/deploy/migrate.sh [--dry-run] [--status-only] [--secrets=FILE]
#
# Steps: status (read-only) -> apply -> status (must report 0 pending).
# Uses DATABASE_URL_DIRECT from the secrets file and refuses a pooled
# (PgBouncer, "-pooler") URL: the runner holds a session advisory lock.
# --dry-run runs only the read-only status step.
set -euo pipefail
# shellcheck source=scripts/deploy/common.sh
. "$(dirname "$0")/common.sh"

usage() { sed -n '2,10p' "$0" | sed 's/^# \{0,1\}//'; }

STATUS_ONLY=0
while [ $# -gt 0 ]; do
  parse_common_flag "$1" && { shift; continue; }
  case "$1" in
    --status-only) STATUS_ONLY=1 ;;
    -h|--help) usage; exit 0 ;;
    *) die "unknown argument: $1" ;;
  esac
  shift
done

log "migrate: production database"
require_cmds go
load_secrets
require_vars DATABASE_URL_DIRECT
if is_pooled_db_url "$DATABASE_URL_DIRECT"; then
  die "DATABASE_URL_DIRECT points at a pooled endpoint ($(url_host "$DATABASE_URL_DIRECT")); migrations need the direct URL"
fi
info "target host: $(url_host "$DATABASE_URL_DIRECT") (direct)"
setup_go_cache

API_DIR="$REPO_ROOT/services/selar-api"
BIN_DIR="$(mktemp -d "${TMPDIR:-/tmp}/selar-migrate.XXXXXX")"
trap 'rm -rf "$BIN_DIR"' EXIT
log "build cmd/migrate (embeds the migrations in this checkout)"
(cd "$API_DIR" && go build -o "$BIN_DIR/migrate" ./cmd/migrate)
info "embedded: $(find "$API_DIR/internal/store/migrations" -name '[0-9]*.sql' | wc -l | tr -d ' ') migration files"

# The URL goes to the child through the environment only.
migrate_cmd() {
  MIGRATION_DATABASE_URL="$DATABASE_URL_DIRECT" DATABASE_URL="" "$BIN_DIR/migrate" "$@"
}

log "status (before)"
before="$(migrate_cmd -status 2>&1)" || { printf '%s\n' "$before" >&2; die "status failed"; }
printf '%s\n' "$before" | sed 's/^/    /'

if [ "$STATUS_ONLY" = "1" ]; then
  exit 0
fi
if printf '%s' "$before" | grep -q ', 0 pending'; then
  ok "nothing to apply"
  exit 0
fi
if [ "$DRY_RUN" = "1" ]; then
  printf '%s[dry-run]%s would apply the pending migrations above, then re-check status\n' "$C_DIM" "$C_OFF"
  exit 0
fi

confirm_production "database migrations"
log "apply"
migrate_cmd 2>&1 | sed 's/^/    /'

log "status (after)"
after="$(migrate_cmd -status 2>&1)"
printf '%s\n' "$after" | sed 's/^/    /'
printf '%s' "$after" | grep -q ', 0 pending' || die "migrations still pending after apply"
ok "database is up to date"
