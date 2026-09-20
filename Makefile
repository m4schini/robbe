GO ?= go
PKGS ?= ./...
# -race catches data races that only appear under load in production;
# -shuffle=on catches tests that pass only because of their ordering.
GOTESTFLAGS ?= -race -shuffle=on
# Build tag that marks the integration suite. Referenced in one place so
# renaming it does not require touching every target.
INTEGRATION_TAG := integration

.DEFAULT_GOAL := help

VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)

.PHONY: help build lint test test-integration

help: ## Show this help
	@grep -hE '^[a-zA-Z_-]+:.*?## ' $(MAKEFILE_LIST) \
		| awk 'BEGIN { FS = ":.*?## " } { printf "  \033[36m%-18s\033[0m %s\n", $$1, $$2 }'

build: ## Build the robbe binary into ./bin
	$(GO) build -trimpath -ldflags "-s -w -X main.version=$(VERSION)" -o bin/robbe .

lint: ## Run golangci-lint
	golangci-lint run ./...

test: ## Run the tests (no build tags)
	$(GO) test $(GOTESTFLAGS) $(PKGS)

test-integration: ## Run the tests including the integration suite (-tags=integration)
	$(GO) test -tags=$(INTEGRATION_TAG) $(GOTESTFLAGS) $(PKGS)
