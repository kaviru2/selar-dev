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

> The repository contains configuration only. Creating accounts, projects,
> buckets, secrets and running deploys are manual steps for the operator.

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
