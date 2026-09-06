# cronos — see AGENTS.md for the rules these targets enforce.

PORTAL := apps/portal
EMBED  := packages/embed
REACT  := packages/react
# Whichever is installed. All three build this Dockerfile: Apple's `container`
# is what this is developed against on macOS, docker is what most CI has, and
# podman is the fallback.
CONTAINER := $(shell command -v container 2>/dev/null || command -v docker 2>/dev/null || command -v podman)
VERSION := $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
GO     := go

.DEFAULT_GOAL := help
.PHONY: help setup dev dev-web dev-api build check test xlsx-oracle duckdb pdf lint fmt boundary live ui shots image load release clean import dist

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
	$(GO) build -o bin/cronos-import ./cmd/cronos-import
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
	@./scripts/check-release-parity.sh
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

import: ## Dry-run the JasperReports importer over a directory: JASPER=./reports make import
	@test -n "$(JASPER)" || { echo "usage: JASPER=<dir-of-jrxml> make import" >&2; exit 1; }
	@$(GO) run ./cmd/cronos-import $(JASPER) || \
		{ echo; echo "Exit 1 means files are blocked and need a person — not that the tool failed."; exit 1; }

pdf: ## Render a sample statement to /tmp/statement.pdf and open it
	CRONOS_PDF_OUT=/tmp/statement.pdf $(GO) test ./internal/adapter/render/paginated/ -run TestRenderProducesAPDF -v
	@open /tmp/statement.pdf 2>/dev/null || echo "wrote /tmp/statement.pdf"

lint: ## Lint the portal and the embed package
	cd $(PORTAL) && bun run lint
	cd $(EMBED) && bun run lint
	cd $(REACT) && bun run lint

fmt: ## Format Go sources
	$(GO) fmt ./...

boundary: ## Verify no BSL artifact depends on ee/, and that both channels ship the same commands
	@./scripts/check-license-boundary.sh
	@./scripts/check-release-parity.sh

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

dist: ## Cross-compile the release archives into dist/
	@./scripts/dist.sh

image: ## Build the container image, and prove the typesetter is in it
	$(CONTAINER) build -t cronos:$(VERSION) --build-arg CRONOS_VERSION=$(VERSION) .
	@echo "--- the one thing an image can be missing and not say so ---"
	$(CONTAINER) run --rm --entrypoint typst cronos:$(VERSION) --version
	@echo "--- and the other: which build it is ---"
	$(CONTAINER) run --rm cronos:$(VERSION) -version

release: ## Check a tag can be cut: VERSION=v0.5.1 make release
	@test -n "$(RELEASE)" || { echo "usage: RELEASE=v0.5.1 make release" >&2; exit 1; }
	@git diff --quiet || { echo "the tree is dirty — a tag would name a build nobody can rebuild" >&2; exit 1; }
	@grep -q "^## $(RELEASE) " CHANGELOG.md || \
		{ echo "CHANGELOG.md has no '## $(RELEASE)' entry — a version an operator cannot look up" >&2; exit 1; }
	@echo "ready: git tag -a $(RELEASE) -m $(RELEASE) && git push origin $(RELEASE)"
	@echo
	@echo "Pushing the tag runs .github/workflows/release.yml, which builds the"
	@echo "archives, writes an SBOM for each, signs SHA256SUMS and publishes the"
	@echo "GitHub Release. Nothing below is needed for that."
	@echo
	@echo "local:  make dist    # the same archives, unsigned, to look at"
	@echo "        make image   # the container image, which CI does not publish"

clean: ## Remove build output
	rm -rf bin dist $(REACT)/dist $(EMBED)/dist $(EMBED)/harness/*.js $(REACT)/harness/*.js $(PORTAL)/dist $(PORTAL)/dev-dist $(PORTAL)/shots
