# SELAR — Development Makefile
# Convenience targets for local development with Docker Compose.

.PHONY: dev down logs build migrate migrate-url replay clean api-dev console-dev

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
