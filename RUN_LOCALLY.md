# SELAR — Run Locally (3 Terminals)

✅ **Setup complete!** Postgres is running in Docker on port **5433**.

## Start Each Service in a New Terminal

All services have **hot reload** enabled for development.

---

### Terminal 1: Go API (http://localhost:8080)

```bash
cd services/selar-api
go run ./cmd/server/main.go
```

Watch for: `[chi] listening on :8080`

---

### Terminal 2: Next.js Console (http://localhost:3000)

```bash
cd services/selar-console
pnpm dev
```

Watch for: `▲ Next.js 16 ... ready on http://localhost:3000`

---

### Terminal 3: Python Worker (http://localhost:8000)

```bash
cd services/selar-worker
source .venv/bin/activate
python -m uvicorn main:app --reload --port 8000
```

Or directly without activate:

```bash
.venv/bin/python -m uvicorn main:app --reload --port 8000
```

Watch for: `INFO: Application startup complete`

---

## Verify Everything Works

```bash
# API health
curl http://localhost:8080/healthz

# Console is up
curl http://localhost:3000

# Worker API docs
curl http://localhost:8000/docs
```

Then open **http://localhost:3000** in a browser to register and start using SELAR!

---

## Notes

- **Postgres** is running in Docker on `localhost:5433` (not 5432, to avoid conflicts)
- Environment variables are already set in `.env` and `.env.development`
- Hot reload is enabled on all services — just save and refresh
- Python dependencies are isolated in `.venv` (created with `uv venv`)
- To stop Postgres: `docker compose down`
- To reset database: `docker compose down -v && docker compose up postgres -d`
