# Local quickstart

[README](../README.md) · [User guide](user-guide.md) · [Hosted deployment](DEPLOYMENT.md)

This is a native, single-computer **development** setup. First verify registration and an empty library without a model key; enable cloud AI separately. It creates a new database cluster, not a connection to an existing or hosted database. Commands use a POSIX shell on macOS/Linux; native Windows instructions are not verified.

## Prerequisites

Install these tools before starting; this guide does not install system packages or change an existing database service.

| Tool | Requirement / source of truth |
|---|---|
| Git | `kaviru2/selar-dev` is public; HTTPS cloning does not require repository access credentials. |
| Go | 1.25.5 or newer, as declared in [go.mod](../services/selar-api/go.mod). |
| Node.js and npm/npx | Use a version compatible with the console dependencies. This smoke test used Node 25.5.0; the [console Dockerfile](../services/selar-console/Dockerfile) uses Node 22. |
| pnpm | 11.5.1, pinned in [package.json](../services/selar-console/package.json). Commands below use `npx` to select that version without replacing your global pnpm. |
| Python | Python 3.11 with `venv` and pip for this tested path. Use the `python3.11` executable, not an older system `python3`. |
| PostgreSQL + pgvector | `initdb`, `pg_ctl`, `createdb`, `psql` on PATH, with pgvector installed for that PostgreSQL major version. Native smoke test: PostgreSQL 17.11. Compose separately specifies PostgreSQL 16. |

Dependency installation needs internet access and downloads executable packages. Worker requirements are currently unpinned, so a future install is not a byte-for-byte reproduction of this smoke test.

## 1. Clone and reserve a local workspace

Run in a directory where you want the new checkout:

```sh
umask 022
git clone https://github.com/kaviru2/selar-dev.git
cd selar-dev
mkdir -p .local/pgsocket .local/uploads
```

Keep this checkout separate from any running instance. The example uses ports **56446** (database), **18086** (API), **18006** (worker), **13006** (console). Confirm they are free before continuing; if occupied, choose other ports and update every matching URL/command. Never stop an unrelated service to make room.

`.local/` contains disposable local test state, including database records and uploaded files. Do not commit it, virtual environments, logs or any `.env` files. Use a new checkout without other `.env` files or inherited production/integration variables; do not copy credentials from a deployed instance.

## 2. Configure the native services

In your editor, create **`.env.development` at the repository root** containing:

```dotenv
APP_ENV=development
DATABASE_URL='postgres://selar@127.0.0.1:56446/selar?sslmode=disable'
JWT_SECRET=local-development-only-change-before-deployment
PORT=18086
CORS_ORIGIN=http://localhost:13006
WORKER_URL=http://127.0.0.1:18006
API_INTERNAL_URL=http://127.0.0.1:18086
NEXT_PUBLIC_API_URL=http://localhost:18086
STORAGE_BACKEND=local
INGESTION_QUEUE_ENABLED=true
INGESTION_CONCURRENCY=1
GEMINI_API_KEY=
NOTIFY_EMAIL_ENABLED=false
CALENDAR_FEED_ENABLED=false
ANALYTICS_ENABLED=false
```

These are disposable local values, **not production credentials**. The [full environment example](../.env.example) mixes container and local URLs; copying it unchanged is not sufficient for this native path. In particular, `postgres` and `selar-api` container hostnames do not resolve as native localhost services.

From the repository root, load your own trusted file and set one shared, absolute upload directory:

```sh
set -a
. ./.env.development
set +a
export LOCAL_STORAGE_DIR="$PWD/.local/uploads"
```

Repeat that block **in each terminal**, from the repository root, before starting a service. The console does not automatically load the root environment file. Explicit exports also take precedence over the API/worker's dotenv loaders. Never source an untrusted environment file.

## 3. Create the isolated database and migrate

Run once from the repository root, in the configured terminal:

```sh
initdb -D .local/pgdata -U selar -A trust --no-locale
LC_ALL=C pg_ctl -D .local/pgdata -l .local/postgres.log \
  -o "-h 127.0.0.1 -p 56446 -k $PWD/.local/pgsocket" start
createdb -h 127.0.0.1 -p 56446 -U selar selar
(cd services/selar-api && go run ./cmd/migrate)
```

`trust` authentication is used **only for this disposable loopback-bound cluster on a trusted single-user development computer**. Other local users can connect without a password. Do not use this configuration on a shared computer or in production. The API itself listens on all interfaces; use a trusted development machine/firewall and do not expose its port publicly.

The migration command creates the required extensions/schema and tracks migration checksums. The merged learning-loop baseline contains 22 migrations, including `021_document_practice.sql` and `022_practice_schedule.sql`. The original smoke run below applied 20; after updating an existing checkout, stop its services, rerun migrations against your isolated database, rebuild the API and restart all services. Re-running the migration command should report no pending changes. Do not re-run `initdb` or `createdb` on an existing cluster. For an ordinary restart, use only the `pg_ctl … start` command above.

## 4. Install dependencies

From the repository root:

```sh
python3.11 -m venv .local/venv
.local/venv/bin/python -m pip install -r services/selar-worker/requirements.txt
(cd services/selar-console && npx --yes pnpm@11.5.1 install --frozen-lockfile)
(cd services/selar-api && go build -o ../../.local/selar-api ./cmd/server)
```

The Go build also fetches its module dependencies. Keep the lockfile unchanged. Rebuild the API after changing its source.

## 5. Run the three services

Open three terminals at the repository root. In **each**, first run the environment-loading block in step 2. Then run the appropriate block below and leave it running.

**API**

```sh
(cd services/selar-api && ../../.local/selar-api)
```

**Worker**

```sh
(cd services/selar-worker && ../../.local/venv/bin/python -m uvicorn main:app \
  --host 127.0.0.1 --port 18006)
```

**Console**

```sh
(cd services/selar-console && npx --yes pnpm@11.5.1 dev \
  --hostname 127.0.0.1 --port 13006)
```

A warning that `GEMINI_API_KEY` is unset is expected for the no-model-call smoke test. Queue polling is explicitly enabled, but do not add documents until you intend to use cloud AI.

## 6. Check the installation

In another terminal:

```sh
curl --fail http://127.0.0.1:18086/healthz
curl --fail http://127.0.0.1:18006/health
curl --fail --output /dev/null http://127.0.0.1:13006/login
```

Open `http://localhost:13006`, register a disposable test account, and confirm that **Library** loads with no readings. Use that same console hostname consistently when signing in. This tests the console-to-API-to-database path; a health response alone does not establish AI readiness.

**To try real ingestion/chat/practice:** set your own `GEMINI_API_KEY` in the local environment file, reload the environment and restart the worker. Review costs and data handling first, then add one small non-sensitive reading using the [user guide](user-guide.md). The worker's current model defaults are `gemini-embedding-2`, dimension `3072`, and `gemini-2.5-flash-lite` with thinking off (see `services/selar-worker/genai_config.py`); access and quota must be verified for your provider account. Changing embedding dimensions/models is not a drop-in workaround for existing stored vectors. This guide has not validated a live model call.

## Stop without deleting your data

Press Ctrl+C in each service terminal, then from the repository root:

```sh
pg_ctl -D .local/pgdata -m fast stop
```

The database and uploads remain under `.local/` for your next session. Do not use `make clean` or `make db-reset` as routine shutdown commands: those targets are destructive and belong to a different setup path.

## Troubleshooting

| Symptom | Check |
|---|---|
| Clone denied | Confirm the exact public HTTPS URL above and check your network/proxy or Git credential configuration; do not switch to a similarly named repository. |
| `vector` extension unavailable | Install pgvector for the PostgreSQL major version used by `initdb`, then rerun migrations. Installing PostgreSQL alone is insufficient. |
| Database fails during startup on macOS | Read `.local/postgres.log`; the tested machine needed `LC_ALL=C` as shown above. Keep the checkout path reasonably short for the Unix socket path. |
| Connection refused / login fails | Confirm database and API ports, migrations, and `API_INTERNAL_URL` in the console terminal. The root dotenv file is not loaded by Next.js automatically. |
| Jobs remain queued | Confirm the worker uses the same `DATABASE_URL`, `INGESTION_QUEUE_ENABLED=true`, and absolute `LOCAL_STORAGE_DIR` as the API. Do not point an ad-hoc worker at another instance's queue. |
| Ingestion/chat fails with no key or quota error | The smoke setup has no cloud credentials. Configure a permitted model/key and check provider quota before retrying; a healthy worker is not proof of available model quota. |
| Google/Drive/email features absent | They require separate operator configuration. This smoke setup deliberately leaves them disabled. |

## Costs and data boundary

The repository is public and has an [Apache-2.0 license](../LICENSE). That is not a promise of free hosted access. Local software does not remove hardware, electricity or storage costs, and hosted infrastructure has its own billing/quotas.

The current worker uses **Google Gemini** for model-backed processing, including practice generation, a separate support check and answer grading. Opening a reading can request generation before you answer. These calls may consume quota, and automated checking does not validate real-world accuracy. Self-hosted does not mean offline: content used for embeddings and generation can leave your machine for that provider. Review the provider account's current terms, billing, limits and data policy before supplying a key. No fixed price or sufficient free quota is promised here. There is no working Ollama/local-model setup verified by this guide.

## Verification record and alternatives

Verified on **October 9, 2026**, against application baseline `f80d5233fc9ca5e6737bc1b285ec95a68a518e8a`, using a fresh isolated cluster and synthetic account only:

- macOS, Go 1.25.6, Node 25.5.0, pnpm 11.5.1, Python 3.11, PostgreSQL 17.11 with pgvector.
- Worker dependency installation, frozen console dependency installation and Go API build succeeded.
- All 20 migrations applied; API `/healthz`, worker `/health` and console `/login` returned HTTP 200.
- Registration through the console returned HTTP 201; authenticated `/api/users/me`, `/api/documents` and `/library` returned HTTP 200.
- No production/shared database, real account, email transport or cloud model call was used.

**Not verified by this pass:** live AI ingestion/chat, OAuth/Drive, production build/deployment, visual browser interactions, Linux/Windows, or Docker. Docker was unavailable on the test machine. The checked-in [Compose configuration](../docker-compose.yml) is a separate option, not the tested path above: it uses fixed container names, different port defaults, container networking, shared volumes and a migration service. A different Compose project name alone does not isolate those fixed names. Review it before running beside another instance. Use [DEPLOYMENT.md](DEPLOYMENT.md) for hosting, not this disposable development configuration.
