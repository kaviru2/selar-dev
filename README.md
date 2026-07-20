# SELAR — Semantic Linking for Active Retention

[![CI](https://github.com/Kavirubc/selar-dev/actions/workflows/ci.yml/badge.svg)](https://github.com/Kavirubc/selar-dev/actions/workflows/ci.yml)
[![Coverage](https://codecov.io/gh/Kavirubc/selar-dev/graph/badge.svg)](https://codecov.io/gh/Kavirubc/selar-dev)
[![License](https://img.shields.io/github/license/Kavirubc/selar-dev?color=blue)](LICENSE)
[![Release](https://img.shields.io/github/v/release/Kavirubc/selar-dev?color=green)](https://github.com/Kavirubc/selar-dev/releases)
[![Last Commit](https://img.shields.io/github/last-commit/Kavirubc/selar-dev)](https://github.com/Kavirubc/selar-dev/commits/main)
[![Issues](https://img.shields.io/github/issues/Kavirubc/selar-dev)](https://github.com/Kavirubc/selar-dev/issues)
[![PRs Welcome](https://img.shields.io/badge/PRs-welcome-brightgreen.svg)](CONTRIBUTING.md)
[![Go Report Card](https://goreportcard.com/badge/github.com/Kavirubc/selar-dev)](https://goreportcard.com/report/github.com/Kavirubc/selar-dev)
[![Dependency Review](https://github.com/Kavirubc/selar-dev/actions/workflows/dependency-review.yml/badge.svg)](https://github.com/Kavirubc/selar-dev/actions/workflows/dependency-review.yml)

SELAR is a web-based PDF reader that discovers semantic connections across your document library using AI-powered vector embeddings. As you read, the system surfaces candidate links between passages in different papers. Its grounded research chat combines vector, lexical, graph, learner, evidence-confidence, and recency signals and cites the source passages behind each answer. A deterministic reducer reinforces cited concepts and adds auditable candidate relationships; repeated independent evidence promotes them without allowing an LLM to write to the graph. Corrections supersede conversational evidence, human decisions govern promotion and rejection, chat-only edges decay, and retained events can replay the current graph and learner projections.

Built for academic research. Designed for students and researchers who read across multiple papers and want to strengthen long-term comprehension.

---

## Table of Contents

- [Architecture](#architecture)
- [Prerequisites](#prerequisites)
- [Local Development Setup](#local-development-setup)
- [Docker Setup](#docker-setup)
- [Environment Variables](#environment-variables)
- [Project Structure](#project-structure)
- [Usage](#usage)
- [Deployment](#deployment)
- [Contributing](#contributing)
- [License](#license)

---

## Architecture

SELAR is a monorepo containing three services:

| Service | Stack | Port | Description |
|---------|-------|------|-------------|
| `selar-api` | Go 1.22+, Chi router | 8080 | REST API, JWT auth, document CRUD, concept graph |
| `selar-console` | Next.js 16, TypeScript | 3000 | Frontend: PDF reader, grounded chat, matches panel, knowledge graph |
| `selar-worker` | Python 3.11+, FastAPI | 8000 | AI ingestion and deterministic hybrid chat retrieval |

**Database:** PostgreSQL 16 with the [pgvector](https://github.com/pgvector/pgvector) extension for 3072-dimensional embedding storage and approximate nearest-neighbor search.

**AI Provider:** Google Gemini API (`gemini-embedding-2` for shared text/image embeddings, `gemini-3-flash-preview` for text generation).

```
User --> selar-console (Next.js :3000)
              |
              v
         selar-api (Go :8080) <---> PostgreSQL + pgvector (:5432)
              |
              v
        selar-worker (Python :8000) <---> Gemini API
```

---

## Prerequisites

### All platforms

- [Git](https://git-scm.com/)
- A [Google AI Studio](https://aistudio.google.com/) API key (free tier is sufficient)

### macOS

```bash
# Install Homebrew if not present
/bin/bash -c "$(curl -fsSL https://raw.githubusercontent.com/Homebrew/install/HEAD/install.sh)"

# Required
brew install go node postgresql@16

# Install pnpm
npm install -g pnpm

# Install Python 3.11+
brew install python@3.11

# Start Postgres
brew services start postgresql@16
```

### Windows

1. Install [Go](https://go.dev/dl/) (1.22 or later).
2. Install [Node.js](https://nodejs.org/) (20 or later) and run `npm install -g pnpm`.
3. Install [Python](https://www.python.org/downloads/) (3.11 or later). Ensure "Add to PATH" is checked.
4. Install [PostgreSQL 16](https://www.enterprisedb.com/downloads/postgres-postgresql-downloads) with the pgvector extension, or use Docker (see below).
5. Install [Git for Windows](https://gitforwindows.org/).

### Linux (Debian/Ubuntu)

```bash
sudo apt update && sudo apt install -y git golang-go nodejs npm python3 python3-pip python3-venv postgresql postgresql-contrib
npm install -g pnpm

# Install pgvector extension
sudo apt install -y postgresql-16-pgvector
```

---

## Local Development Setup

### 1. Clone and configure

```bash
git clone https://github.com/Kavirubc/selar-dev.git
cd selar-dev
cp .env.example .env.development
```

Edit `.env.development` and set your `GEMINI_API_KEY`. All other defaults work for local development.

### 2. Set up the database

Create the database and install the pgvector extension:

```bash
# macOS / Linux
createdb selar
psql -d selar -c "CREATE EXTENSION IF NOT EXISTS vector;"
psql -d selar -c "CREATE EXTENSION IF NOT EXISTS pgcrypto;"
psql -d selar -f services/selar-api/internal/store/migrations/001_init.sql
psql -d selar -f services/selar-api/internal/store/migrations/002_add_summary.sql
psql -d selar -f services/selar-api/internal/store/migrations/003_runtime_mental_model.sql
psql -d selar -f services/selar-api/internal/store/migrations/004_adaptive_chat.sql
psql -d selar -f services/selar-api/internal/store/migrations/005_chat_graph_reducer.sql
psql -d selar -f services/selar-api/internal/store/migrations/006_graph_governance.sql
psql -d selar -f services/selar-api/internal/store/migrations/007_multimodal_sources.sql
psql -d selar -f services/selar-api/internal/store/migrations/008_grounded_chat_concepts.sql
psql -d selar -f services/selar-api/internal/store/migrations/009_durable_ingestion_jobs.sql
```

On Windows, use `psql` from the PostgreSQL installation directory, or pgAdmin.

If your local Postgres uses different credentials, update `DATABASE_URL` in `.env.development` accordingly.

To verify deterministic replay for one user without changing projections:

```bash
make replay USER_ID=<uuid>
```

To rebuild the graph and learner projections from retained events and evidence:

```bash
make replay USER_ID=<uuid> APPLY=1
```

### 3. Start the services

Open three terminal windows and run one command in each:

**Terminal 1 — API (Go)**
```bash
cd services/selar-api
go run cmd/server/main.go
```

**Terminal 2 — Worker (Python)**
```bash
cd services/selar-worker
pip install -r requirements.txt
python3 -m uvicorn main:app --reload --port 8000
```

**Terminal 3 — Console (Next.js)**
```bash
cd services/selar-console
pnpm install
pnpm dev
```

The application will be available at [http://localhost:3000](http://localhost:3000).

---

## Docker Setup

If you prefer Docker over local installation:

```bash
cp .env.example .env
docker compose up --build
```

This starts Postgres (with pgvector and initialization migrations), the Python worker, the Go API, and the Next.js console. Run `make migrate` after pulling new migrations into an existing database volume.

### Adding and managing research sources

The Library **Add content** flow accepts a public article/blog URL, pasted text or Markdown, and PDFs. SELAR stores the origin separately from each immutable ingestion snapshot and records extractor/model provenance in an ingestion run. Web sources can be refreshed or archived from **Managed sources**; refreshing creates a new snapshot rather than silently rewriting prior research material.

PDF batches accept up to 10 files, with a 50 MB limit per file. The console transfers files sequentially, the API validates the PDF signature, caps each request, and spills multipart data above 8 MB to temporary disk. Accepted PDF, web, and text work is stored in a PostgreSQL-backed queue before the API responds. Workers claim jobs with expiring leases, renew active leases, retry transient failures up to three times, and recover abandoned work after a restart. The worker processes one document at a time by default (`INGESTION_CONCURRENCY=1`) to keep several large PDFs from multiplying peak memory use. Queue polling is disabled for ad-hoc local worker processes unless `INGESTION_QUEUE_ENABLED=true`; Docker Compose enables it on the worker service so a stray local `uvicorn` process cannot consume jobs without access to the shared upload volume.

Grounded chat can add a missing concept candidate when a bounded phrase from the question appears directly in cited library evidence. Graph evidence is applied only after the user marks an answer helpful; unhelpful feedback or a correction retracts that message's adaptive evidence. The reducer does not mine generated answers or feedback comments for facts, use embedding-only concept bindings, or manufacture relationships from concepts that merely co-occur in retrieved passages.

The Managed sources panel can copy a small **Save to SELAR** bookmarklet. The bookmarklet only opens the authenticated SELAR add screen with the current URL—no API key or page contents are stored in the bookmark. Public pages are fetched server-side with redirect, size, content-type, robots, and private-network protections. It does not bypass authentication or paywalls.

Because issue #5 establishes a new development schema and a new embedding space, reset a pre-issue-5 Docker database once:

```bash
make db-reset
```

The research hypotheses, evaluation conditions, provenance requirements, and scholarly references are maintained in [`dev_artifacts/ISSUE_5_RESEARCH_PROTOCOL.md`](dev_artifacts/ISSUE_5_RESEARCH_PROTOCOL.md).

See the [Makefile](Makefile) for convenience targets:

| Command | Description |
|---------|-------------|
| `make dev` | Start all Docker services |
| `make down` | Stop all services |
| `make logs` | Follow service logs |
| `make db-reset` | Drop and recreate the database |
| `make clean` | Remove containers, volumes, and build artifacts |

---

## Environment Variables

Copy `.env.example` to `.env.development` (local) or `.env` (Docker) and configure:

| Variable | Required | Default | Description |
|----------|----------|---------|-------------|
| `GEMINI_API_KEY` | Yes | — | Google AI Studio API key |
| `GEMINI_MULTIMODAL_EMBEDDING_MODEL` | No | `gemini-embedding-2` | Shared text/image embedding model identifier |
| `INGESTION_QUEUE_ENABLED` | No | `false` | Enables durable queue polling; Docker Compose sets this to `true` for its shared-volume worker |
| `INGESTION_CONCURRENCY` | No | `1` | Maximum documents processed simultaneously by each worker process; `1` is safest for large PDFs |
| `GEMINI_EMBEDDING_DIMENSION` | No | `3072` | Shared pgvector embedding dimension |
| `GEMINI_TEXT_MODEL` | No | `models/gemini-3-flash-preview` | Text generation model identifier |
| `DATABASE_URL` | No | `postgres://selar:selar_dev@localhost:5432/selar?sslmode=disable` | Postgres connection string |
| `JWT_SECRET` | No | `dev-secret-change-in-production` | Secret for JWT token signing |
| `PORT` | No | `8080` | Go API listen port |
| `POSTGRES_USER` | No | `selar` | Postgres username |
| `POSTGRES_PASSWORD` | No | `selar_dev` | Postgres password |
| `POSTGRES_DB` | No | `selar` | Postgres database name |

---

## Project Structure

```
selar-dev/
├── services/
│   ├── selar-api/               # Go REST API
│   │   ├── cmd/server/          # Entry point
│   │   └── internal/
│   │       ├── handler/         # HTTP handlers
│   │       ├── middleware/      # JWT auth middleware
│   │       ├── model/           # Domain types
│   │       └── store/           # Postgres queries + migrations
│   ├── selar-console/           # Next.js frontend
│   │   ├── app/                 # App Router pages
│   │   │   ├── (app)/           # Authenticated views (reader, library, graph, etc.)
│   │   │   └── (auth)/          # Login / register
│   │   ├── components/          # React components
│   │   └── lib/                 # API client, auth, context
│   └── selar-worker/            # Python AI ingestion worker
│       └── main.py              # FastAPI app with ingestion pipeline
├── design_handoff_selar/        # Design reference files (prototypes)
├── docker-compose.yml           # Docker orchestration
├── Makefile                     # Development convenience targets
├── .env.example                 # Environment variable template
└── .env.development             # Local environment (git-ignored)
```

---

## Usage

1. Open [http://localhost:3000](http://localhost:3000) and register an account.
2. Navigate to **Library** and upload one or more PDF documents.
3. Wait for the status to change from "processing" to "ready" (the worker handles chunking, embedding, and link generation).
4. Open a document in the **Reader**. The right-side Matches panel shows AI-discovered semantic links to other documents.
5. **Confirm** or **Reject** each match. Every interaction is a retrieval-practice event.
6. Visit the **Graph** tab to explore your knowledge graph — concepts extracted from your papers with force-directed visualization.

---

## Deployment

### Production considerations

- Set `JWT_SECRET` to a cryptographically random string (at minimum 32 characters).
- Use a managed Postgres instance with the pgvector extension enabled (e.g., Supabase, Neon, or AWS RDS with pgvector).
- Run the Go API behind a reverse proxy (nginx, Caddy) with TLS.
- Build the Next.js console for production: `pnpm build && pnpm start`.
- Run the Python worker as a long-lived process with a process manager (systemd, PM2, or container orchestration).
- Set `CORS_ORIGIN` to your production domain.

### Docker production

```bash
docker compose -f docker-compose.yml up --build -d
```

Ensure all environment variables are set via `.env` or your orchestration platform's secret management.

---

## Contributing

See [CONTRIBUTING.md](CONTRIBUTING.md) for guidelines on submitting issues, feature requests, and pull requests.

---

## License

This project is licensed under the Apache License 2.0. See [LICENSE](LICENSE) for the full text.
