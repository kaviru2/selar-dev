#!/usr/bin/env bash
# test-common.sh: offline unit tests for common.sh (no network, no secrets).
#
#   scripts/deploy/test-common.sh
#
# Covers secrets-file parsing (values never echoed), pooled-URL refusal, URL
# host extraction and the per-script --help/unknown-flag handling.
set -euo pipefail
DIR="$(cd "$(dirname "$0")" && pwd)"
# shellcheck source=scripts/deploy/common.sh
. "$DIR/common.sh"

FAILS=0
check() { # check NAME CONDITION...
  local name="$1"; shift
  if "$@"; then printf ' ok  %s\n' "$name"; else printf 'FAIL %s\n' "$name"; FAILS=$((FAILS + 1)); fi
}

TMP="$(mktemp -d "${TMPDIR:-/tmp}/selar-test-common.XXXXXX")"
trap 'rm -rf "$TMP"' EXIT
SENTINEL="sentinel-value-$$-must-not-appear"
cat > "$TMP/secrets.env" <<EOF
# comment
EMPTY_ONE=
export EXPORTED_KEY=exported-$SENTINEL
QUOTED_KEY="quoted $SENTINEL"
SINGLE_KEY='single-$SENTINEL'
PLAIN_KEY=plain=$SENTINEL
not a pair
1BAD=ignored
EOF
chmod 600 "$TMP/secrets.env"

out="$(
  unset EXPORTED_KEY QUOTED_KEY SINGLE_KEY PLAIN_KEY EMPTY_ONE
  PLAIN_KEY="pre-existing"
  SELAR_SECRETS="$TMP/secrets.env" load_secrets 2>&1
  printf 'E=%s|Q=%s|S=%s|P=%s|EMPTY=%s\n' "$EXPORTED_KEY" "$QUOTED_KEY" "$SINGLE_KEY" "$PLAIN_KEY" "${EMPTY_ONE-unset}"
)"
log_line="$(printf '%s\n' "$out" | grep 'secrets:')"
vals="$(printf '%s\n' "$out" | tail -1)"
check "load_secrets log line hides values" bash -c "! printf '%s' \"\$1\" | grep -q '$SENTINEL'" _ "$log_line"
check "load_secrets counts 4 values" bash -c "printf '%s' \"\$1\" | grep -q 'loaded 4 values'" _ "$log_line"
check "export prefix handled" [ "$(printf '%s' "$vals" | sed -E 's/.*E=([^|]*).*/\1/')" = "exported-$SENTINEL" ]
check "double quotes stripped" [ "$(printf '%s' "$vals" | sed -E 's/.*Q=([^|]*).*/\1/')" = "quoted $SENTINEL" ]
check "single quotes stripped" [ "$(printf '%s' "$vals" | sed -E 's/.*S=([^|]*).*/\1/')" = "single-$SENTINEL" ]
check "file overrides shell, keeps '=' in value" [ "$(printf '%s' "$vals" | sed -E 's/.*P=([^|]*).*/\1/')" = "plain=$SENTINEL" ]
check "empty values skipped" [ "$(printf '%s' "$vals" | sed -E 's/.*EMPTY=(.*)$/\1/')" = "unset" ]

check "file_mode reads 600" [ "$(file_mode "$TMP/secrets.env")" = "600" ]
check "file_owner_uid is the current user" [ "$(file_owner_uid "$TMP/secrets.env")" = "$(id -u)" ]
check "no mode warning for a 600 file" bash -c "! printf '%s' \"\$1\" | grep -q 'mode is'" _ "$out"

names="$(SELAR_SECRETS="$TMP/secrets.env" secret_names | tr '\n' ' ')"
check "secret_names lists names only" [ "$names" = "EXPORTED_KEY PLAIN_KEY QUOTED_KEY SINGLE_KEY " ]

check "url_host strips credentials" [ "$(url_host 'postgres://u:p%40ss@ep-a.us-east-1.aws.neon.tech:5432/db?sslmode=require')" = "ep-a.us-east-1.aws.neon.tech" ]
check "pooled URL detected" is_pooled_db_url 'postgres://u:p@ep-a-pooler.us-east-1.aws.neon.tech/db'
check "direct URL accepted" bash -c "! (. '$DIR/common.sh'; DATABASE_URL_POOLED=''; is_pooled_db_url 'postgres://u:p@ep-a.us-east-1.aws.neon.tech/db')"
check "URL equal to DATABASE_URL_POOLED detected" bash -c "DATABASE_URL_POOLED='postgres://x@h/db'; . '$DIR/common.sh'; DATABASE_URL_POOLED='postgres://x@h/db'; is_pooled_db_url 'postgres://x@h/db'"

printf 'DATABASE_URL_DIRECT=postgres://u:%s@ep-a-pooler.example.neon.tech/db?sslmode=require\n' "$SENTINEL" > "$TMP/pooled.env"
chmod 600 "$TMP/pooled.env"
if command -v go >/dev/null 2>&1; then
  mig="$("$DIR/migrate.sh" --dry-run --secrets="$TMP/pooled.env" 2>&1 || true)"
  check "migrate.sh refuses a pooled URL" bash -c "printf '%s' \"\$1\" | grep -q 'pooled endpoint'" _ "$mig"
  check "migrate.sh output hides the password" bash -c "! printf '%s' \"\$1\" | grep -q '$SENTINEL'" _ "$mig"
fi

for s in deploy-all deploy-api deploy-console deploy-worker migrate smoke env-sync; do
  check "$s.sh --help" bash -c "'$DIR/$s.sh' --help >/dev/null 2>&1"
  check "$s.sh rejects unknown flags" bash -c "! '$DIR/$s.sh' --definitely-not-a-flag >/dev/null 2>&1"
done

if [ "$FAILS" -ne 0 ]; then
  printf '%d test(s) failed\n' "$FAILS"
  exit 1
fi
printf 'all common.sh tests passed\n'
