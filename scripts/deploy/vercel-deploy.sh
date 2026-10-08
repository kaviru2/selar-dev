#!/usr/bin/env bash
# vercel-deploy.sh: shared implementation of deploy-api.sh and deploy-console.sh.
# Not meant to be run directly.
#
# How it deploys: export the committed HEAD of the service dir into a fresh
# temp dir (git archive), then `vercel deploy --prod` from there with
# VERCEL_ORG_ID/VERCEL_PROJECT_ID in the environment. No `vercel link`, so no
# .env.local with pulled secrets and no .vercel dir is written in the repo.
# Production env vars are already stored in Vercel and are NOT touched here.

# vercel_deploy LABEL PROJECT PROJECT_ID SERVICE_SUBDIR HEALTH_URL EXPECTED_CODE
vercel_deploy() {
  local label="$1" project="$2" project_id="$3" sub="$4" health_url="$5" expected="$6"
  local tree out url

  log "$label: preflight"
  require_cmds vercel git curl
  check_vercel_login
  if ! VERCEL_ORG_ID="$VERCEL_ORG" vercel project inspect "$project" --scope "$VERCEL_SCOPE" >/dev/null 2>&1; then
    die "Vercel project $project not found in scope $VERCEL_SCOPE"
  fi
  ok "Vercel project $project exists"
  info "current production: $(http_status "$health_url") from $health_url"

  local target="--prod" where="production"
  if [ "${SELAR_PREVIEW:-0}" = "1" ]; then
    target=""; where="preview (production untouched)"
  fi

  if [ "$DRY_RUN" = "1" ]; then
    printf '%s[dry-run]%s git archive HEAD %s -> temp dir\n' "$C_DIM" "$C_OFF" "$sub"
    printf '%s[dry-run]%s VERCEL_ORG_ID=%s VERCEL_PROJECT_ID=%s vercel deploy %s --yes --scope %s -m gitSha=%s  [%s]\n' \
      "$C_DIM" "$C_OFF" "$VERCEL_ORG" "$project_id" "$target" "$VERCEL_SCOPE" "$(git_short_sha)" "$where"
    [ -n "$target" ] && printf '%s[dry-run]%s wait for %s to return %s\n' "$C_DIM" "$C_OFF" "$health_url" "$expected"
    return 0
  fi

  [ -z "$target" ] || confirm_production "$label ($project)"
  tree="$(export_release_tree "$sub")"
  # shellcheck disable=SC2064  # expand now: the trap must remove this tree
  trap "rm -rf '${tree%/"$sub"}'" EXIT
  remove_vercel_artifacts "$tree"
  log "$label: vercel deploy to $where ($(git_short_sha))"
  # shellcheck disable=SC2086  # $target is empty or --prod
  out="$(cd "$tree" && VERCEL_ORG_ID="$VERCEL_ORG" VERCEL_PROJECT_ID="$project_id" \
    vercel deploy $target --yes --scope "$VERCEL_SCOPE" \
      -m "gitSha=$(git -C "$REPO_ROOT" rev-parse HEAD)" -m "deployedBy=scripts/deploy")" \
    || die "$label: vercel deploy failed (production unchanged unless the deploy reached Ready)"
  remove_vercel_artifacts "$tree"
  url="$(printf '%s\n' "$out" | grep -Eo 'https://[A-Za-z0-9.-]+\.vercel\.app' | tail -1)"
  ok "$label deployment: ${url:-unknown URL}"
  if [ -z "$target" ]; then
    info "preview deployments sit behind Vercel deployment protection; open it while logged in or use: vercel curl $url"
    return 0
  fi

  log "$label: wait for $health_url -> $expected"
  wait_for_status "$health_url" "$expected" 12 || die "$label: health check failed; consider: vercel rollback --scope $VERCEL_SCOPE (see scripts/deploy/ROLLBACK.md)"
  ok "$label healthy"
}
