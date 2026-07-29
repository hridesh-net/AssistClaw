#!/usr/bin/env bash
# AssistClaw — cross-compile a portable, statically-linked single binary
# for darwin-arm64, darwin-amd64, linux-amd64, linux-arm64.
#
# Strategy:
#   • Rust TUI is built as a static lib (libassistclaw_tui.a) per target.
#   • Go binary is built with CGO using zig cc as the cross C compiler.
#   • The `assistclaw_localgemma` tag is intentionally omitted from cross
#     builds because gollama.cpp loads libllama via libffi at runtime, which
#     does not survive a portable static build. Users who want local Gemma
#     can run `make build EXTRA_TAGS=assistclaw_embedlocalgemma` on host.
#
# Requirements (install once):
#   • zig (>= 0.13)              — brew install zig         | apt install zig
#   • rustup targets for Rust    — see RUST_TARGETS below
#   • Go 1.22+ with CGO_ENABLED  — already required by the project
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"

VERSION="$(git describe --tags --always --dirty 2>/dev/null || echo dev)"
LDFLAGS="-X main.version=${VERSION} -s -w"
DIST="${ROOT}/dist"
RUST_DIR="${ROOT}/cmd/assistclaw/tui_rs"
RUST_OUT="${RUST_DIR}/target/release/libassistclaw_tui.a"
BUILD_TAGS="${BUILD_TAGS:-fts5}"

mkdir -p "${DIST}"

if ! command -v zig >/dev/null 2>&1; then
  echo "✗ zig not found. Install with 'brew install zig' or 'apt install zig'." >&2
  exit 1
fi

# ONLY filters targets, e.g. ONLY=linux-arm64 ./scripts/build_cross.sh
# (used by `make pi` to build just the Raspberry Pi 5 binary).
ONLY="${ONLY:-}"

# target_triple zig_target goos goarch
TARGETS=(
  "aarch64-apple-darwin       aarch64-macos-none   darwin  arm64"
  "x86_64-apple-darwin        x86_64-macos-none    darwin  amd64"
  "x86_64-unknown-linux-musl  x86_64-linux-musl    linux   amd64"
  "aarch64-unknown-linux-musl aarch64-linux-musl   linux   arm64"
)

RUST_TARGETS=(aarch64-apple-darwin x86_64-apple-darwin x86_64-unknown-linux-musl aarch64-unknown-linux-musl)
for t in "${RUST_TARGETS[@]}"; do
  if ! rustup target list --installed | grep -q "^${t}$"; then
    echo "→ adding Rust target ${t}"
    rustup target add "${t}"
  fi
done

for line in "${TARGETS[@]}"; do
  # shellcheck disable=SC2086
  set -- $line
  RUST_T="$1"; ZIG_T="$2"; GOOS="$3"; GOARCH="$4"
  if [[ -n "${ONLY}" && "${ONLY}" != "${GOOS}-${GOARCH}" ]]; then
    continue
  fi
  OUT="${DIST}/assistclaw-${GOOS}-${GOARCH}"

  echo
  echo "═══ ${GOOS}/${GOARCH} (rust=${RUST_T}, zig=${ZIG_T}) ═══"

  echo "• cargo build --release --target=${RUST_T}"
  (cd "${RUST_DIR}" && cargo build --release --target="${RUST_T}")

  # cgo LDFLAGS in tui_rs.go expect the lib at target/release/libassistclaw_tui.a
  install -m644 "${RUST_DIR}/target/${RUST_T}/release/libassistclaw_tui.a" "${RUST_OUT}"

  # Rust's staticlib references _Unwind_* (std backtrace/personality); on musl
  # targets neither zig's musl libc nor Go's link provides them. Rust ships a
  # self-contained libunwind.a alongside every musl target — link it in.
  EXTRA_CGO_LDFLAGS=""
  if [[ "${GOOS}" == "linux" ]]; then
    RUST_SELFCONTAINED="$(rustc --print target-libdir --target "${RUST_T}")/self-contained"
    if [[ -f "${RUST_SELFCONTAINED}/libunwind.a" ]]; then
      EXTRA_CGO_LDFLAGS="${RUST_SELFCONTAINED}/libunwind.a"
    fi
  fi

  echo "• go build -tags ${BUILD_TAGS}"
  CC="zig cc -target ${ZIG_T}" \
  CXX="zig c++ -target ${ZIG_T}" \
  CGO_ENABLED=1 \
  CGO_LDFLAGS="${EXTRA_CGO_LDFLAGS}" \
  GOOS="${GOOS}" \
  GOARCH="${GOARCH}" \
  go build -tags "${BUILD_TAGS}" -ldflags "${LDFLAGS}" -o "${OUT}" ./cmd/assistclaw

  echo "• built $(file "${OUT}" | cut -d: -f2-)"
done

echo
echo "✓ Cross-compile complete. Artifacts in ${DIST}/"
ls -la "${DIST}"
