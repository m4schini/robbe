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

.PHONY: help build lint check-units test test-integration

help: ## Show this help
	@grep -hE '^[a-zA-Z_-]+:.*?## ' $(MAKEFILE_LIST) \
		| awk 'BEGIN { FS = ":.*?## " } { printf "  \033[36m%-18s\033[0m %s\n", $$1, $$2 }'

build: ## Build the robbe binary into ./bin
	$(GO) build -trimpath -ldflags "-s -w -X main.version=$(VERSION)" -o bin/robbe .

lint: check-units ## Run golangci-lint and the unit-file drift check
	golangci-lint run ./...

# deploy/systemd/robbe-sync.service is the reference copy of the embedded
# template with the binary path filled in. Both must agree line for line
# apart from that substitution, so a directive added to one cannot be
# forgotten in the other.
check-units: ## Check that the reference service unit matches the embedded template
	@sed 's#/usr/local/bin/robbe#{{.Binary}}#' deploy/systemd/robbe-sync.service | diff - app/install/templates/robbe-sync.service \
		&& echo "check-units: deploy/systemd/robbe-sync.service matches app/install/templates/robbe-sync.service"

test: ## Run the tests (no build tags)
	$(GO) test $(GOTESTFLAGS) $(PKGS)

test-integration: ## Run the tests including the integration suite (-tags=integration)
	$(GO) test -tags=$(INTEGRATION_TAG) $(GOTESTFLAGS) $(PKGS)
