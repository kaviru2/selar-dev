#!/usr/bin/env bash
# backup-db.sh: encrypted pg_dump of production Postgres to Google Cloud Storage.
#
#   scripts/deploy/backup-db.sh [--dry-run] [--local-only DIR]
#
# Dumps DATABASE_URL_DIRECT (never the -pooler URL) with `pg_dump -Fc`,
# encrypts the stream with age to BACKUP_AGE_RECIPIENT (a public key; the
# matching identity is kept offline by the operator), and uploads it to
# gs://$BACKUP_BUCKET/neon/selar-<UTC time>-<sha>.dump.age. Plaintext never
# touches disk: pg_dump | age > encrypted temp file (0600) > gcloud storage cp.
# --local-only DIR writes the encrypted file to DIR instead of uploading.
#
# Runs nightly from .github/workflows/db-backup.yml (Workload Identity
# Federation, service account with roles/storage.objectCreator only) and works
# the same from an operator machine (gcloud logged in). Free-tier guard: the
# run fails if the encrypted dump exceeds BACKUP_MAX_BYTES (default 150 MB;
# 30 days x 150 MB stays under the 5 GB GCS free tier).
set -euo pipefail
# shellcheck source=scripts/deploy/common.sh
. "$(dirname "$0")/common.sh"

BACKUP_BUCKET="${BACKUP_BUCKET:-selar-db-backups-261008}"
BACKUP_PREFIX="${BACKUP_PREFIX:-neon}"
BACKUP_MAX_BYTES="${BACKUP_MAX_BYTES:-157286400}"
LOCAL_ONLY=""

while [ $# -gt 0 ]; do
  parse_common_flag "$1" && { shift; continue; }
  case "$1" in
    --local-only) LOCAL_ONLY="${2:?--local-only needs a directory}"; shift ;;
    -h|--help) sed -n '2,18p' "$0" | sed 's/^# \{0,1\}//'; exit 0 ;;
    *) die "unknown argument: $1" ;;
  esac
  shift
done

load_secrets
require_vars DATABASE_URL_DIRECT BACKUP_AGE_RECIPIENT
require_cmds pg_dump age python3
[ -n "$LOCAL_ONLY" ] || require_cmds gcloud
if is_pooled_db_url "$DATABASE_URL_DIRECT"; then
  die "DATABASE_URL_DIRECT is a pooled (-pooler) URL; pg_dump needs the direct endpoint"
fi
case "$BACKUP_AGE_RECIPIENT" in
  age1*) ;;
  *) die "BACKUP_AGE_RECIPIENT must be an age public key (age1...); never put the identity in CI" ;;
esac

STAMP="$(date -u +%Y%m%dT%H%M%SZ)"
SHA="$(git -C "$REPO_ROOT" rev-parse --short HEAD 2>/dev/null || echo nogit)"
NAME="selar-$STAMP-$SHA.dump.age"
DEST="gs://$BACKUP_BUCKET/$BACKUP_PREFIX/$NAME"
[ -n "$LOCAL_ONLY" ] && DEST="$LOCAL_ONLY/$NAME"

log "backup $(url_host "$DATABASE_URL_DIRECT") -> $DEST"
info "pg_dump $(pg_dump --version | awk '{print $3}'), custom format, encrypted with age (recipient ${BACKUP_AGE_RECIPIENT:0:12}…)"
if [ "$DRY_RUN" = "1" ]; then
  printf '[dry-run] pg_dump -Fc | age -r <recipient> > tmp && gcloud storage cp tmp %s\n' "$DEST"
  exit 0
fi

TMP="$(mktemp -d "${TMPDIR:-/tmp}/selar-backup.XXXXXX")"
chmod 700 "$TMP"
trap 'rm -rf "$TMP"' EXIT
OUT="$TMP/$NAME"

# libpq settings come from the URL through the environment (never argv).
PGDUMP_PY='
import os, sys, urllib.parse
u = urllib.parse.urlsplit(os.environ.pop("BACKUP_DB_URL"))
env = dict(os.environ)
parts = {"PGHOST": u.hostname or "", "PGPORT": str(u.port or 5432),
         "PGUSER": urllib.parse.unquote(u.username or ""),
         "PGPASSWORD": urllib.parse.unquote(u.password or ""),
         "PGDATABASE": u.path.lstrip("/")}
env.update({k: v for k, v in parts.items() if v})
q = dict(urllib.parse.parse_qsl(u.query))
for key, var in (("sslmode", "PGSSLMODE"), ("channel_binding", "PGCHANNELBINDING"), ("sslrootcert", "PGSSLROOTCERT")):
    if key in q:
        env[var] = q[key]
env.setdefault("PGSSLMODE", "require")
env["PGCONNECT_TIMEOUT"] = "20"
os.execvpe("pg_dump", ["pg_dump", "--format=custom", "--compress=6", "--no-password"] + sys.argv[1:], env)
'
( umask 077
  BACKUP_DB_URL="$DATABASE_URL_DIRECT" python3 -c "$PGDUMP_PY" \
    | age --encrypt --recipient "$BACKUP_AGE_RECIPIENT" --output "$OUT" )

BYTES="$(wc -c < "$OUT" | tr -d ' ')"
[ "$BYTES" -gt 1024 ] || die "encrypted dump is only $BYTES bytes; refusing to upload"
[ "$BYTES" -le "$BACKUP_MAX_BYTES" ] || die "encrypted dump is $BYTES bytes > BACKUP_MAX_BYTES ($BACKUP_MAX_BYTES); free-tier guard"
SUM="$(shasum -a 256 "$OUT" 2>/dev/null || sha256sum "$OUT")"
ok "encrypted dump: $BYTES bytes, sha256 ${SUM%% *}"

if [ -n "$LOCAL_ONLY" ]; then
  mkdir -p "$LOCAL_ONLY"
  mv "$OUT" "$DEST"
  ok "written to $DEST"
  exit 0
fi

# objectCreator cannot overwrite; names are unique per second + sha.
gcloud storage cp --no-clobber --content-type=application/octet-stream \
  --custom-metadata="sha256=${SUM%% *},git=$SHA,format=pg_dump-custom+age" \
  "$OUT" "$DEST" >/dev/null
ok "uploaded $DEST"
echo "BACKUP_OBJECT=$DEST"
echo "BACKUP_BYTES=$BYTES"
