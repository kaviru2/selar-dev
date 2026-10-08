#!/usr/bin/env bash
# deploy-api.sh: deploy the Go API (services/selar-api) to Vercel production.
#
#   scripts/deploy/deploy-api.sh [--dry-run] [--yes] [--preview]
#
# --preview builds a Vercel preview deployment instead (production untouched).
#
# Run migrate.sh first when the release adds migrations. Production env vars
# already live in the Vercel project and are not modified here.
set -euo pipefail
# shellcheck source=scripts/deploy/common.sh
. "$(dirname "$0")/common.sh"
# shellcheck source=scripts/deploy/vercel-deploy.sh
. "$DEPLOY_DIR/vercel-deploy.sh"

while [ $# -gt 0 ]; do
  parse_common_flag "$1" && { shift; continue; }
  case "$1" in
    --preview) SELAR_PREVIEW=1; export SELAR_PREVIEW ;;
    -h|--help) sed -n '2,9p' "$0" | sed 's/^# \{0,1\}//'; exit 0 ;;
    *) die "unknown argument: $1" ;;
  esac
  shift
done

vercel_deploy "api" "$VERCEL_API_PROJECT" "$VERCEL_API_PROJECT_ID" \
  "services/selar-api" "$API_URL/healthz" 200
