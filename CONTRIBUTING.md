# Contributing to SELAR

Thank you for your interest in contributing to SELAR. This document outlines the process for contributing code, reporting issues, and requesting features.

---

## Table of Contents

- [Code of Conduct](#code-of-conduct)
- [Getting Started](#getting-started)
- [Development Environment](#development-environment)
- [Development Workflow](#development-workflow)
- [Commit Conventions](#commit-conventions)
- [Pull Request Process](#pull-request-process)
- [CI/CD Pipeline](#cicd-pipeline)
- [Local Git Hooks](#local-git-hooks)
- [Issue Guidelines](#issue-guidelines)
- [Code Style](#code-style)

---

## Code of Conduct

This project follows a standard code of conduct. Be respectful, constructive, and professional in all interactions. Harassment, discrimination, and disruptive behavior will not be tolerated.

---

## Getting Started

1. Fork the repository on GitHub.
2. Clone your fork locally:
   ```bash
   git clone https://github.com/<your-username>/selar-dev.git
   cd selar-dev
   ```
3. Follow the setup instructions below.
4. Create a feature branch from `main`:
   ```bash
   git checkout -b feat/your-feature-name
   ```

---

## Development Environment

### Prerequisites

| Tool | Version | Purpose |
|------|---------|---------|
| Go | 1.22+ | API server |
| Node.js | 20+ | Console frontend |
| pnpm | 10+ | Package manager |
| Python | 3.11+ | AI worker |
| PostgreSQL | 16 | Database (with pgvector extension) |
| Lefthook | latest | Local git hooks |

See [README.md](README.md#prerequisites) for platform-specific installation instructions (macOS, Windows, Linux).

### Initial Setup

```bash
# 1. Copy and configure environment
cp .env.example .env.development
# Edit .env.development and set your GEMINI_API_KEY

# 2. Set up the database
createdb selar
psql -d selar -c "CREATE EXTENSION IF NOT EXISTS vector;"
psql -d selar -c "CREATE EXTENSION IF NOT EXISTS pgcrypto;"
psql -d selar -f services/selar-api/internal/store/migrations/001_init.sql
psql -d selar -f services/selar-api/internal/store/migrations/002_add_summary.sql
psql -d selar -f services/selar-api/internal/store/migrations/003_runtime_mental_model.sql
psql -d selar -f services/selar-api/internal/store/migrations/004_adaptive_chat.sql

# 3. Install dependencies
cd services/selar-console && pnpm install && cd ../..
cd services/selar-worker && pip install -r requirements.txt && cd ../..

# 4. Install local git hooks
brew install lefthook   # or: go install github.com/evilmartians/lefthook@latest
lefthook install
```

### Running the Services

Open three terminals:

```bash
# Terminal 1 — Go API (port 8080)
cd services/selar-api && go run cmd/server/main.go

# Terminal 2 — Python Worker (port 8000)
cd services/selar-worker && python3 -m uvicorn main:app --reload --port 8000

# Terminal 3 — Next.js Console (port 3000)
cd services/selar-console && pnpm dev
```

The application will be available at [http://localhost:3000](http://localhost:3000).

---

## Development Workflow

SELAR is a monorepo with three services. When working on a change:

1. Identify which service(s) your change affects: `selar-api`, `selar-console`, or `selar-worker`.
2. Make your changes in the relevant service directory under `services/`.
3. Run the build for each affected service before committing:
   - **Go API:** `cd services/selar-api && go build ./...`
   - **Next.js Console:** `cd services/selar-console && pnpm build`
   - **Python Worker:** `cd services/selar-worker && python3 -m py_compile main.py`
4. Test your changes locally by running all three services.
5. Commit using [Conventional Commits](#commit-conventions). Lefthook will automatically run pre-commit checks.

---

## Commit Conventions

We use [Conventional Commits](https://www.conventionalcommits.org/) with service-scoped prefixes:

```
type(scope): description
```

**Types:** `feat`, `fix`, `docs`, `chore`, `refactor`, `test`, `perf`

**Scopes:** `api`, `console`, `worker`

Examples:
```
feat(console): add document search to sidebar
fix(api): handle null bboxes in ListSuggestions
docs: update README with Windows setup instructions
chore(worker): upgrade google-genai to 1.80
```

Keep the subject line under 72 characters. Use the commit body for additional context when necessary.

---

## Pull Request Process

1. Ensure your branch is up to date with `main`:
   ```bash
   git fetch origin
   git rebase origin/main
   ```
2. Run all builds and confirm they pass with zero errors.
3. Push your branch and open a pull request against `main`.
4. Fill out the pull request template completely.
5. Link any related issues using `Closes #123` or `Refs #123`.
6. Wait for CI checks and Claude Code review to complete (see [CI/CD Pipeline](#cicd-pipeline) below).
7. A maintainer will review your PR. Address any feedback with additional commits.
8. Once approved with all checks passing, a maintainer will merge the PR using squash-merge.

### Branch Protection Rules

The `main` branch has the following protections:

- All three CI status checks must pass (Go build, Next.js build, Python check).
- At least 1 approving review is required.
- Stale reviews are dismissed when new commits are pushed.
- Linear history is enforced (squash or rebase only, no merge commits).
- Force pushes and branch deletion are blocked.

---

## CI/CD Pipeline

The following GitHub Actions workflows run automatically:

### CI (`ci.yml`)

Triggers on every push to `main` and every PR. Runs three parallel jobs:

| Job | What it checks |
|-----|----------------|
| **Build Go API** | `go mod download`, `go build ./...`, `go vet ./...` |
| **Build Next.js Console** | `pnpm install --frozen-lockfile`, `pnpm build` |
| **Check Python Worker** | `pip install -r requirements.txt`, `py_compile`, import check |

All three must pass before a PR can be merged.

### Coverage (`coverage.yml`)

Triggers alongside CI. Runs Go and TypeScript test suites with coverage instrumentation and uploads reports to [Codecov](https://codecov.io). Coverage diffs are posted as comments on PRs once tests are present.

Note: There are currently no unit tests in the codebase. Coverage reporting will activate once tests are added.

### Claude Code Review (`claude-review.yml`)

Triggers on PR open/sync/reopen. Claude reviews the diff for correctness, security issues, and code quality. Findings are posted as a sticky comment grouped by severity (Critical, Warning, Suggestion).

Also responds to `@claude` mentions in PR comments for on-demand clarifications.

Note: This workflow skips fork PRs since secrets are unavailable to external contributors.

### Release (`release.yml`)

Triggers when a semver tag (`v*.*.*`) is pushed. Builds multi-arch Docker images for `selar-api` and `selar-console`, pushes them to GitHub Container Registry (GHCR), and creates a GitHub Release with an auto-generated changelog via [git-cliff](https://git-cliff.org/).

To create a release:
```bash
git tag v0.2.0
git push origin v0.2.0
```

---

## Local Git Hooks

We use [Lefthook](https://github.com/evilmartians/lefthook) for local pre-commit and pre-push hooks. These run the same checks as CI but locally, catching errors before they reach the remote.

### What runs

| Hook | Commands | Speed |
|------|----------|-------|
| `pre-commit` | `go vet`, `py_compile` | Fast (<10s) |
| `pre-push` | `go build`, `pnpm build`, Python import check | Full verification |

### Setup

```bash
# macOS
brew install lefthook

# Any platform with Go
go install github.com/evilmartians/lefthook@latest

# Activate hooks (run once after cloning)
lefthook install
```

### Bypassing hooks

In emergencies, you can skip hooks:
```bash
git commit --no-verify
git push --no-verify
```

---

## Issue Guidelines

- Use the provided issue templates when filing bugs or requesting features.
- Search existing issues before creating a new one to avoid duplicates.
- Include reproduction steps, expected behavior, and actual behavior for bugs.
- For feature requests, describe the use case and proposed solution.
- Use the applicable `service:` label (`service: api`, `service: console`, `service: worker`).

---

## Code Style

### Go (selar-api)

- Follow standard `gofmt` formatting.
- Use meaningful variable names. Avoid single-letter names outside of loop indices.
- Keep functions focused and under 50 lines where possible.
- Document exported types and functions.

### TypeScript / React (selar-console)

- Use functional components with hooks.
- Prefer inline styles or CSS class names from `globals.css` — avoid introducing new CSS frameworks.
- Type all props and API responses explicitly.
- Use `clientFetch` for client-side API calls and `serverFetch` for server components.

### Python (selar-worker)

- Follow PEP 8 conventions.
- Use type hints for function signatures.
- Keep the ingestion pipeline stages clearly separated with comments.
- Handle all external API errors gracefully with try/except blocks.

---

## Questions

If you have questions about contributing, open a [Discussion](https://github.com/Kavirubc/selar-dev/discussions) on the repository.
