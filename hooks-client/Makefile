GO      := $(shell which go 2>/dev/null || echo /usr/local/go/bin/go)
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS := -s -w -X main.version=$(VERSION)

.PHONY: build clean test

build: ## Build hook-client binary
	@mkdir -p bin
	$(GO) build -ldflags="$(LDFLAGS)" -o bin/hook-client .
	@echo "Built bin/hook-client"

test: ## Run tests
	$(GO) test ./...

clean: ## Remove build artifacts
	rm -rf bin/
