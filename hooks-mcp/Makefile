VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")

.PHONY: build test clean install

build:
	go build -ldflags="-s -w -X main.version=$(VERSION)" -o bin/hooks-mcp ./cmd/hooks-mcp

test:
	go test ./...

clean:
	rm -rf bin/

install: build
	cp bin/hooks-mcp ~/.local/bin/hooks-mcp
