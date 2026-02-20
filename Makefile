# AssistClaw — Makefile
# Industry-standard build targets for the polyglot project.

.DEFAULT_GOAL := build
.PHONY: all build build-go build-ts build-sensing clean test lint vet fmt install uninstall help

# ─────────────────────────────────────────────
# Variables
# ─────────────────────────────────────────────
BINARY     := assistclaw
GOCMD      := go
GOBUILD    := CGO_ENABLED=1 $(GOCMD) build
GOTEST     := CGO_ENABLED=1 $(GOCMD) test
GOVET      := $(GOCMD) vet
GOLINT     := golangci-lint
GOFMT      := gofmt
PKG        := ./...
BIN_DIR    := ./bin
INSTALL_DIR?= /usr/local/bin
VERSION    := $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
LDFLAGS    := -X main.version=$(VERSION) -s -w
SENSING_DIR:= sensing
NODE_CMD   := node
PNPM       := pnpm

# Colors
RED    := \033[0;31m
GREEN  := \033[0;32m
YELLOW := \033[0;33m
BLUE   := \033[0;34m
NC     := \033[0m

# ─────────────────────────────────────────────
# Top-level targets
# ─────────────────────────────────────────────

## all: Build everything (Go binary + TypeScript layer)
all: build-go build-ts

## build: Build the Go binary
build: build-go

## build-go: Build the Go orchestrator binary
build-go:
	@echo "$(BLUE)Building Go binary...$(NC)"
	@mkdir -p $(BIN_DIR)
	$(GOBUILD) -ldflags "$(LDFLAGS)" -o $(BIN_DIR)/$(BINARY) ./cmd/assistclaw
	@echo "$(GREEN)✓ Binary: $(BIN_DIR)/$(BINARY)$(NC)"

## build-ts: Build the TypeScript agent layer
build-ts:
	@echo "$(BLUE)Building TypeScript layer...$(NC)"
	$(PNPM) build
	@echo "$(GREEN)✓ TypeScript build complete$(NC)"

## build-sensing: Build the C++ sensing binaries
build-sensing:
	@echo "$(BLUE)Building C++ sensing layer...$(NC)"
	@command -v cmake >/dev/null 2>&1 || { echo "$(RED)cmake not found — skipping C++ build$(NC)"; exit 0; }
	cmake -S $(SENSING_DIR) -B $(SENSING_DIR)/build -DCMAKE_BUILD_TYPE=Release
	cmake --build $(SENSING_DIR)/build --parallel
	@echo "$(GREEN)✓ Sensing binaries built$(NC)"

## cross: Build release binaries for multiple platforms
cross:
	@echo "$(BLUE)Cross-compiling release binaries...$(NC)"
	@mkdir -p dist
	GOOS=darwin  GOARCH=arm64 $(GOBUILD) -ldflags "$(LDFLAGS)" -o dist/$(BINARY)-darwin-arm64 ./cmd/assistclaw
	GOOS=darwin  GOARCH=amd64 $(GOBUILD) -ldflags "$(LDFLAGS)" -o dist/$(BINARY)-darwin-amd64 ./cmd/assistclaw
	GOOS=linux   GOARCH=amd64 $(GOBUILD) -ldflags "$(LDFLAGS)" -o dist/$(BINARY)-linux-amd64  ./cmd/assistclaw
	GOOS=linux   GOARCH=arm64 $(GOBUILD) -ldflags "$(LDFLAGS)" -o dist/$(BINARY)-linux-arm64  ./cmd/assistclaw
	GOOS=windows GOARCH=amd64 $(GOBUILD) -ldflags "$(LDFLAGS)" -o dist/$(BINARY)-windows-amd64.exe ./cmd/assistclaw
	@echo "$(GREEN)✓ Cross-compiled binaries in dist/$(NC)"

# ─────────────────────────────────────────────
# Testing
# ─────────────────────────────────────────────

## test: Run all Go tests with race detector
test:
	@echo "$(BLUE)Running Go tests (race detector enabled)...$(NC)"
	$(GOTEST) -race -coverprofile=coverage.out $(PKG)
	@echo "$(GREEN)✓ Tests passed$(NC)"

## test-ts: Run TypeScript/Node tests
test-ts:
	$(PNPM) test

## coverage: Run tests and show coverage report
coverage: test
	go tool cover -html=coverage.out

# ─────────────────────────────────────────────
# Code quality
# ─────────────────────────────────────────────

## lint: Run golangci-lint
lint:
	@command -v $(GOLINT) >/dev/null 2>&1 || { echo "$(YELLOW)golangci-lint not found, run: go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest$(NC)"; exit 1; }
	$(GOLINT) run $(PKG)

## vet: Run go vet
vet:
	$(GOVET) $(PKG)

## fmt: Format Go source with gofmt
fmt:
	$(GOFMT) -w -s .

## fmt-check: Check Go source formatting
fmt-check:
	@out=$$($(GOFMT) -l .); if [ -n "$$out" ]; then echo "$(RED)Needs formatting:$(NC)\n$$out"; exit 1; fi

## check: Run all quality checks (vet + fmt-check + lint)
check: vet fmt-check lint
	@echo "$(GREEN)✓ All checks passed$(NC)"

# ─────────────────────────────────────────────
# Installation
# ─────────────────────────────────────────────

## install: Build and install binary to INSTALL_DIR
install: build-go
	@echo "$(BLUE)Installing to $(INSTALL_DIR)/$(BINARY)...$(NC)"
	cp $(BIN_DIR)/$(BINARY) $(INSTALL_DIR)/$(BINARY)
	chmod +x $(INSTALL_DIR)/$(BINARY)
	@echo "$(GREEN)✓ Installed: $(INSTALL_DIR)/$(BINARY)$(NC)"

## uninstall: Remove installed binary
uninstall:
	rm -f $(INSTALL_DIR)/$(BINARY)
	@echo "$(GREEN)✓ Uninstalled$(NC)"

## deps: Download all Go dependencies
deps:
	$(GOCMD) mod download
	$(GOCMD) mod tidy
	$(PNPM) install

# ─────────────────────────────────────────────
# Cleanup
# ─────────────────────────────────────────────

## clean: Remove build artifacts
clean:
	rm -rf $(BIN_DIR) dist coverage.out
	rm -rf $(SENSING_DIR)/build
	$(PNPM) exec rimraf dist 2>/dev/null || true

## help: Show this help message
help:
	@echo "$(BLUE)AssistClaw Build System$(NC)"
	@echo ""
	@grep -E '^## ' Makefile | sed 's/## //' | column -t -s ':'
