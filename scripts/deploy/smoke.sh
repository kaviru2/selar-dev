#!/usr/bin/env bash
# smoke.sh: post-deploy checks against production. Read-only by default.
#
#   scripts/deploy/smoke.sh [--dry-run] [--auth-flow]
#
# Checks: API /healthz 200, API /api/users/me without a token 401, API CORS
# preflight allows the console origin, worker /health 200, worker
# POST /jobs/trigger without the secret 401, console /login 200, / is the
# landing page (or /login), a deep link redirects signed-out visitors to /login, a cookie on /login redirects into
# the app, an off-site from= is not followed, console /api proxy without a
# cookie is not 200, bucket CORS allows the console origin (if S3_* is known).
#
# --auth-flow also registers a throwaway account with invented data
# (deploy-smoke-<time>-<random>@example.invalid), checks the Secure/HttpOnly
# session cookie, /api/users/me and /api/documents through the console, logs
# out, and finally deletes the account (needs psql + DATABASE_URL_DIRECT;
# all rows are removed by ON DELETE CASCADE). The account is deleted even when
# a check fails.
set -uo pipefail
# shellcheck source=scripts/deploy/common.sh
. "$(dirname "$0")/common.sh"

AUTH_FLOW=0
while [ $# -gt 0 ]; do
  parse_common_flag "$1" && { shift; continue; }
  case "$1" in
    --auth-flow) AUTH_FLOW=1 ;;
    -h|--help) sed -n '2,19p' "$0" | sed 's/^# \{0,1\}//'; exit 0 ;;
    *) die "unknown argument: $1" ;;
  esac
  shift
done

require_cmds curl
if [ -r "$(secrets_file)" ] || [ "$(secrets_file)" = "env" ]; then
  load_secrets
else
  warn "secrets file not found; worker and bucket checks need SELAR_WORKER_URL / S3_* in the environment"
fi
WORKER_BASE="${SELAR_WORKER_URL:-${WORKER_URL:-}}"
WORKER_BASE="${WORKER_BASE%/}"

PASS=0
FAIL=0
SKIP=0
RESULTS=""
record() { # record STATUS NAME DETAIL
  RESULTS="$RESULTS$1|$2|$3
"
  case "$1" in
    PASS) PASS=$((PASS + 1)); ok "$2 ($3)" ;;
    FAIL) FAIL=$((FAIL + 1)); err "$2 ($3)" ;;
    SKIP) SKIP=$((SKIP + 1)); warn "skip: $2 ($3)" ;;
  esac
}

# expect NAME EXPECTED_CODE URL [curl args...]
expect() {
  local name="$1" expected="$2" url="$3" code; shift 3
  if [ "$DRY_RUN" = "1" ]; then
    printf '%s[dry-run]%s %s: %s -> %s\n' "$C_DIM" "$C_OFF" "$name" "$url" "$expected"
    return 0
  fi
  code="$(http_status "$url" "$@")"
  if [ "$code" = "$expected" ]; then record PASS "$name" "$code"; else record FAIL "$name" "got $code, want $expected"; fi
}

# expect_redirect NAME URL LOCATION_PREFIX [curl args...]
expect_redirect() {
  local name="$1" url="$2" want="$3" out code loc; shift 3
  if [ "$DRY_RUN" = "1" ]; then
    printf '%s[dry-run]%s %s: %s -> 3xx %s*\n' "$C_DIM" "$C_OFF" "$name" "$url" "$want"
    return 0
  fi
  out="$(curl -sS -o /dev/null -w '%{http_code} %{redirect_url}' --max-time 20 "$@" "$url" 2>/dev/null)" || true
  code="${out%% *}"; loc="${out#* }"
  case "$code" in
    30[12378])
      case "$loc" in
        "$want"*) record PASS "$name" "$code -> ${loc#"$CONSOLE_URL"}" ;;
        *) record FAIL "$name" "$code -> $loc, want $want*" ;;
      esac ;;
    *) record FAIL "$name" "got $code, want a redirect to $want" ;;
  esac
}

# expect_header NAME URL HEADER_REGEX [curl args...]
expect_header() {
  local name="$1" url="$2" re="$3"; shift 3
  if [ "$DRY_RUN" = "1" ]; then
    printf '%s[dry-run]%s %s: %s has header /%s/\n' "$C_DIM" "$C_OFF" "$name" "$url" "$re"
    return 0
  fi
  if curl -sS -D - -o /dev/null --max-time 20 "$@" "$url" 2>/dev/null | tr -d '\r' | grep -Eiq "$re"; then
    record PASS "$name" "header present"
  else
    record FAIL "$name" "header /$re/ missing"
  fi
}

log "smoke: API $API_URL"
expect "api healthz" 200 "$API_URL/healthz"
expect "api /api/users/me without token" 401 "$API_URL/api/users/me"
expect_header "api CORS allows console origin" "$API_URL/api/users/me" \
  "^access-control-allow-origin: $CONSOLE_URL\$" \
  -X OPTIONS -H "Origin: $CONSOLE_URL" -H "Access-Control-Request-Method: GET"

log "smoke: worker"
if [ -n "$WORKER_BASE" ]; then
  expect "worker health" 200 "$WORKER_BASE/health"
  expect "worker trigger without secret" 401 "$WORKER_BASE/jobs/trigger" -X POST -H 'Content-Type: application/json' -d '{}'
else
  record SKIP "worker checks" "WORKER_URL unknown"
fi

log "smoke: console $CONSOLE_URL"
expect "console /login" 200 "$CONSOLE_URL/login"
expect "console /register" 200 "$CONSOLE_URL/register"
# / is either the public landing page (200) or a redirect to /login.
if [ "$DRY_RUN" = "1" ]; then
  printf '%s[dry-run]%s console / signed out: 200 landing or redirect to /login\n' "$C_DIM" "$C_OFF"
else
  out="$(curl -sS -o /dev/null -w '%{http_code} %{redirect_url}' --max-time 20 "$CONSOLE_URL/" 2>/dev/null)" || true
  case "$out" in
    "200 "*) record PASS "console / signed out" "200 landing page" ;;
    30[12378]" $CONSOLE_URL/login"*) record PASS "console / signed out" "${out%% *} -> /login" ;;
    *) record FAIL "console / signed out" "got $out" ;;
  esac
fi
expect_redirect "console / with cookie -> app" "$CONSOLE_URL/" "$CONSOLE_URL/library" -H 'Cookie: selar_token=smoke-placeholder'
expect_redirect "console deep link -> /login?from=" "$CONSOLE_URL/library" "$CONSOLE_URL/login?from=%2Flibrary"
expect_redirect "console /login with cookie -> app" "$CONSOLE_URL/login" "$CONSOLE_URL/library" -H 'Cookie: selar_token=smoke-placeholder'
expect_redirect "console off-site from= ignored" "$CONSOLE_URL/login?from=https%3A%2F%2Fevil.example" "$CONSOLE_URL/library" -H 'Cookie: selar_token=smoke-placeholder'
expect_redirect "console session-expired -> /login" "$CONSOLE_URL/api/auth/session-expired" "$CONSOLE_URL/login"
if [ "$DRY_RUN" = "1" ]; then
  printf '%s[dry-run]%s console /api/documents without cookie is not 200\n' "$C_DIM" "$C_OFF"
else
  code="$(http_status "$CONSOLE_URL/api/documents")"
  case "$code" in 200|000|5*) record FAIL "console /api proxy without cookie" "got $code" ;;
    *) record PASS "console /api proxy without cookie" "$code" ;; esac
fi

log "smoke: bucket CORS"
if [ -n "${S3_ENDPOINT:-}" ] && [ -n "${S3_BUCKET:-}" ]; then
  expect_header "bucket CORS allows console origin" "${S3_ENDPOINT%/}/$S3_BUCKET/deploy-smoke-cors-probe" \
    "^access-control-allow-origin: $CONSOLE_URL\$" \
    -X OPTIONS -H "Origin: $CONSOLE_URL" -H "Access-Control-Request-Method: PUT"
else
  record SKIP "bucket CORS" "S3_ENDPOINT/S3_BUCKET unknown"
fi

# ---------- optional authenticated flow ----------
# psql_with_url ARGS...: psql against DATABASE_URL_DIRECT. The URL is split into
# PG* environment variables so neither it nor the password appears in argv.
# The helper is passed with -c so psql still reads SQL from the caller's stdin.
PSQL_EXEC_PY='
import os, sys, urllib.parse
u = urllib.parse.urlsplit(os.environ.pop("SMOKE_DB_URL"))
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
env["PGCONNECT_TIMEOUT"] = "15"
os.execvpe("psql", ["psql", "-X", "-q", "-v", "ON_ERROR_STOP=1", "-At"] + sys.argv[1:], env)
'
psql_with_url() {
  SMOKE_DB_URL="$DATABASE_URL_DIRECT" python3 -c "$PSQL_EXEC_PY" "$@"
}

SMOKE_EMAIL=""
SMOKE_TMP=""
SMOKE_REGISTERED=0
delete_smoke_account() {
  [ -n "$SMOKE_EMAIL" ] || return 0
  case "$SMOKE_EMAIL" in deploy-smoke-*@example.invalid) ;; *) err "refusing to delete unexpected account"; return 1 ;; esac
  local deleted
  deleted="$(psql_with_url -v email="$SMOKE_EMAIL" <<'SQL'
WITH d AS (DELETE FROM users WHERE email = :'email' AND email LIKE 'deploy-smoke-%@example.invalid' RETURNING 1)
SELECT count(*) FROM d;
SQL
)" || { record FAIL "auth: delete throwaway account" "psql failed; delete $SMOKE_EMAIL manually"; SMOKE_EMAIL=""; return 1; }
  if [ "$deleted" = "1" ]; then
    record PASS "auth: throwaway account deleted" "1 row + cascades"
  elif [ "$deleted" = "0" ] && [ "$SMOKE_REGISTERED" = "0" ]; then
    info "no throwaway account was created; nothing to delete"
  else
    record FAIL "auth: delete throwaway account" "deleted '$deleted' rows; check $SMOKE_EMAIL manually"
  fi
  SMOKE_EMAIL=""
}
cleanup() {
  delete_smoke_account
  [ -n "$SMOKE_TMP" ] && rm -rf "$SMOKE_TMP"
}
trap cleanup EXIT

auth_flow() {
  local jar body code pw headers
  log "smoke: authenticated flow with a throwaway account"
  if [ "$DRY_RUN" = "1" ]; then
    printf '%s[dry-run]%s register deploy-smoke-<ts>-<rand>@example.invalid via %s/api/auth/register\n' "$C_DIM" "$C_OFF" "$CONSOLE_URL"
    printf '%s[dry-run]%s check cookie flags, /api/users/me, /api/documents, logout\n' "$C_DIM" "$C_OFF"
    printf '%s[dry-run]%s DELETE FROM users WHERE email = <that address> (psql, DATABASE_URL_DIRECT)\n' "$C_DIM" "$C_OFF"
    return 0
  fi
  if ! command -v psql >/dev/null 2>&1 || ! command -v python3 >/dev/null 2>&1 || [ -z "${DATABASE_URL_DIRECT:-}" ]; then
    record SKIP "auth flow" "needs psql, python3 and DATABASE_URL_DIRECT to delete the account afterwards"
    return 0
  fi
  psql_with_url -c 'SELECT 1' >/dev/null 2>&1 || { record SKIP "auth flow" "database not reachable for cleanup"; return 0; }

  SMOKE_TMP="$(mktemp -d "${TMPDIR:-/tmp}/selar-smoke.XXXXXX")" || { record FAIL "auth flow" "cannot create a temp dir"; return 0; }
  chmod 700 "$SMOKE_TMP"
  jar="$SMOKE_TMP/cookies"; headers="$SMOKE_TMP/headers"
  SMOKE_EMAIL="deploy-smoke-$(date +%Y%m%d%H%M%S)-$(od -An -N4 -tx4 /dev/urandom | tr -d ' \n')@example.invalid"
  pw="$(od -An -N18 -tx1 /dev/urandom | tr -d ' \n')"
  body="$(printf '{"email":"%s","password":"%s"}' "$SMOKE_EMAIL" "$pw")"
  info "account: $SMOKE_EMAIL (invented; deleted at exit)"

  code="$(printf '%s' "$body" | curl -sS -o "$SMOKE_TMP/register.json" -D "$headers" -w '%{http_code}' --max-time 30 \
    -c "$jar" -H 'Content-Type: application/json' --data-binary @- "$CONSOLE_URL/api/auth/register" 2>/dev/null)" || true
  code="${code:-000}"
  # Even on a timeout the account may exist server-side; the exit trap deletes by address.
  SMOKE_REGISTERED=1
  if [ "$code" = "403" ] && grep -q '"recaptcha_failed"' "$SMOKE_TMP/register.json" 2>/dev/null; then
    # reCAPTCHA is on in production (docs/DEPLOYMENT.md §6); curl cannot get a token.
    SMOKE_REGISTERED=0  # refused before any insert; nothing to delete
    record PASS "auth: register refuses a request without a reCAPTCHA token" "$code"
    record SKIP "auth flow (rest)" "reCAPTCHA enabled; sign up once in a browser to test the full flow"
    return 0
  fi
  if [ "$code" != "201" ]; then
    record FAIL "auth: register" "got $code"
    return 0
  fi
  record PASS "auth: register" "$code"
  if tr -d '\r' < "$headers" | grep -i '^set-cookie: selar_token=' | grep -i 'httponly' | grep -qi 'secure'; then
    record PASS "auth: session cookie Secure+HttpOnly" "ok"
  else
    record FAIL "auth: session cookie Secure+HttpOnly" "flags missing"
  fi
  code="$(curl -sS -o "$SMOKE_TMP/me.json" -w '%{http_code}' --max-time 30 -b "$jar" "$CONSOLE_URL/api/users/me" 2>/dev/null)" || true
  if [ "$code" = "200" ] && grep -q "$SMOKE_EMAIL" "$SMOKE_TMP/me.json"; then
    record PASS "auth: /api/users/me via console proxy" "$code"
  else
    record FAIL "auth: /api/users/me via console proxy" "got $code"
  fi
  expect "auth: /api/documents" 200 "$CONSOLE_URL/api/documents" -b "$jar"
  expect "auth: /library renders" 200 "$CONSOLE_URL/library" -b "$jar"
  code="$(printf '%s' "$body" | curl -sS -o /dev/null -w '%{http_code}' --max-time 30 \
    -H 'Content-Type: application/json' --data-binary @- "$API_URL/auth/login" 2>/dev/null)" || true
  if [ "$code" = "200" ]; then record PASS "auth: API login" "$code"; else record FAIL "auth: API login" "got $code"; fi
  expect "auth: logout" 200 "$CONSOLE_URL/api/auth/logout" -X POST -b "$jar" -c "$jar"
  delete_smoke_account
}

if [ "$AUTH_FLOW" = "1" ]; then
  auth_flow
fi

if [ "$DRY_RUN" = "1" ]; then
  log "smoke: dry run, nothing requested"
  exit 0
fi
log "smoke summary: $PASS passed, $FAIL failed, $SKIP skipped"
[ "$FAIL" = "0" ]
