#!/usr/bin/env bash
# common.sh: shared helpers for the SELAR deploy scripts. Source it; do not run it.
#
# Rules enforced for every script:
#   - Secrets come from a KEY=value file (SELAR_SECRETS, default
#     ~/.hermes/secrets/selar-deploy.env). The file is parsed, never sourced or
#     evaluated, and no value is ever printed. SELAR_SECRETS=env uses variables
#     that are already exported (CI).
#   - Secrets reach CLIs through environment variables or 0600 temp files,
#     never through argv (visible in `ps`).
#   - umask 022 (Vercel's .vercel dir and build caches break under a restrictive
#     umask) and a private, user-owned GOCACHE.
#   - Every script supports --dry-run: print the plan and run read-only checks,
#     change nothing.
#
# Compatible with macOS /bin/bash 3.2 (no associative arrays, no mapfile).

# shellcheck disable=SC2034  # variables are used by the scripts that source this

if [ -n "${SELAR_DEPLOY_COMMON_LOADED:-}" ]; then
  return 0
fi
SELAR_DEPLOY_COMMON_LOADED=1

set -o pipefail
umask 022

DEPLOY_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$DEPLOY_DIR/../.." && pwd)"

# Production identifiers (not secrets). Override with env vars if they change.
VERCEL_SCOPE="${SELAR_VERCEL_SCOPE:-kavirus-projects}"
VERCEL_ORG="${SELAR_VERCEL_ORG_ID:-team_9lSup3KXRbUnnkgmky5APa2R}"
VERCEL_API_PROJECT="${SELAR_VERCEL_API_PROJECT:-selar-api}"
VERCEL_API_PROJECT_ID="${SELAR_VERCEL_API_PROJECT_ID:-prj_F6ujMNwxGJBHQjUfuW7RiIMmk6P9}"
VERCEL_CONSOLE_PROJECT="${SELAR_VERCEL_CONSOLE_PROJECT:-selar-console}"
VERCEL_CONSOLE_PROJECT_ID="${SELAR_VERCEL_CONSOLE_PROJECT_ID:-prj_Te8uNGcfAqoaOfP4Bzowo9zmpUjv}"
MODAL_APP="${SELAR_MODAL_APP:-selar-worker}"
MODAL_SECRET="${SELAR_MODAL_SECRET:-selar-worker-secrets}"
API_URL="${SELAR_API_URL:-https://selar-api.vercel.app}"
CONSOLE_URL="${SELAR_CONSOLE_URL:-https://selar-console.vercel.app}"

# Env var names each runtime needs (names only; see env-sync.sh).
REQUIRED_API_ENV="APP_ENV JWT_SECRET DATABASE_URL CORS_ORIGIN STORAGE_BACKEND S3_ENDPOINT S3_BUCKET S3_REGION S3_ACCESS_KEY_ID S3_SECRET_ACCESS_KEY WORKER_URL WORKER_TRIGGER_SECRET"
REQUIRED_CONSOLE_ENV="API_INTERNAL_URL"
REQUIRED_WORKER_ENV="DATABASE_URL GEMINI_API_KEY WORKER_TRIGGER_SECRET STORAGE_BACKEND S3_ENDPOINT S3_BUCKET S3_REGION S3_ACCESS_KEY_ID S3_SECRET_ACCESS_KEY"
# Optional worker keys copied by deploy-worker.sh --sync-secrets when present.
OPTIONAL_WORKER_ENV="S3_FORCE_PATH_STYLE GEMINI_MULTIMODAL_EMBEDDING_MODEL GEMINI_EMBEDDING_DIMENSION GEMINI_TEXT_MODEL INGESTION_LEASE_SECONDS"

DRY_RUN="${DRY_RUN:-0}"

# ---------- output ----------
if [ -t 1 ] && [ -z "${NO_COLOR:-}" ]; then
  C_RED=$'\033[31m'; C_GRN=$'\033[32m'; C_YEL=$'\033[33m'; C_BLU=$'\033[34m'; C_DIM=$'\033[2m'; C_OFF=$'\033[0m'
else
  C_RED=""; C_GRN=""; C_YEL=""; C_BLU=""; C_DIM=""; C_OFF=""
fi

log()  { printf '%s==>%s %s\n' "$C_BLU" "$C_OFF" "$*"; }
info() { printf '    %s\n' "$*"; }
ok()   { printf '%s ok %s %s\n' "$C_GRN" "$C_OFF" "$*"; }
warn() { printf '%swarn%s %s\n' "$C_YEL" "$C_OFF" "$*" >&2; }
err()  { printf '%sFAIL%s %s\n' "$C_RED" "$C_OFF" "$*" >&2; }
die()  { err "$*"; exit 1; }

# run CMD...: execute, or only print it in dry-run mode. Never pass secrets as args.
run() {
  if [ "$DRY_RUN" = "1" ]; then
    printf '%s[dry-run]%s %s\n' "$C_DIM" "$C_OFF" "$*"
    return 0
  fi
  printf '%s$ %s%s\n' "$C_DIM" "$*" "$C_OFF"
  "$@"
}

# parse_common_flag FLAG: flags shared by every script. Returns 1 if unknown.
parse_common_flag() {
  case "$1" in
    --dry-run|-n) DRY_RUN=1; export DRY_RUN ;;
    --yes|-y) SELAR_YES=1; export SELAR_YES ;;
    --secrets=*) SELAR_SECRETS="${1#--secrets=}"; export SELAR_SECRETS ;;
    *) return 1 ;;
  esac
  return 0
}

# ---------- secrets ----------
secrets_file() {
  printf '%s' "${SELAR_SECRETS:-$HOME/.hermes/secrets/selar-deploy.env}"
}

file_mode() {
  stat -f '%Lp' "$1" 2>/dev/null || stat -c '%a' "$1" 2>/dev/null || echo '?'
}

file_owner_uid() {
  stat -f '%u' "$1" 2>/dev/null || stat -c '%u' "$1" 2>/dev/null || echo '?'
}

# load_secrets: parse KEY=value lines and export them; values are never printed.
# Blank lines, comments and non KEY=value lines are ignored; one pair of
# surrounding quotes is stripped; empty values are skipped. The file is
# authoritative: it overrides same-named variables already in the shell (a
# stray DATABASE_URL_DIRECT must not redirect a migration). To use exported
# variables instead, set SELAR_SECRETS=env.
load_secrets() {
  local file key val line count=0
  file="$(secrets_file)"
  if [ "$file" = "env" ]; then
    info "secrets: using the already-exported environment (SELAR_SECRETS=env)"
    return 0
  fi
  [ -f "$file" ] || die "secrets file not found: $file (set SELAR_SECRETS=/path or SELAR_SECRETS=env)"
  [ -r "$file" ] || die "secrets file not readable: $file"
  case "$(file_mode "$file")" in
    600|400) ;;
    *) warn "secrets file mode is $(file_mode "$file"); run: chmod 600 $file" ;;
  esac
  while IFS= read -r line || [ -n "$line" ]; do
    line="${line%$'\r'}"
    case "$line" in ''|'#'*) continue ;; esac
    case "$line" in *=*) ;; *) continue ;; esac
    key="${line%%=*}"
    key="${key#export }"
    key="$(printf '%s' "$key" | tr -d '[:space:]')"
    val="${line#*=}"
    printf '%s' "$key" | grep -Eq '^[A-Za-z_][A-Za-z0-9_]*$' || continue
    case "$val" in
      \"*\") val="${val#\"}"; val="${val%\"}" ;;
      \'*\') val="${val#\'}"; val="${val%\'}" ;;
    esac
    [ -n "$val" ] || continue
    export "$key=$val"
    count=$((count + 1))
  done < "$file"
  info "secrets: loaded $count values from $file (values hidden)"
}

# secret_names: names (only) of non-empty keys in the secrets file.
secret_names() {
  local file
  file="$(secrets_file)"
  if [ "$file" = "env" ] || [ ! -r "$file" ]; then
    return 0
  fi
  grep -E '^[[:space:]]*(export[[:space:]]+)?[A-Za-z_][A-Za-z0-9_]*=.+' "$file" \
    | sed -E 's/^[[:space:]]*(export[[:space:]]+)?([A-Za-z_][A-Za-z0-9_]*)=.*/\2/' \
    | sort -u
}

# require_vars NAME...: fail if any variable is empty. Prints names only.
require_vars() {
  local missing="" name
  for name in "$@"; do
    [ -n "${!name:-}" ] || missing="$missing $name"
  done
  [ -z "$missing" ] || die "missing required values (names only):$missing"
}

# url_host URL: host of a URL without credentials, port or path.
url_host() {
  printf '%s' "$1" | sed -E 's#^[a-zA-Z][a-zA-Z0-9+.-]*://##; s#^[^@/]*@##; s#[:/?].*$##'
}

# is_pooled_db_url URL: true for Neon/PgBouncer pooled endpoints, or when the
# URL equals DATABASE_URL_POOLED.
is_pooled_db_url() {
  case "$(url_host "$1")" in *-pooler.*|*pooler*) return 0 ;; esac
  if [ -n "${DATABASE_URL_POOLED:-}" ] && [ "$1" = "$DATABASE_URL_POOLED" ]; then
    return 0
  fi
  return 1
}

# ---------- tools and logins ----------
require_cmds() {
  local missing="" c
  for c in "$@"; do
    command -v "$c" >/dev/null 2>&1 || missing="$missing $c"
  done
  [ -z "$missing" ] || die "missing CLIs:$missing"
}

check_vercel_login() {
  local who
  who="$(vercel whoami --scope "$VERCEL_SCOPE" 2>/dev/null | tail -1)"
  [ -n "$who" ] || die "vercel is not logged in for scope $VERCEL_SCOPE (run: vercel login, or export VERCEL_TOKEN)"
  ok "vercel logged in as $who (scope $VERCEL_SCOPE)"
}

check_modal_login() {
  modal app list --json >/dev/null 2>&1 \
    || die "modal is not authenticated (run: modal token new, or export MODAL_TOKEN_ID/MODAL_TOKEN_SECRET)"
  ok "modal authenticated"
}

# use_file_modal_token: the secrets file holds MODAL_TOKEN_ID/SECRET. Modal's
# CLI prefers env tokens over ~/.modal.toml, so a stale value in the file would
# silently switch workspaces. Keep the file tokens only if no profile exists
# (CI) or SELAR_MODAL_ENV_TOKEN=1.
modal_token_policy() {
  if [ "${SELAR_MODAL_ENV_TOKEN:-0}" = "1" ]; then
    return 0
  fi
  if [ -f "$HOME/.modal.toml" ]; then
    unset MODAL_TOKEN_ID MODAL_TOKEN_SECRET
  fi
}

# setup_go_cache: a user-owned Go build cache (shared scratch caches hit
# permission errors when another user/agent created them).
setup_go_cache() {
  local base="${SELAR_DEPLOY_CACHE:-${TMPDIR:-/tmp}}"
  base="${base%/}/selar-deploy-gocache-$(id -u)"
  mkdir -p "$base" 2>/dev/null || true
  if [ ! -w "$base" ] || [ "$(file_owner_uid "$base")" != "$(id -u)" ]; then
    base="$(mktemp -d "${TMPDIR:-/tmp}/selar-deploy-gocache.XXXXXX")"
  fi
  chmod 700 "$base" 2>/dev/null || true
  export GOCACHE="$base"
}

# ---------- git ----------
git_short_sha() { git -C "$REPO_ROOT" rev-parse --short HEAD; }

git_problem() {
  if [ "$DRY_RUN" = "1" ] || [ "${SELAR_ALLOW_NON_MAIN:-0}" = "1" ]; then
    warn "git: $1"
  else
    err "git: $1"
  fi
}

# check_git_release_state: clean tree, on main, equal to origin/main.
# Dry-run (or SELAR_ALLOW_NON_MAIN=1) reports problems as warnings.
check_git_release_state() {
  local problems=0 branch
  if ! git -C "$REPO_ROOT" fetch --quiet origin main; then
    git_problem "git fetch origin main failed"; problems=1
  fi
  if [ -n "$(git -C "$REPO_ROOT" status --porcelain)" ]; then
    git_problem "working tree is not clean (commit or stash first)"; problems=1
  fi
  branch="$(git -C "$REPO_ROOT" rev-parse --abbrev-ref HEAD)"
  if [ "$branch" != "main" ]; then
    git_problem "on branch '$branch', not main"; problems=1
  fi
  if [ "$(git -C "$REPO_ROOT" rev-parse HEAD)" != "$(git -C "$REPO_ROOT" rev-parse origin/main 2>/dev/null)" ]; then
    git_problem "HEAD $(git_short_sha) is not origin/main $(git -C "$REPO_ROOT" rev-parse --short origin/main 2>/dev/null)"; problems=1
  fi
  if [ "$problems" = "0" ]; then
    ok "git: clean main at $(git_short_sha), equal to origin/main"
    return 0
  fi
  if [ "$DRY_RUN" != "1" ] && [ "${SELAR_ALLOW_NON_MAIN:-0}" != "1" ]; then
    die "refusing to deploy: not a clean, up-to-date main (SELAR_ALLOW_NON_MAIN=1 overrides)"
  fi
}

# check_ci_status: warn unless GitHub CI for HEAD is green (needs gh).
check_ci_status() {
  local sha out
  command -v gh >/dev/null 2>&1 || { warn "gh not installed; CI status for HEAD not checked"; return 0; }
  sha="$(git -C "$REPO_ROOT" rev-parse HEAD)"
  out="$(cd "$REPO_ROOT" && gh run list --commit "$sha" --workflow ci.yml --json conclusion,status --jq '.[0] | "\(.status) \(.conclusion)"' 2>/dev/null)"
  case "$out" in
    "completed success") ok "CI (ci.yml) is green for $(git_short_sha)" ;;
    "") warn "no CI run found for $(git_short_sha)" ;;
    *) warn "CI (ci.yml) for $(git_short_sha): $out" ;;
  esac
}

# export_release_tree SUBDIR: copy the committed HEAD version of SUBDIR into a
# fresh temp dir, so no .env.local, node_modules or .vercel dir can leak into a
# deploy. Prints the path.
export_release_tree() {
  local sub="$1" dest
  dest="$(mktemp -d "${TMPDIR:-/tmp}/selar-release.XXXXXX")"
  chmod 755 "$dest"
  git -C "$REPO_ROOT" archive --format=tar HEAD "$sub" | tar -x -C "$dest"
  printf '%s/%s' "$dest" "$sub"
}

# remove_vercel_artifacts DIR: `vercel link`/`vercel pull` write .env.local (with
# secret values) and .vercel/. Never leave them behind.
remove_vercel_artifacts() {
  [ -n "${1:-}" ] || return 0
  rm -f "$1/.env.local" "$1/.env.production.local" 2>/dev/null || true
  rm -rf "$1/.vercel" 2>/dev/null || true
}

confirm_production() {
  local what="$1" answer
  if [ "$DRY_RUN" = "1" ] || [ "${SELAR_YES:-0}" = "1" ]; then
    return 0
  fi
  [ -t 0 ] || die "non-interactive run: pass --yes to confirm the production $what deploy"
  printf 'Deploy %s to PRODUCTION from %s? Type yes: ' "$what" "$(git_short_sha)"
  read -r answer
  [ "$answer" = "yes" ] || die "aborted"
}

# http_status URL [curl args...]: print the HTTP status code (000 on failure).
http_status() {
  local url="$1" code; shift
  code="$(curl -sS -o /dev/null -w '%{http_code}' --max-time 20 "$@" "$url" 2>/dev/null)" || true
  printf '%s' "${code:-000}"
}

# wait_for_status URL EXPECTED [TRIES]: poll until URL returns EXPECTED.
wait_for_status() {
  local url="$1" expected="$2" tries="${3:-12}" i code=""
  for i in $(seq 1 "$tries"); do
    code="$(http_status "$url")"
    [ "$code" = "$expected" ] && return 0
    [ "$i" -lt "$tries" ] && sleep 5
  done
  err "$url returned $code (expected $expected)"
  return 1
}

# modal_secret_key_names: key NAMES stored in the Modal secret (one per line),
# via a ~10 s ephemeral function. Never prints values.
modal_secret_key_names() {
  local out
  out="$(cd "$DEPLOY_DIR" && SELAR_MODAL_SECRET="$MODAL_SECRET" modal run modal_secret_keys.py 2>&1)" \
    || { printf '%s\n' "$out" | tail -5 >&2; return 1; }
  printf '%s\n' "$out" | sed -n '/^SELAR_SECRET_KEYS_BEGIN$/,/^SELAR_SECRET_KEYS_END$/p' | sed '1d;$d'
}
