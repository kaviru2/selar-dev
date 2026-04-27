# Contributing to SELAR

Thank you for your interest in contributing to SELAR. This document outlines the process for contributing code, reporting issues, and requesting features.

---

## Table of Contents

- [Code of Conduct](#code-of-conduct)
- [Getting Started](#getting-started)
- [Development Workflow](#development-workflow)
- [Commit Conventions](#commit-conventions)
- [Pull Request Process](#pull-request-process)
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
3. Follow the setup instructions in [README.md](README.md#local-development-setup).
4. Create a feature branch from `main`:
   ```bash
   git checkout -b feat/your-feature-name
   ```

---

## Development Workflow

SELAR is a monorepo with three services. When working on a change:

1. Identify which service(s) your change affects: `selar-api`, `selar-console`, or `selar-worker`.
2. Make your changes in the relevant service directory under `services/`.
3. Run the build for each affected service before committing:
   - **Go API:** `cd services/selar-api && go build ./...`
   - **Next.js Console:** `cd services/selar-console && pnpm build`
   - **Python Worker:** `cd services/selar-worker && python3 -m py_compile main.py`
4. Test your changes locally by running all three services (see README).

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
6. A maintainer will review your PR. Address any feedback with additional commits.
7. Once approved, a maintainer will merge the PR using squash-merge.

### PR requirements

- All CI checks must pass (Go build, Next.js build, Python syntax check).
- No unrelated changes bundled into the PR.
- New features should include relevant documentation updates.
- Breaking changes must be clearly documented in the PR description.

---

## Issue Guidelines

- Use the provided issue templates when filing bugs or requesting features.
- Search existing issues before creating a new one to avoid duplicates.
- Include reproduction steps, expected behavior, and actual behavior for bugs.
- For feature requests, describe the use case and proposed solution.

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
