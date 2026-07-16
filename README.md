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

SELAR is a web-based PDF reader that discovers semantic connections across your document library using AI-powered vector embeddings. As you read, the system surfaces candidate links between passages in different papers. You confirm, reject, or relabel each suggestion — every interaction is a retrieval-practice event grounded in cognitive science research on active retention.

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
| `selar-console` | Next.js 16, TypeScript | 3000 | Frontend: PDF reader, matches panel, knowledge graph |
| `selar-worker` | Python 3.11+, FastAPI | 8000 | AI ingestion pipeline: chunking, embedding, link generation |

**Database:** PostgreSQL 16 with the [pgvector](https://github.com/pgvector/pgvector) extension for 3072-dimensional embedding storage and approximate nearest-neighbor search.

**AI Provider:** Google Gemini API (`gemini-embedding-001` for embeddings, `gemini-3-flash-preview` for text generation).

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
```

On Windows, use `psql` from the PostgreSQL installation directory, or pgAdmin.

If your local Postgres uses different credentials, update `DATABASE_URL` in `.env.development` accordingly.

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

This starts Postgres (with pgvector and auto-migration), the Go API, and the Next.js console. The worker is not included in the compose file and must be run separately.

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
| `GEMINI_EMBEDDING_MODEL` | No | `models/gemini-embedding-001` | Embedding model identifier |
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
