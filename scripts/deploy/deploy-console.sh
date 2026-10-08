#!/usr/bin/env bash
# deploy-console.sh: deploy the Next.js console (services/selar-console) to
# Vercel production.
#
#   scripts/deploy/deploy-console.sh [--dry-run] [--yes] [--preview]
#
# --preview builds a Vercel preview deployment instead (production untouched).
#
# Vercel builds it remotely (pnpm install --frozen-lockfile && pnpm build, from
# vercel.json). API_INTERNAL_URL is already set in the Vercel project.
set -euo pipefail
# shellcheck source=scripts/deploy/common.sh
. "$(dirname "$0")/common.sh"
# shellcheck source=scripts/deploy/vercel-deploy.sh
. "$DEPLOY_DIR/vercel-deploy.sh"

while [ $# -gt 0 ]; do
  parse_common_flag "$1" && { shift; continue; }
  case "$1" in
    --preview) SELAR_PREVIEW=1; export SELAR_PREVIEW ;;
    -h|--help) sed -n '2,10p' "$0" | sed 's/^# \{0,1\}//'; exit 0 ;;
    *) die "unknown argument: $1" ;;
  esac
  shift
done

vercel_deploy "console" "$VERCEL_CONSOLE_PROJECT" "$VERCEL_CONSOLE_PROJECT_ID" \
  "services/selar-console" "$CONSOLE_URL/login" 200
