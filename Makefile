# cronos — see AGENTS.md for the rules these targets enforce.

PORTAL := apps/portal
GO     := go

.DEFAULT_GOAL := help
.PHONY: help setup dev dev-web dev-api build check test lint fmt boundary ui shots clean

help: ## Show this help
	@awk 'BEGIN {FS = ":.*##"; printf "\nUsage: make <target>\n\n"} \
		/^[a-zA-Z_-]+:.*?##/ { printf "  \033[36m%-12s\033[0m %s\n", $$1, $$2 } \
		END { printf "\n" }' $(MAKEFILE_LIST)

setup: ## Verify the toolchain and install dependencies
	@./scripts/setup.sh

dev: ## Run the API and portal together
	@./scripts/dev.sh

dev-web: ## Run the portal only
	@./scripts/dev.sh --web

dev-api: ## Run the API only
	@./scripts/dev.sh --api

build: ## Build both binaries and the portal
	$(GO) build -o bin/cronosd ./cmd/cronosd
	$(GO) build -o bin/cronosd-ee ./cmd/cronosd-ee
	cd $(PORTAL) && bun run build

check: ## Everything CI runs — build, vet, boundary, typecheck, lint, budgets
	$(GO) build ./...
	$(GO) vet ./...
	@./scripts/check-license-boundary.sh
	cd $(PORTAL) && bun run check

test: ## Run Go tests
	$(GO) test ./...

lint: ## Lint the portal
	cd $(PORTAL) && bun run lint

fmt: ## Format Go sources
	$(GO) fmt ./...

boundary: ## Verify no BSL artifact depends on ee/
	@./scripts/check-license-boundary.sh

ui: ## Run every browser suite against a running portal (make dev-web first)
	cd $(PORTAL) && bun run build && bun run verify

shots: ## Drive the portal in headless Chrome and write screenshots
	cd $(PORTAL) && bun run shots

clean: ## Remove build output
	rm -rf bin $(PORTAL)/dist $(PORTAL)/dev-dist $(PORTAL)/shots
