# AssistClaw — Makefile
# Industry-standard build targets for the polyglot project.

.DEFAULT_GOAL := build
.PHONY: all build build-go build-sensing clean test load-test lint vet fmt install uninstall help

# ─────────────────────────────────────────────
# Variables
# ─────────────────────────────────────────────
BINARY     := assistclaw
GOCMD      := go
# Default build includes in-process local Gemma; append optional extras (e.g. assistclaw_embedlocalgemma)
EXTRA_TAGS ?=
comma      := ,
BUILD_TAGS := fts5,assistclaw_localgemma
ifneq ($(strip $(EXTRA_TAGS)),)
  BUILD_TAGS := $(BUILD_TAGS)$(comma)$(EXTRA_TAGS)
endif
GOBUILD    := CGO_ENABLED=1 $(GOCMD) build -tags $(BUILD_TAGS)
GOTEST     := CGO_ENABLED=1 $(GOCMD) test -tags $(BUILD_TAGS)
GOVET      := $(GOCMD) vet
GOLINT     := golangci-lint
GOFMT      := gofmt
PKG        := ./...
BIN_DIR    := ./bin
INSTALL_DIR?= /usr/local/bin
VERSION    := $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
LDFLAGS    := -X main.version=$(VERSION) -s -w
SENSING_DIR:= sensing
RUST_DIR   := cmd/assistclaw/tui_rs
RUST_LIB   := $(RUST_DIR)/target/release/libassistclaw_tui.a

# Colors
RED    := \033[0;31m
GREEN  := \033[0;32m
YELLOW := \033[0;33m
BLUE   := \033[0;34m
NC     := \033[0m

# ─────────────────────────────────────────────
# Top-level targets
# ─────────────────────────────────────────────

## all: Build everything (Go binary + C++ sensing)
all: build build-sensing

## build: Build the Go binary (includes Rust TUI)
build: build-tui-rs build-go

## build-tui-rs: Build the Rust TUI static library
build-tui-rs:
	@echo "$(BLUE)Building Rust TUI library...$(NC)"
	cd $(RUST_DIR) && cargo build --release
	@echo "$(GREEN)✓ Rust TUI library built$(NC)"

## build-go: Build the Go orchestrator binary
build-go: $(RUST_LIB)
	@echo "$(BLUE)Building Go binary...$(NC)"
	@mkdir -p $(BIN_DIR)
	$(GOBUILD) -ldflags "$(LDFLAGS)" -o $(BIN_DIR)/$(BINARY) ./cmd/assistclaw
	@echo "$(GREEN)✓ Binary: $(BIN_DIR)/$(BINARY)$(NC)"

## build-sensing: Build the C++ sensing binaries
build-sensing:
	@echo "$(BLUE)Building C++ sensing layer...$(NC)"
	@command -v cmake >/dev/null 2>&1 || { echo "$(RED)cmake not found — skipping C++ build$(NC)"; exit 0; }
	cmake -S $(SENSING_DIR) -B $(SENSING_DIR)/build -DCMAKE_BUILD_TYPE=Release
	cmake --build $(SENSING_DIR)/build --parallel
	@echo "$(GREEN)✓ Sensing binaries built$(NC)"

## cross: Build portable release binaries for darwin and linux (arm64 + amd64)
## Requires zig + rustup targets (see scripts/build_cross.sh). Run `make cross-deps` once.
## Note: cross builds intentionally omit the assistclaw_localgemma tag — the
## gollama runtime loader does not survive a portable static build. Use
## `make build` on the host to get a binary with local Gemma support.
cross:
	@echo "$(BLUE)Cross-compiling release binaries (darwin+linux × arm64+amd64)...$(NC)"
	@./scripts/build_cross.sh
	@echo "$(GREEN)✓ Cross-compiled binaries in dist/$(NC)"

## pi: Build only the Raspberry Pi 5 binary (linux/arm64, portable static)
pi:
	@echo "$(BLUE)Cross-compiling for Raspberry Pi 5 (linux/arm64)...$(NC)"
	@ONLY=linux-arm64 ./scripts/build_cross.sh
	@echo "$(GREEN)✓ dist/assistclaw-linux-arm64 ready — see doc/RASPBERRY_PI.md$(NC)"

## cross-deps: Install one-time dependencies for cross-compilation
cross-deps:
	@echo "$(BLUE)Checking cross-compile prerequisites...$(NC)"
	@command -v zig >/dev/null 2>&1 || { echo "$(RED)zig missing — brew install zig (or apt install zig)$(NC)"; exit 1; }
	@rustup target add aarch64-apple-darwin x86_64-apple-darwin \
	    x86_64-unknown-linux-musl aarch64-unknown-linux-musl
	@echo "$(GREEN)✓ Cross-compile toolchain ready$(NC)"

# ─────────────────────────────────────────────
# Testing
# ─────────────────────────────────────────────

## test: Run all Go tests with race detector
test:
	@echo "$(BLUE)Running Go tests (race detector enabled)...$(NC)"
	$(GOTEST) -race -coverprofile=coverage.out $(PKG)
	@echo "$(GREEN)✓ Tests passed$(NC)"

## load-test: Run sprint-02 load and failure-injection harness
load-test:
	@echo "$(BLUE)Running load test harness...$(NC)"
	$(GOTEST) ./internal/channels/adapter -run TestLoadHarness_Report -count=1 -v
	@echo "$(GREEN)✓ Load test harness completed$(NC)"

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

# ─────────────────────────────────────────────
# Cleanup
# ─────────────────────────────────────────────

## clean: Remove build artifacts
clean:
	rm -rf $(BIN_DIR) dist coverage.out
	rm -rf $(SENSING_DIR)/build
	cd $(RUST_DIR) && cargo clean

## help: Show this help message
help:
	@echo "$(BLUE)AssistClaw Build System$(NC)"
	@echo ""
	@echo "$(YELLOW)Optional:$(NC) EXTRA_TAGS=assistclaw_embedlocalgemma to embed GGUF in binary."
	@echo ""
	@grep -E '^## ' Makefile | sed 's/## //' | column -t -s ':'
