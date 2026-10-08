#!/usr/bin/env bash
# restore-db.sh: decrypt an encrypted backup and pg_restore it into a database.
#
#   scripts/deploy/restore-db.sh --list
#   scripts/deploy/restore-db.sh (--object gs://… | --latest | --file PATH) \
#       --identity ~/.hermes/secrets/selar-backup-age.key \
#       --target postgres://localhost:5432/selar_restore [--allow-remote] [--dry-run]
#
# --list      list backups in the bucket (name, size, time)
# --latest    newest object under gs://$BACKUP_BUCKET/$BACKUP_PREFIX/
# --identity  age identity file (private key, mode 600; never committed, never in CI)
# --target    database to restore INTO. It must exist and be empty. Only
#             localhost / 127.0.0.1 / a unix socket is accepted unless
#             --allow-remote (e.g. a fresh Neon branch). The script refuses
#             DATABASE_URL_DIRECT / DATABASE_URL_POOLED: never restore over
#             production in place; restore into a new Neon branch and switch.
#
# The encrypted object is downloaded into a private temp dir (0700) that is
# deleted on exit; plaintext only ever exists as a pipe into pg_restore.
set -euo pipefail
# shellcheck source=scripts/deploy/common.sh
. "$(dirname "$0")/common.sh"

BACKUP_BUCKET="${BACKUP_BUCKET:-selar-db-backups-261008}"
BACKUP_PREFIX="${BACKUP_PREFIX:-neon}"
OBJECT=""; FILE=""; IDENTITY=""; TARGET=""; LATEST=0; LIST=0; ALLOW_REMOTE=0

while [ $# -gt 0 ]; do
  parse_common_flag "$1" && { shift; continue; }
  case "$1" in
    --list) LIST=1 ;;
    --latest) LATEST=1 ;;
    --object) OBJECT="${2:?}"; shift ;;
    --file) FILE="${2:?}"; shift ;;
    --identity) IDENTITY="${2:?}"; shift ;;
    --target) TARGET="${2:?}"; shift ;;
    --allow-remote) ALLOW_REMOTE=1 ;;
    -h|--help) sed -n '2,20p' "$0" | sed 's/^# \{0,1\}//'; exit 0 ;;
    *) die "unknown argument: $1" ;;
  esac
  shift
done

if [ "$LIST" = "1" ]; then
  require_cmds gcloud
  gcloud storage ls -l "gs://$BACKUP_BUCKET/$BACKUP_PREFIX/" | sort -k2
  exit 0
fi

require_cmds age pg_restore psql python3
[ -n "$IDENTITY" ] && [ -r "$IDENTITY" ] || die "--identity FILE (age private key) is required and must be readable"
[ "$(file_mode "$IDENTITY")" = "600" ] || [ "$(file_mode "$IDENTITY")" = "400" ] || die "$IDENTITY must be mode 600 or 400"
[ -n "$TARGET" ] || die "--target URL is required"
n=0; [ -n "$OBJECT" ] && n=$((n+1)); [ -n "$FILE" ] && n=$((n+1)); [ "$LATEST" = "1" ] && n=$((n+1))
[ "$n" = "1" ] || die "choose exactly one of --object, --latest, --file"

# Never restore over production.
if [ "$(secrets_file)" = "env" ] || [ -r "$(secrets_file)" ]; then load_secrets; fi
for prod in "${DATABASE_URL_DIRECT:-}" "${DATABASE_URL_POOLED:-}" "${DATABASE_URL:-}"; do
  if [ -n "$prod" ] && [ "$(url_host "$TARGET")" = "$(url_host "$prod")" ]; then
    die "target host is the production database; restore into a new Neon branch instead"
  fi
done
host="$(url_host "$TARGET")"
case "$host" in
  localhost|127.0.0.1|::1|"") ;;
  *) [ "$ALLOW_REMOTE" = "1" ] || die "target host $host is not local; pass --allow-remote for a fresh Neon branch" ;;
esac

if [ "$LATEST" = "1" ]; then
  require_cmds gcloud
  OBJECT="$(gcloud storage ls "gs://$BACKUP_BUCKET/$BACKUP_PREFIX/" | grep '\.dump\.age$' | sort | tail -1)"
  [ -n "$OBJECT" ] || die "no backups in gs://$BACKUP_BUCKET/$BACKUP_PREFIX/"
fi

log "restore ${OBJECT:-$FILE} -> ${host:-local socket}/$(python3 -c 'import sys,urllib.parse;print(urllib.parse.urlsplit(sys.argv[1]).path.lstrip("/"))' "$TARGET")"
if [ "$DRY_RUN" = "1" ]; then
  echo "[dry-run] age -d -i <identity> | pg_restore --no-owner --no-acl -d <target>"
  exit 0
fi

TMP="$(mktemp -d "${TMPDIR:-/tmp}/selar-restore.XXXXXX")"
chmod 700 "$TMP"
trap 'rm -rf "$TMP"' EXIT
if [ -n "$OBJECT" ]; then
  require_cmds gcloud
  ( umask 077; gcloud storage cp "$OBJECT" "$TMP/backup.dump.age" >/dev/null )
  FILE="$TMP/backup.dump.age"
fi
[ -r "$FILE" ] || die "cannot read $FILE"
ok "encrypted backup: $(wc -c < "$FILE" | tr -d ' ') bytes"

existing="$(psql "$TARGET" -X -At -v ON_ERROR_STOP=1 -c "select count(*) from pg_tables where schemaname='public'")" \
  || die "cannot connect to target"
[ "$existing" = "0" ] || die "target already has $existing public tables; restore into an empty database"

# Extensions first (pg_restore of CREATE EXTENSION needs them available).
psql "$TARGET" -X -q -v ON_ERROR_STOP=1 -c 'CREATE EXTENSION IF NOT EXISTS vector; CREATE EXTENSION IF NOT EXISTS pgcrypto;' \
  || die "target needs pgvector and pgcrypto available"

age --decrypt --identity "$IDENTITY" "$FILE" \
  | pg_restore --no-owner --no-acl --exit-on-error --dbname "$TARGET" \
  || die "pg_restore failed"

ok "restored. Row counts:"
psql "$TARGET" -X -At -F ' ' -c "
  select 'schema_migrations', count(*) from schema_migrations
  union all select 'users', count(*) from users
  union all select 'documents', count(*) from documents
  union all select 'public tables', count(*) from pg_tables where schemaname='public'"
