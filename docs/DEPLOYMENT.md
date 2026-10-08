# Deploying SELAR (Vercel + Modal + managed Postgres + object storage)

This guide covers the serverless deployment shape tracked in issue #72:

| Piece | Runs on | Root directory | Entry point |
|---|---|---|---|
| Next.js console | Vercel project #1 | `services/selar-console` | Next.js preset (`vercel.json`) |
| Go API | Vercel project #2 | `services/selar-api` | Go preset, `cmd/server/main.go` (`vercel.json`) |
| Python worker | Modal | `services/selar-worker` | `modal deploy modal_app.py` |
| Postgres 16+ with pgvector | Neon (recommended) or any managed Postgres with TLS | — | migrations via `go run ./cmd/migrate` |
| PDFs and extracted figures | S3-compatible bucket (Cloudflare R2, Supabase Storage, AWS S3) | — | `STORAGE_BACKEND=s3` |

Docker Compose and the local three-terminal setup are unchanged. They keep
`STORAGE_BACKEND=local`, the shared `selar_uploads` volume and the polling worker.
Nothing in this guide is required for local development.

> Creating accounts, projects, buckets and secrets is a one-time manual step for the
> operator. Once they exist, releases go through the scripts in
> [`scripts/deploy/`](../scripts/deploy) (see [One-command deploy](#one-command-deploy)).

## One-command deploy

Once the stack exists (sections 1–5 below), release `main` with:

```bash
git checkout main && git pull
scripts/deploy/deploy-all.sh --dry-run   # read-only checks + the full plan
scripts/deploy/deploy-all.sh             # asks once, then deploys
make deploy ARGS="--skip-tests --yes"    # same, via make, unattended
```

`deploy-all.sh` runs these steps in order and stops at the first failure:

1. **Preflight.** Checks that the required CLIs are installed, that Vercel and Modal are logged in, and that the secrets file is readable. Runs `env-sync.sh` to check that every required env var *name* exists in Vercel and in the Modal secret.
2. **Git.** The tree must be clean, on `main`, and equal to `origin/main`. It also reports the `ci.yml` status for HEAD. `SELAR_ALLOW_NON_MAIN=1` overrides this for hotfix or rollback builds.
3. **Tests.** Runs `go vet` and `go test -short` for the API, `vitest` for the console when `node_modules` exists, and `pytest` for the worker when its dependencies are installed. Every database URL is removed from the test environment. `--skip-tests` skips this step.
4. **Migrate.** Runs `migrate.sh`: status, then apply, then status again. Production must end with `0 pending`. It uses `DATABASE_URL_DIRECT` and refuses a pooled (`-pooler`) URL.
5. **Worker.** Runs `deploy-worker.sh`, which does `modal deploy` from a `git archive` of HEAD. This takes about 5 minutes. The script then waits for `/health`.
6. **API.** Runs `deploy-api.sh`, which does `vercel deploy --prod` of `services/selar-api`, then waits for `/healthz`.
7. **Console.** Runs `deploy-console.sh`, which does `vercel deploy --prod` of `services/selar-console`, then waits for `/login`.
8. **Smoke.** Runs `smoke.sh`.

| Script | What it does | Useful flags |
|---|---|---|
| `deploy-all.sh` | the whole release, in the order above | `--dry-run`, `--yes`, `--skip-tests`, `--only=api,console`, `--sync-secrets`, `--auth-smoke` |
| `migrate.sh` | status → apply → status on production | `--dry-run` / `--status-only` (read-only) |
| `deploy-worker.sh` | Modal deploy of the worker | `--sync-secrets` (overwrite `selar-worker-secrets` from the secrets file), `--allow-key-removal` |
| `deploy-api.sh` / `deploy-console.sh` | Vercel production deploy | `--preview` (preview deployment only; production untouched) |
| `smoke.sh` | read-only production checks | `--auth-flow` (throwaway account, deleted afterwards) |
| `env-sync.sh` | required env var **names** vs Vercel and Modal (never values, never writes) | `--skip-modal` |
| [`ROLLBACK.md`](../scripts/deploy/ROLLBACK.md) | `vercel rollback` / `promote`, `modal app rollback`, forward-fix migrations | |

Every script accepts `--dry-run` and `--help`. In dry-run mode, read-only checks (logins,
migration status, env names and smoke checks against current production) run for
real. Anything that would change production is printed instead.

**Secrets.** The scripts read `~/.hermes/secrets/selar-deploy.env`, or the file named by
`SELAR_SECRETS=/path` (`SELAR_SECRETS=env` uses variables that are already exported). This is a
plain `KEY=value` file with mode `600`. Names used: `DATABASE_URL_DIRECT` and `DATABASE_URL_POOLED` (Neon),
`WORKER_URL`, `WORKER_TRIGGER_SECRET`, `GEMINI_API_KEY`, `JWT_SECRET`, `STORAGE_BACKEND` and `S3_*`.
`MODAL_TOKEN_ID`/`MODAL_TOKEN_SECRET` are used only when there is no `~/.modal.toml` profile, as in CI.
The file is parsed and never `source`d, no value is ever printed, and values reach CLIs through
the environment or a `0600` temp file, never through argv.

**What the scripts do not do.**
- They do not overwrite Vercel env vars. Production values are managed in the Vercel dashboard or with `vercel env`.
- They do not touch the Modal secret unless you pass `--sync-secrets`. That flag replaces the whole secret and refuses if it would drop a key that exists in Modal but not locally.
- They do not run `vercel link`. Deploys run from a fresh `git archive` of HEAD with `VERCEL_ORG_ID`/`VERCEL_PROJECT_ID` set, so no `.env.local` (which would hold pulled secrets) and no `.vercel/` directory is ever written into the checkout.

**Hygiene the scripts handle for you.** The scripts set `umask 022`; under a restrictive umask,
`.vercel/` and build caches become unreadable. They use a private, user-owned `GOCACHE`. They
build only committed code, and they run migrations before the API that needs them.

**Smoke checks** (`smoke.sh`, about 5 s):
- API: `/healthz` returns 200. `/api/users/me` without a token returns 401. The CORS preflight allows the console origin.
- Worker: `/health` returns 200. `POST /jobs/trigger` without the secret returns 401.
- Console: `/login` and `/register` return 200, and `/` returns the landing page.
- Redirects: a signed-out deep link redirects to `/login?from=…`. With a session cookie, `/` and `/login` redirect into the app. An off-site `from=` is ignored. `session-expired` redirects to `/login`. The `/api` proxy rejects requests without a cookie.
- Bucket: CORS allows the console origin.

With `--auth-flow`, the script also registers `deploy-smoke-<time>-<random>@example.invalid` with
a random password. It checks that the cookie is `Secure`+`HttpOnly`, that `/api/users/me`,
`/api/documents` and `/library` load, that API login works, and that logout works. It then
deletes the account with `psql` over `DATABASE_URL_DIRECT`; dependent rows go through
`ON DELETE CASCADE`. The delete runs on exit even when a check fails. Only invented data is used.

**CI (optional, disabled).** [`.github/workflows/deploy.yml.disabled`](../.github/workflows/deploy.yml.disabled)
is a manual-dispatch template that runs the same scripts. GitHub ignores it until it is renamed
to `.yml`. The repository secrets it needs are listed in the file. None are set.

## How requests flow

```
browser ──► console (Vercel) ──/api/* proxy──► Go API (Vercel) ──► Postgres (Neon, TLS)
   │                                              │  ▲
   │  presigned PUT/GET (short-lived)             │  └── job committed to ingestion_jobs
   └──────────────► bucket (R2/S3) ◄──────────────┤
                                                  └── POST WORKER_URL/jobs/trigger (secret header, 3 s bound)
                                                          │
                                       Modal web ──spawn──► process_job(job_id) ──► Postgres + bucket + Gemini
                                       Modal sweep (every 5 min) ── drains ready / expired-lease jobs
```

- The browser only talks to the console origin and, for file bytes, to the bucket through signed URLs.
  The console server attaches the httpOnly JWT and forwards `/api/*` to the Go API (`API_INTERNAL_URL`).
- **Large PDFs.** Vercel Functions accept request bodies of about 4.5 MB at most. With `STORAGE_BACKEND=s3` the console:
  1. asks `POST /api/documents/upload-url`, which returns a 10-minute presigned PUT bound to the declared size and `application/pdf`, for a server-generated key `users/<user>/uploads/<uuid>.pdf`;
  2. PUTs the file straight to the bucket;
  3. calls `POST /api/documents/upload-complete`. The API checks the object exists, its size (≤ 50 MB), its stored type and the `%PDF-` magic bytes before creating records and queueing ingestion.
- Reading a PDF or figure returns a 302 to a 5-minute presigned GET after the ownership check.
- The ingestion queue in Postgres is the source of truth. A trigger is only a hint: Modal claims the job with the same lease SQL as the local poller (`FOR UPDATE SKIP LOCKED`, backoff, max attempts), so duplicate triggers cannot double-process a job. The scheduled sweep picks up anything a lost trigger missed.

## 1. Managed Postgres (Neon recommended)

1. Create a Neon project (Postgres 16 or 17) in the region closest to your Vercel functions and Modal (for example `aws-us-east-1` with Vercel `iad1`).
2. Neon gives two connection strings. Use both:
   - **Direct** (`ep-xxx.region.aws.neon.tech`): for **migrations** and the **worker**. The migration runner holds a session-level advisory lock and runs each file in its own transaction, which needs a real session. asyncpg also caches prepared statements.
   - **Pooled** (`ep-xxx-pooler.region.aws.neon.tech`, PgBouncer): for the **Go API** on Vercel, where many short-lived function instances connect at once.
     Neon's pooler supports protocol-level prepared statements. If you ever see `prepared statement ... already exists`, append `&default_query_exec_mode=simple_protocol` to the API URL.
3. Always keep `sslmode=require` (or `verify-full` with `sslrootcert=system`). In production the API refuses a remote `DATABASE_URL` without TLS, and the migration runner refuses one unless `MIGRATION_ALLOW_INSECURE=true`.
4. **pgvector.** Migration `001` runs `CREATE EXTENSION IF NOT EXISTS vector`. This works on Neon, Supabase and RDS (where the extension is allow-listed) as the database owner role. On another provider, enable the extension in its console first if the owner role cannot create it.

### Migrations (a separate deploy step)

The API never migrates at startup. Run the runner from a trusted machine or a CI job **before** promoting a new API deployment:

```bash
cd services/selar-api
DATABASE_URL='postgres://USER:PASSWORD@ep-xxx.region.aws.neon.tech/selar?sslmode=require' \
  go run ./cmd/migrate -status   # read-only: lists pending migrations
DATABASE_URL='…same direct URL…' go run ./cmd/migrate
```

How the runner behaves:

- It applies the embedded `internal/store/migrations/NNN_*.sql` in order, each in its own transaction, and records the version plus a SHA-256 checksum in `schema_migrations`.
- Re-running it is a no-op.
- It refuses to start if an applied file's checksum changed (never edit an applied migration; add a new one), if the database has an unknown newer version, or if versions have a gap.
- **Existing Docker-initialised databases are adopted.** When `schema_migrations` is missing, the runner probes for the last object each migration created and records the contiguous applied prefix as `adopted` before applying the rest. Ambiguous partial schemas are refused rather than guessed.
- `MIGRATION_DATABASE_URL` takes precedence over `DATABASE_URL` if you keep both in one environment.
- Docker Compose runs the same runner as the one-shot `selar-migrate` service. `make migrate` runs it through Compose, and `DATABASE_URL=… make migrate-url` runs it against any URL.

## 2. Object storage

Any S3-compatible service works. Cloudflare R2 is the cheapest to start with because it charges no egress.

1. Create a **private** bucket, for example `selar-uploads`. Do not enable public access.
2. Create an access key limited to that bucket with object read, write, delete and list.
3. Configure **bucket CORS** so the browser can PUT uploads and pdf.js can read PDFs. Replace the origin with your exact console origin:

   ```json
   [
     {
       "AllowedOrigins": ["https://selar-console.example.vercel.app"],
       "AllowedMethods": ["PUT", "GET", "HEAD"],
       "AllowedHeaders": ["Content-Type", "Range"],
       "ExposeHeaders": ["Content-Length", "Content-Range", "Accept-Ranges", "ETag"],
       "MaxAgeSeconds": 600
     }
   ]
   ```

4. Endpoints:
   - R2: `https://<account-id>.r2.cloudflarestorage.com`, `S3_REGION=auto`.
   - Supabase Storage (S3 protocol): `https://<project-ref>.supabase.co/storage/v1/s3` with `S3_FORCE_PATH_STYLE=true` and your project region.
   - AWS: `https://s3.<region>.amazonaws.com`.
5. Do not add lifecycle expiry rules on `users/` or document prefixes: finalized direct uploads stay at their `users/<user>/uploads/` key and are deleted when the document is deleted. Abandoned (never finalized) uploads are bounded by the 10-minute URL and 50 MB size limit; clean them up manually if needed.

The API and the worker must use the **same bucket and credentials**. Stored locators look like `s3://<bucket>/<key>`.

## 3. Go API on Vercel

Vercel's Go runtime runs a standard `net/http` server from `cmd/server/main.go` and passes the port in `PORT`. See [the Go runtime docs](https://vercel.com/docs/functions/runtimes/go).

1. **New Project** → import `kaviru2/selar-dev` → **Root Directory** `services/selar-api`. The framework preset comes from `services/selar-api/vercel.json` (`"framework": "go"`).
2. Set the environment variables (Production, plus Preview if you use it):

   | Variable | Value |
   |---|---|
   | `APP_ENV` | `production` (enforces a non-default `JWT_SECRET` and TLS DB URLs) |
   | `JWT_SECRET` | 32+ random bytes, e.g. `openssl rand -base64 48` (only the API signs and verifies tokens) |
   | `DATABASE_URL` | Neon **pooled** URL with `sslmode=require` |
   | `CORS_ORIGIN` | exact console origin(s), comma-separated, e.g. `https://selar-console.example.vercel.app`. `*` is rejected. |
   | `STORAGE_BACKEND` | `s3` |
   | `S3_ENDPOINT`, `S3_BUCKET`, `S3_REGION`, `S3_ACCESS_KEY_ID`, `S3_SECRET_ACCESS_KEY` | bucket settings (`S3_FORCE_PATH_STYLE=true` for Supabase/MinIO) |
   | `WORKER_URL` | Modal `web` endpoint URL (see §4), e.g. `https://<workspace>--selar-worker-web.modal.run` |
   | `WORKER_TRIGGER_SECRET` | 32+ random bytes; **same value as in Modal** |
   | optional `DB_MAX_CONNS` (default 4), `DB_MAX_CONN_IDLE_TIME` (30s), `DB_MAX_CONN_LIFETIME` (5m), `WORKER_TRIGGER_TIMEOUT` (3s) | tuning |

3. Deploy. Check `GET https://<api>/healthz`, which should return `{"status":"ok","service":"selar-api"}`. Logs should show `upload storage backend: s3` and `worker trigger enabled: true`. No secret values are logged.

Notes:

- The pgx pool is sized for serverless use: at most 4 connections, none kept warm, 30 s idle, 5 min lifetime and a 5 s connect timeout. A slow or unreachable database at cold start does not crash the function; the first query retries.
- Chat requests wait up to 50 s for the worker. Keep the API function's max duration at or above 60 s; the Vercel default for current plans is sufficient.

## 4. Worker on Modal

1. `pip install modal` and run `modal token new` on the operator's machine.
2. Create **one** secret named `selar-worker-secrets` (Modal dashboard → Secrets, or `modal secret create selar-worker-secrets KEY=value …`) with:

   | Key | Value |
   |---|---|
   | `DATABASE_URL` | Neon **direct** URL with `sslmode=require` |
   | `GEMINI_API_KEY` | Google AI Studio key |
   | `WORKER_TRIGGER_SECRET` | same value as the API |
   | `STORAGE_BACKEND` | `s3` |
   | `S3_ENDPOINT`, `S3_BUCKET`, `S3_REGION`, `S3_ACCESS_KEY_ID`, `S3_SECRET_ACCESS_KEY` (+ `S3_FORCE_PATH_STYLE`) | same bucket as the API |
   | optional `GEMINI_MULTIMODAL_EMBEDDING_MODEL` (`gemini-embedding-2`), `GEMINI_EMBEDDING_DIMENSION` (`3072`), `GEMINI_TEXT_MODEL`, `INGESTION_LEASE_SECONDS` (300) | tuning |

3. Deploy from the worker directory: `cd services/selar-worker && modal deploy modal_app.py`.
   This creates:
   - `web`: an ASGI endpoint. Every route except `/health` needs the `X-Selar-Worker-Secret` header, compared in constant time, and it fails closed if the secret is missing. Copy its URL into the API's `WORKER_URL`.
   - `process_job`: spawned per trigger, with a 30-minute timeout.
   - `sweep`: runs every 5 minutes, re-queues expired leases, retries jobs whose backoff has elapsed and drains anything a lost trigger missed.
4. Check: `curl https://<web-url>/health` returns `{"status":"ok"}`. `curl -X POST https://<web-url>/jobs/trigger` without the header returns 401.

Do **not** also run the Docker poller against the production database unless you want two consumers. Running both is safe (leases prevent double processing) but wastes Gemini quota on races.

## 5. Console on Vercel

1. **New Project** → same repository → **Root Directory** `services/selar-console`. `vercel.json` selects Next.js with `pnpm install --frozen-lockfile` and `pnpm build`.
2. Environment variables:

   | Variable | Value |
   |---|---|
   | `API_INTERNAL_URL` | the Go API deployment URL, e.g. `https://selar-api.example.vercel.app` (no trailing slash) |

   That is the only variable the console needs. It stores the API-issued JWT in an httpOnly cookie and `proxy.ts` only checks that the cookie is present; the API verifies every token.

3. Deploy. Then set the API's `CORS_ORIGIN` to this exact origin and redeploy the API. Also add the origin to the bucket CORS rule.

Session cookies are `httpOnly`, `secure` in production and `SameSite=Lax`. The browser never sees the JWT or any storage credential.

## 6. Sign-up protection (Google reCAPTCHA Enterprise)

Account creation is protected by an invisible, score-based reCAPTCHA Enterprise
key in GCP project `selar-research-261008` (free tier: 10,000 assessments a
month; one assessment per sign-up attempt). It is **off** unless the variables
below are set, so local development, CI and tests never call Google.

| Where | Variable | Value |
|---|---|---|
| Console (build time) | `NEXT_PUBLIC_RECAPTCHA_SITE_KEY` | the site key (public; key `selar-console-register`, domains `selar-console.vercel.app`, `localhost`) |
| API | `RECAPTCHA_PROJECT_ID` | `selar-research-261008` |
| API | `RECAPTCHA_SITE_KEY` | same site key |
| API | `RECAPTCHA_API_KEY` | API key restricted to `recaptchaenterprise.googleapis.com` (secret) |
| API (optional) | `RECAPTCHA_MIN_SCORE` | default `0.3` (only very-likely-bot traffic is refused) |

- Only `/register` (and any future Google sign-up page) loads `enterprise.js`. The
  badge is hidden and Google's required attribution text is shown under the form.
- The API calls `createAssessment` before creating the user and checks the token is
  valid, for action `register`, for our site key, and scores at or above the
  threshold. A missing, invalid or low-score token gets **403** with
  `code: recaptcha_failed`.
- **Fail-open is limited to outages**: if Google is unreachable, times out (3 s) or
  answers 429/5xx, registration continues and the API logs
  `WARNING recaptcha unavailable…`. A bad API key (400/403) fails closed.
- Any other account-creating path (e.g. Google sign-up) must call
  `h.requireHuman(w, r, token, handler.RecaptchaActionRegister)` before creating
  the user and the console must send `recaptcha_token` the same way.
- To turn it off in an emergency, remove `RECAPTCHA_API_KEY` from the API and redeploy.

Create or inspect the key: `gcloud recaptcha keys list --project selar-research-261008`.

## 7. Database backups (Google Cloud Storage, free tier)

Nightly encrypted logical backups of the Neon database, on top of Neon's own
point-in-time restore. Everything lives in GCP project `selar-research-261008`
and stays inside the always-free tier.

| Resource | Setting |
|---|---|
| Bucket `gs://selar-db-backups-261008` | `us-east1`, Standard, uniform bucket-level access, public access prevention **enforced**, versioning off, soft delete off, lifecycle **delete after 30 days** |
| Service account `selar-db-backup@selar-research-261008.iam.gserviceaccount.com` | only `roles/storage.objectCreator` on that bucket (cannot read, list, overwrite or delete) |
| Workload Identity pool `github` / provider `selar-dev` | trusts only `kaviru2/selar-dev`, `refs/heads/main`, workflow `db-backup.yml`; no service-account key exists |
| Budget `SELAR cap 1 USD` (billing account `01B63E-CB9E16-1BC9EE`) | scoped to the project; email alerts at 1/50/90/100 % |

**How a backup runs.** [`.github/workflows/db-backup.yml`](../.github/workflows/db-backup.yml)
runs daily at 02:17 UTC (or *Run workflow*). It calls
[`scripts/deploy/backup-db.sh`](../scripts/deploy/backup-db.sh):
`pg_dump -Fc` over `DATABASE_URL_DIRECT` → `age -r $BACKUP_AGE_RECIPIENT` → a
0600 temp file → `gcloud storage cp --no-clobber` to
`gs://selar-db-backups-261008/neon/selar-<UTC>-<sha>.dump.age`. Plaintext is
never written to disk. The run fails if the encrypted dump is under 1 KB or over
150 MB (30 × 150 MB < 5 GB free storage).

- GitHub repo secret `DATABASE_URL_DIRECT`; repo variable `BACKUP_AGE_RECIPIENT` (a public key).
- **Encryption key.** Client-side, age X25519. CI holds only the public recipient.
  The private identity is `~/.hermes/secrets/selar-backup-age.key` (mode 600) on the
  operator's machine. **Keep an offline copy** (password manager): without it no
  backup can be decrypted. Rotate by generating a new key
  (`age-keygen -o new.key`), updating the variable, and keeping the old key for 30 days.
- Run one by hand from an operator machine: `scripts/deploy/backup-db.sh` (needs
  `gcloud` logged in, `pg_dump` 17+, `age`; reads `BACKUP_AGE_RECIPIENT` from the secrets file).

**Restore** ([`scripts/deploy/restore-db.sh`](../scripts/deploy/restore-db.sh)):

```bash
scripts/deploy/restore-db.sh --list                 # what exists
# 1. Throwaway local check (Homebrew postgresql@17 + pgvector):
D=$(mktemp -d); initdb -D $D/data -U postgres --auth=trust >/dev/null
pg_ctl -D $D/data -o "-p 55437 -k $D" -w start && createdb -h localhost -p 55437 -U postgres selar_restore
scripts/deploy/restore-db.sh --latest --identity ~/.hermes/secrets/selar-backup-age.key \
  --target postgres://postgres@localhost:55437/selar_restore
pg_ctl -D $D/data -w stop && rm -rf "$D"           # delete the copy: it holds participant data
# 2. Real recovery: create an EMPTY Neon branch/database, then
scripts/deploy/restore-db.sh --object gs://selar-db-backups-261008/neon/<name> \
  --identity ~/.hermes/secrets/selar-backup-age.key --target '<new branch direct URL>' --allow-remote
```

The script refuses the production host, a non-empty target, a remote target
without `--allow-remote`, and an identity file that is not mode 600/400. It
downloads into a 0700 temp dir that is deleted on exit and prints row counts.
Reading objects needs an account with `storage.objects.get` on the bucket (the
project owner); the CI service account cannot read backups.

**Data protection.** Backups contain participant and research data. Keep the
bucket private (public access prevention is enforced), never grant `allUsers`,
download only into a throwaway directory, delete local restores straight after
checking them, and keep the 30-day retention unless the ethics protocol says
otherwise. Deleting a participant's account does not remove them from backups
already taken; they age out within 30 days.

## Environment variable reference

| Variable | API | Worker | Console | Default / notes |
|---|:-:|:-:|:-:|---|
| `APP_ENV` | ✓ | | | `development`; `production` enforces secret and TLS checks |
| `PORT` | ✓ | | | `8080`; Vercel injects it |
| `DATABASE_URL` | ✓ | ✓ | | API: pooled; worker and migrations: direct; `sslmode=require` |
| `MIGRATION_DATABASE_URL` | migrate | | | overrides `DATABASE_URL` for `cmd/migrate` |
| `MIGRATION_ALLOW_INSECURE` | migrate | | | `false`; only for non-TLS remote test databases |
| `DB_MAX_CONNS` / `DB_MAX_CONN_IDLE_TIME` / `DB_MAX_CONN_LIFETIME` | ✓ | | | `4` / `30s` / `5m` |
| `JWT_SECRET` | ✓ | | | API only; 32+ random characters in production |
| `CORS_ORIGIN` | ✓ | | | exact origins, comma-separated |
| `API_INTERNAL_URL` | | | ✓ | URL of the Go API |
| `GOOGLE_CLIENT_ID`, `GOOGLE_CLIENT_SECRET` | ✓ | | | Sign in with Google; unset = off. The secret lives only in the API |
| `NEXT_PUBLIC_GOOGLE_CLIENT_ID` | | | ✓ | same client id; inlined at build time, so redeploy after setting it. Unset = no Google button. Redirect URI: `<console>/api/auth/google/callback` |
| `NEXT_PUBLIC_GOOGLE_PICKER_API_KEY` | | | ✓ | Google Drive import: browser key restricted to `picker.googleapis.com` + console referrers (`gcloud services api-keys list --project selar-research-261008`, key `selar-console-picker`). Unset = sidebar says "Google Drive import not set up" |
| `WORKER_URL` | ✓ | | | Modal `web` URL; local `http://localhost:8000` |
| `WORKER_TRIGGER_URL` | ✓ | | | optional full trigger URL override |
| `WORKER_TRIGGER_SECRET` | ✓ | ✓ | | unset = triggers off (local polling mode) |
| `WORKER_TRIGGER_TIMEOUT` | ✓ | | | `3s` |
| `STORAGE_BACKEND` | ✓ | ✓ | | `local` (default) or `s3` |
| `LOCAL_STORAGE_DIR` | ✓ | ✓ | | `/tmp/selar_uploads` |
| `S3_ENDPOINT`, `S3_BUCKET`, `S3_REGION`, `S3_ACCESS_KEY_ID`, `S3_SECRET_ACCESS_KEY`, `S3_FORCE_PATH_STYLE` | ✓ | ✓ | | required when `STORAGE_BACKEND=s3`; `S3_REGION` defaults to `auto` |
| `GEMINI_API_KEY` | | ✓ | | required for real ingestion and chat |
| `GEMINI_MULTIMODAL_EMBEDDING_MODEL`, `GEMINI_EMBEDDING_DIMENSION`, `GEMINI_TEXT_MODEL` | | ✓ | | see `.env.example` |
| `INGESTION_QUEUE_ENABLED`, `INGESTION_CONCURRENCY`, `INGESTION_POLL_SECONDS` | | local poller | | Modal forces `INGESTION_QUEUE_ENABLED=false` |
| `INGESTION_LEASE_SECONDS`, `INGESTION_WORKER_ID` | | ✓ | | `300`, hostname-pid |
| `RECAPTCHA_PROJECT_ID`, `RECAPTCHA_SITE_KEY`, `RECAPTCHA_API_KEY`, `RECAPTCHA_MIN_SCORE` | ✓ | | | unset = no reCAPTCHA check (see §6) |
| `NEXT_PUBLIC_RECAPTCHA_SITE_KEY` | | | ✓ | build time; unset = no script on `/register` |
| `CALENDAR_FEED_ENABLED`, `CALENDAR_FEED_ALLOWLIST` | ✓ | | | off / nobody: optional quiz-window calendar, see [REMINDERS.md](REMINDERS.md) |
| `PUBLIC_API_URL`, `PUBLIC_CONSOLE_URL`, `LINK_TOKEN_SECRET` | ✓ | | | origins used in signed links; link key defaults to `JWT_SECRET` |
| `NOTIFY_EMAIL_ENABLED`, `NOTIFY_EMAIL_ALLOWLIST`, `NOTIFY_SMTP_*`, `NOTIFY_EMAIL_FROM`, `NOTIFY_TIMEZONE` | ✓ | | | off: optional email notices, dry-run by default, see [REMINDERS.md](REMINDERS.md) |
| `SELAR_API_URL` | | ✓ (Modal) | | unset = notification schedule idle |

Generate secrets with `openssl rand -base64 48`. Never commit them; `.env*` files other than `.env.example` are git-ignored.

## Deploy order

1. Create the Neon database, then run `go run ./cmd/migrate` against the **direct** URL.
2. Create the bucket, key and CORS rule.
3. Create the Modal secret, then `modal deploy modal_app.py`, and note the `web` URL.
4. Create the API Vercel project with its env vars (including `WORKER_URL`), deploy, and check `/healthz`.
5. Create the console Vercel project (`API_INTERNAL_URL`) and deploy.
6. Set the API's `CORS_ORIGIN` and the bucket CORS to the console origin, then redeploy the API.

For later releases: run migrations first (they are additive), then deploy the worker, then the API, then the console.

## Post-deploy smoke checklist

Use a fresh test account and synthetic or public-domain PDFs only. Do not use participant data.

- [ ] `GET <api>/healthz` returns 200. `GET <modal-web>/health` returns 200. `POST <modal-web>/jobs/trigger` without the secret returns 401.
- [ ] `go run ./cmd/migrate -status` against production reports `0 pending`.
- [ ] Register and log in on the console. The session cookie is `Secure` and `HttpOnly`.
- [ ] **Upload PDF:** add a PDF **larger than 5 MB**. In the network tab: `upload-url` 200 (`mode: direct`), a PUT to the bucket returns 200, then `upload-complete` returns 202. The document appears as *processing*.
- [ ] Within about a minute the Modal logs show `process_job` for that job and the document becomes **ready**. If the trigger was lost, the next sweep (≤ 5 min) picks it up.
- [ ] Open the reader. The PDF renders (a 302 to a presigned GET) and figures load.
- [ ] Upload a second related PDF and wait for it to be ready. **Suggestion:** a suggested connection appears in the Connections panel.
- [ ] **Explain:** write the connection in your own words and submit it.
- [ ] **Compare:** open the side-by-side evidence and check the quoted passages and locators match both sources.
- [ ] **Decide:** confirm one suggestion and reject another, then reload. The decisions persist and appear in the graph and history.
- [ ] Ask a grounded chat question. It returns an answer with citations (API → Modal `/chat` with the secret).
- [ ] Delete a test document. It disappears, and its objects (`<doc>.pdf` or `users/.../uploads/...pdf`, plus `<doc>/assets/`) are removed from the bucket.
- [ ] A second account cannot open the first account's `/api/documents/<id>/pdf` (404).

## Rollback

- API and console: use **Promote** on the previous Vercel deployment.
- Worker: `modal app rollback selar-worker`, or redeploy the previous commit.
- Database: migrations are forward-only. Write a new corrective migration rather than editing an applied one; the runner refuses changed checksums.
