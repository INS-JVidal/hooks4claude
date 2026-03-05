# hooks4claude — Top-level Makefile
# Usage: make help

.PHONY: help build build-monitor build-hooks-client build-store build-shim test test-e2e clean

help: ## Show all targets
	@echo ""
	@echo "  hooks4claude"
	@echo "  ────────────"
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | \
		awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-20s\033[0m %s\n", $$1, $$2}'
	@echo ""

build: build-monitor build-hooks-client ## Build all binaries

build-monitor: ## Build the monitor server
	$(MAKE) -C hooks-monitor build

build-hooks-client: ## Build the hook client
	$(MAKE) -C hooks-client build

build-store: ## Build hooks-store
	$(MAKE) -C hooks-store build

build-shim: ## Build the Rust hook-shim binary
	cd hook-shim && cargo build --release

test: ## Run all tests
	$(MAKE) -C hooks-monitor test
	cd hooks-client && $$(which go 2>/dev/null || echo /usr/local/go/bin/go) test ./...
	$(MAKE) -C hooks-store test

test-e2e: build build-store ## Run end-to-end pipeline tests (requires MeiliSearch on :7700)
	@./scripts/e2e-test.sh

clean: ## Clean all build artifacts
	$(MAKE) -C hooks-monitor clean
	$(MAKE) -C hooks-client clean
