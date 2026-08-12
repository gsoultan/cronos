# cronos — see AGENTS.md for the rules these targets enforce.

PORTAL := apps/portal
EMBED  := packages/embed
REACT  := packages/react
# Whichever is installed. Both build this Dockerfile; podman is what this was
# developed against and docker is what most CI has.
CONTAINER := $(shell command -v podman 2>/dev/null || command -v docker)
VERSION := $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
GO     := go

.DEFAULT_GOAL := help
.PHONY: help setup dev dev-web dev-api build check test xlsx-oracle duckdb pdf lint fmt boundary live ui shots image load clean

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
	cd $(EMBED) && bun run build
	cd $(REACT) && bun run build

XLSX_PY := $(shell command -v /tmp/xlsxvenv/bin/python 2>/dev/null)

check: ## Everything CI runs — build, vet, test, boundary, typecheck, lint, budgets
	$(GO) build ./...
	$(GO) vet ./...
	@gofmt -l . | grep . && { echo "gofmt: files above need formatting"; exit 1; } || true
	CRONOS_XLSX_PYTHON=$(XLSX_PY) $(GO) test ./...
	@./scripts/check-license-boundary.sh
	cd $(PORTAL) && bun run check
	cd $(EMBED) && bun run check
	cd $(REACT) && bun run check

test: ## Run Go tests
	CRONOS_XLSX_PYTHON=$(XLSX_PY) $(GO) test ./...

xlsx-oracle: ## Install the reader the spreadsheet tests check against
	python3 -m venv /tmp/xlsxvenv && /tmp/xlsxvenv/bin/pip install --quiet openpyxl
	@echo "ok  spreadsheet tests will now run rather than skip"

duckdb: ## Build and test the federation adapter (cgo, several hundred MB)
	$(GO) build -tags duckdb ./...
	$(GO) test -tags duckdb ./internal/adapter/driver/duckdb/

pdf: ## Render a sample statement to /tmp/statement.pdf and open it
	CRONOS_PDF_OUT=/tmp/statement.pdf $(GO) test ./internal/adapter/render/paginated/ -run TestRenderProducesAPDF -v
	@open /tmp/statement.pdf 2>/dev/null || echo "wrote /tmp/statement.pdf"

lint: ## Lint the portal and the embed package
	cd $(PORTAL) && bun run lint
	cd $(EMBED) && bun run lint
	cd $(REACT) && bun run lint

fmt: ## Format Go sources
	$(GO) fmt ./...

boundary: ## Verify no BSL artifact depends on ee/
	@./scripts/check-license-boundary.sh

live: ## Drive the embed component and the portal against a real cronosd
	@./scripts/live-embed.sh
	@./scripts/live-portal.sh

ui: ## Run every browser suite against a running portal (make dev-web first)
	cd $(PORTAL) && bun run build && bun run verify
	cd $(EMBED) && bun run build && bun run embed && bun run vue
	cd $(REACT) && bun run react
	@./scripts/live-embed.sh
	@./scripts/live-portal.sh

shots: ## Drive the portal in headless Chrome and write screenshots
	cd $(PORTAL) && bun run shots

load: ## Measure under load — needs a postgres on 5433, or WAREHOUSE=sqlite
	@./scripts/load.sh

image: ## Build the container image, and prove the typesetter is in it
	$(CONTAINER) build -t cronos:$(VERSION) .
	@echo "--- the one thing an image can be missing and not say so ---"
	$(CONTAINER) run --rm --entrypoint typst cronos:$(VERSION) --version

clean: ## Remove build output
	rm -rf bin $(REACT)/dist $(EMBED)/dist $(EMBED)/harness/*.js $(REACT)/harness/*.js $(PORTAL)/dist $(PORTAL)/dev-dist $(PORTAL)/shots
