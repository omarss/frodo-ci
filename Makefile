# Frodo CI -- Makefile
# Opinionated modular CI/CD framework for monorepos.
#
# Run `make help` to see available targets.

BINARY  := frodo-ci
PKG     := github.com/frodo-ci/frodo-ci
CMD_PKG := ./cmd/frodo-ci
BIN_DIR := bin
BIN     := $(BIN_DIR)/$(BINARY)

# Version metadata injected at build time via -ldflags.
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
COMMIT  ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo "none")
DATE    ?= $(shell date -u +%Y-%m-%dT%H:%M:%SZ)
LDFLAGS := -s -w \
	-X $(PKG)/internal/version.Version=$(VERSION) \
	-X $(PKG)/internal/version.Commit=$(COMMIT) \
	-X $(PKG)/internal/version.BuildDate=$(DATE)

GO      ?= go
GOFLAGS ?=

# Prefer golangci-lint when installed; otherwise fall back to `go vet`.
GOLANGCI := $(shell command -v golangci-lint 2>/dev/null)

.DEFAULT_GOAL := build

.PHONY: all
all: tidy fmt lint test build ## Tidy, format, lint, test, then build

.PHONY: build
build: ## Compile the frodo-ci binary into ./bin
	@mkdir -p $(BIN_DIR)
	$(GO) build $(GOFLAGS) -ldflags '$(LDFLAGS)' -o $(BIN) $(CMD_PKG)

.PHONY: install
install: ## Install frodo-ci into GOBIN
	$(GO) install $(GOFLAGS) -ldflags '$(LDFLAGS)' $(CMD_PKG)

.PHONY: run
run: build ## Build then run (use ARGS="plan --json")
	$(BIN) $(ARGS)

.PHONY: test
test: ## Run all tests with the race detector
	$(GO) test $(GOFLAGS) -race ./...

.PHONY: test-short
test-short: ## Run tests without the race detector
	$(GO) test $(GOFLAGS) -short ./...

.PHONY: cover
cover: ## Generate an HTML coverage report (needs a full Go SDK for `covdata`)
	$(GO) test $(GOFLAGS) -covermode=atomic -coverprofile=coverage.out ./...
	$(GO) tool cover -html=coverage.out -o coverage.html

.PHONY: fmt
fmt: ## Format all Go code
	$(GO) fmt ./...

.PHONY: vet
vet: ## Run go vet
	$(GO) vet ./...

.PHONY: lint
lint: ## Run golangci-lint (falls back to go vet when not installed)
ifndef GOLANGCI
	@echo ">> golangci-lint not found; running 'go vet' instead"
	@$(GO) vet ./...
else
	$(GOLANGCI) run ./...
endif

.PHONY: tidy
tidy: ## Tidy and verify module dependencies
	$(GO) mod tidy
	$(GO) mod verify

.PHONY: schemas
schemas: build ## Regenerate JSON Schemas into .github/frodo-ci/schemas
	$(BIN) schemas export --out .github/frodo-ci/schemas

.PHONY: clean
clean: ## Remove build artifacts
	rm -rf $(BIN_DIR) dist coverage.out coverage.html

.PHONY: help
help: ## Show this help
	@grep -hE '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | \
		awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-14s\033[0m %s\n", $$1, $$2}'
