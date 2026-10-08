# Rolling back a SELAR production release

Roll back the piece that broke, in reverse deploy order: console → API →
worker. Database migrations are forward-only. Fix the schema with a new numbered migration;
never edit or delete an applied one (the runner checks checksums).

After any rollback, run `scripts/deploy/smoke.sh`.

## Console and API (Vercel)

```bash
# See recent production deployments (newest first)
vercel ls selar-console --scope kavirus-projects --environment production
vercel ls selar-api     --scope kavirus-projects --environment production

# Instant rollback to the previous production deployment
#   (run from any directory; --cwd is not needed with an explicit URL)
vercel rollback <previous-deployment-url> --scope kavirus-projects --yes
vercel rollback status selar-api --scope kavirus-projects

# Or promote a specific known-good deployment
vercel promote <deployment-url> --scope kavirus-projects --yes
```

Notes:

- `vercel rollback` and `vercel promote` reuse an existing build, so they take seconds and do not rebuild.
  Env vars are the ones stored with that deployment. If you rolled back
  because of an env var change, fix the variable and redeploy instead.
- After a rollback, Vercel pauses automatic production aliasing of new deployments
  for that project. The next `deploy-api.sh`/`deploy-console.sh` (`vercel deploy --prod`) promotes
  normally. If it does not, run `vercel promote <url>`.
- If the API rollback crosses a migration, check that the older API still works with
  the newer schema. Migrations are additive, so it normally does.

## Worker (Modal)

```bash
modal app history selar-worker          # versions, commit and who deployed
modal app rollback selar-worker         # back to the previous version
modal app rollback selar-worker v3      # back to a specific version
```

Rollback does not touch the `selar-worker-secrets` secret. If a
`--sync-secrets` run wrote a bad value, fix it in the Modal dashboard or
correct the local file and run `deploy-worker.sh --sync-secrets` again.
Modal has no secret history.

Alternatively, redeploy a known-good commit: `git checkout <sha>` then
`SELAR_ALLOW_NON_MAIN=1 scripts/deploy/deploy-worker.sh`.

## Database (Neon)

- Forward fix: add `services/selar-api/internal/store/migrations/NNN_fix.sql`, merge it, then run `migrate.sh`.
- For data loss or corruption, use Neon's point-in-time restore (branch restore) from the Neon console.
  Restore into a new branch first, verify it, then switch.
- If Neon's history window does not reach far enough back (or Neon itself is the problem), use the
  nightly encrypted GCS backups (30 days kept):

  ```bash
  scripts/deploy/restore-db.sh --list
  scripts/deploy/restore-db.sh --object gs://selar-db-backups-261008/neon/<name>.dump.age \
    --identity ~/.hermes/secrets/selar-backup-age.key --target '<EMPTY new branch direct URL>' --allow-remote
  ```

  Rehearse against a throwaway local Postgres first (see `docs/DEPLOYMENT.md` §7), then point
  `DATABASE_URL` (API, pooled) and the worker's `DATABASE_URL` (direct) at the new branch and redeploy.
