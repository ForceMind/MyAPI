WEB_DIR = ./web
API_DIR = .
DEV_WEB_PORT ?= 5173
DEV_COMPOSE_FILE = docker-compose.dev.yml
DEV_POSTGRES_SERVICE = postgres
# MyAPI's compose service uses the machine slug; database names remain the
# legacy values so existing SQLite/PostgreSQL data can be adopted in place.
DEV_API_SERVICE = my-api
DEV_POSTGRES_DB = new-api
DEV_POSTGRES_USER = root
# my-api.db is the canonical SQLite filename for new development instances.
# Existing one-api.db files remain readable through the runtime fallback; set
# SQLITE_PATH/DEV_SQLITE_PATH to select a specific database explicitly.
DEV_SQLITE_PATH ?= my-api.db

.PHONY: all build-web build-all-web start-api dev dev-api dev-api-rebuild dev-web reset-setup test

all: build-all-web start-api

build-web:
	@echo "Building web frontend..."
	@cd $(WEB_DIR) && bun install --frozen-lockfile
	@cd $(WEB_DIR) && DISABLE_ESLINT_PLUGIN='true' VITE_REACT_APP_VERSION=$$(cat ../VERSION) bun run build

build-all-web: build-web

start-api:
	@echo "Starting MyAPI dev server..."
	@cd $(API_DIR) && go run main.go &

dev-api:
	@echo "Starting MyAPI services (docker)..."
	@docker compose -f $(DEV_COMPOSE_FILE) up -d

dev-api-rebuild:
	@echo "Rebuilding and starting MyAPI service (docker)..."
	@docker compose -f $(DEV_COMPOSE_FILE) up -d --build $(DEV_API_SERVICE)

dev-web:
	@echo "Starting web frontend dev server..."
	@echo "Web frontend: http://localhost:$(DEV_WEB_PORT)"
	@cd $(WEB_DIR) && bun install
	@cd $(WEB_DIR) && bun run dev -- --host 0.0.0.0 --port $(DEV_WEB_PORT)

dev: dev-api dev-web

# The main package embeds the ignored web/dist output and is covered after build-web.
test:
	@echo "Testing root Go module..."
	@root_module=$$(GOWORK=off go list -m); \
		root_packages=$$(GOWORK=off go list -e ./... | grep -vxF "$$root_module"); \
		GOWORK=off go test $$root_packages
	@echo "Testing relaykit Go module..."
	@cd relaykit && GOWORK=off go test ./...

reset-setup:
	@echo "Resetting local setup wizard state..."
	@if docker compose -f $(DEV_COMPOSE_FILE) ps --services --status running | grep -qx "$(DEV_POSTGRES_SERVICE)"; then \
		echo "Detected running docker dev PostgreSQL. Removing setup record and root users..."; \
		docker compose -f $(DEV_COMPOSE_FILE) exec -T $(DEV_POSTGRES_SERVICE) \
			psql -U $(DEV_POSTGRES_USER) -d $(DEV_POSTGRES_DB) \
			-c 'DELETE FROM setups;' \
			-c 'DELETE FROM users WHERE role = 100;' \
			-c "DELETE FROM options WHERE key IN ('SelfUseModeEnabled', 'DemoSiteEnabled');"; \
		echo "Restarting docker dev MyAPI so setup status is recalculated..."; \
		docker compose -f $(DEV_COMPOSE_FILE) restart $(DEV_API_SERVICE); \
	else \
		db_path="$${SQLITE_PATH:-$(DEV_SQLITE_PATH)}"; \
		db_path="$${db_path%%\?*}"; \
		if [ ! -f "$$db_path" ] && [ -z "$${SQLITE_PATH:-}" ] && [ -f one-api.db ]; then db_path=one-api.db; fi; \
		if [ -f "$$db_path" ]; then \
		echo "Detected local SQLite database: $$db_path"; \
		sqlite3 "$$db_path" \
			"DELETE FROM setups; DELETE FROM users WHERE role = 100; DELETE FROM options WHERE key IN ('SelfUseModeEnabled', 'DemoSiteEnabled');"; \
		echo "SQLite setup state reset. Restart the local MyAPI process before testing the setup wizard."; \
		else \
		echo "No running docker dev PostgreSQL or local SQLite database found."; \
		echo "Start the dev stack with 'make dev-api', or set SQLITE_PATH/DEV_SQLITE_PATH to your local SQLite database."; \
		exit 1; \
		fi; \
	fi
