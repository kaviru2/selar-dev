# SELAR — Development Makefile
# Convenience targets for local development with Docker Compose.

.PHONY: dev down logs build migrate replay clean api-dev console-dev

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

# Run database migration manually
migrate:
	docker compose exec postgres psql -U selar -d selar -f /docker-entrypoint-initdb.d/001_init.sql
	docker compose exec postgres psql -U selar -d selar -f /docker-entrypoint-initdb.d/002_add_summary.sql
	docker compose exec postgres psql -U selar -d selar -f /docker-entrypoint-initdb.d/003_runtime_mental_model.sql
	docker compose exec postgres psql -U selar -d selar -f /docker-entrypoint-initdb.d/004_adaptive_chat.sql
	docker compose exec postgres psql -U selar -d selar -f /docker-entrypoint-initdb.d/005_chat_graph_reducer.sql
	docker compose exec postgres psql -U selar -d selar -f /docker-entrypoint-initdb.d/006_graph_governance.sql
	docker compose exec postgres psql -U selar -d selar -f /docker-entrypoint-initdb.d/007_multimodal_sources.sql
	docker compose exec postgres psql -U selar -d selar -f /docker-entrypoint-initdb.d/008_grounded_chat_concepts.sql
	docker compose exec postgres psql -U selar -d selar -f /docker-entrypoint-initdb.d/009_durable_ingestion_jobs.sql

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
