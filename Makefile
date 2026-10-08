# SELAR — Development Makefile
# Convenience targets for local development with Docker Compose.

.PHONY: dev down logs build migrate migrate-url replay clean api-dev console-dev \
	deploy deploy-dry-run deploy-migrate deploy-api deploy-console deploy-worker \
	deploy-smoke deploy-env-sync deploy-lint

# Start all services in development mode
dev:
	docker compose up --build

# Start services in detached mode
dev-d:
	docker compose up --build -d

# Stop all services
down:
	docker compose down

# View logs (follow mode)
logs:
	docker compose logs -f

# Build all images without starting
build:
	docker compose build

# Apply pending migrations (checksum-tracked, adopts initdb-created databases)
migrate:
	docker compose run --rm --build selar-migrate

# Apply migrations to any DATABASE_URL / MIGRATION_DATABASE_URL (e.g. Neon direct URL)
migrate-url:
	cd services/selar-api && go run ./cmd/migrate

# Verify deterministic projections. Use APPLY=1 to rebuild them.
replay:
	cd services/selar-api && go run ./cmd/replay -user $(USER_ID) $(if $(APPLY),-apply,)

# Run only the Go API locally (requires local Postgres)
api-dev:
	cd services/selar-api && go run ./cmd/server

# Run only the Next.js console locally
console-dev:
	cd services/selar-console && pnpm dev

# Reset database (destructive!)
db-reset:
	docker compose down -v
	docker compose up postgres -d
	@echo "Waiting for postgres..."
	@sleep 3
	@echo "Database reset complete."

# Clean everything
clean:
	docker compose down -v --rmi all
	rm -rf services/selar-console/.next
	rm -rf services/selar-console/node_modules

# ---- Production deploy (see docs/DEPLOYMENT.md "One-command deploy") ----
# Pass extra flags with ARGS, e.g. make deploy ARGS="--skip-tests --yes"
DEPLOY := scripts/deploy

deploy:
	$(DEPLOY)/deploy-all.sh $(ARGS)

deploy-dry-run:
	$(DEPLOY)/deploy-all.sh --dry-run $(ARGS)

deploy-migrate:
	$(DEPLOY)/migrate.sh $(ARGS)

deploy-api:
	$(DEPLOY)/deploy-api.sh $(ARGS)

deploy-console:
	$(DEPLOY)/deploy-console.sh $(ARGS)

deploy-worker:
	$(DEPLOY)/deploy-worker.sh $(ARGS)

deploy-smoke:
	$(DEPLOY)/smoke.sh $(ARGS)

deploy-env-sync:
	$(DEPLOY)/env-sync.sh $(ARGS)

deploy-lint:
	shellcheck -x -P SCRIPTDIR $(DEPLOY)/*.sh
