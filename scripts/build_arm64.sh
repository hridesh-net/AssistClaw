#!/bin/bash
# Local build script for AssistClaw linux-arm64 using Zig
# Requires Zig installed: brew install zig

set -e

echo "[assistclaw] Building for linux-arm64 using Zig..."

# Create dist directory
mkdir -p dist

# Set CGO flags and compilers
export CGO_ENABLED=1
export GOOS=linux
export GOARCH=arm64
export CC="zig cc -target aarch64-linux-musl"
export CXX="zig c++ -target aarch64-linux-musl"

# Build with static linking and fts5 tags
go build -mod=vendor -tags fts5 -ldflags "-s -w -extldflags '-static'" -o dist/assistclaw-linux-arm64 ./cmd/assistclaw

echo "[assistclaw] Build complete: dist/assistclaw-linux-arm64"
ls -lh dist/assistclaw-linux-arm64
file dist/assistclaw-linux-arm64
