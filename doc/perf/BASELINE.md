# Performance Baseline — pre-revamp

Recorded at the start of the revamp (branch `revamp/foundation`, commit baseline = tip of `main` `787b32c`) so later workstreams (esp. WS5 build presets + perf gates) can measure regressions against a real starting point.

## How this was measured

- Host: macOS (darwin/arm64, Apple Silicon), Go 1.26.2, cargo 1.96.0.
- Build: `make build` — builds the Rust TUI staticlib (`cargo build --release`) then the Go binary with `CGO_ENABLED=1 -tags fts5,assistclaw_localgemma -ldflags "-s -w"`.
- This is the **default preset** (single monolithic binary linking every channel, tsnet, otel, TUI, local Gemma). The revamp introduces `edge` / `default` / `full` presets (WS5); this baseline is the "everything on" number those will be compared against.

## Numbers

| Metric | Value | Notes |
|---|---|---|
| Binary size | **51.9 MB** (54,425,330 bytes) | stripped (`-s -w`), default preset |
| Clean build time | 34.3 s wall | includes cargo release build of the Rust TUI + CGO |
| Peak build RSS | ~1.05 GB | the compiler, not the runtime |
| CLI subcommand cold start (`version`) | ~0.75 s cold / **0.01 s warm** | page-cache cold vs warm; not the daemon "ready" path |

## Not yet measured (needs a configured daemon)

These require onboarding (providers/channels/config) and will be captured once the WS1 kernel gives a clean headless start path, and on a Raspberry Pi via the `make pi` artifact:

- Idle daemon RSS after 10 min (the number the edge budget of ≤30 MB targets).
- Cold start → gateway `/readyz` ready.
- Time-to-first-token on a real provider call.
- Idle goroutine count.

## Target budgets (from the master plan, enforced from WS5)

| Metric | edge | default | full |
|---|---|---|---|
| Binary size | ≤ 25 MB | ≤ 45 MB | ≤ 80 MB |
| Cold start → ready | ≤ 150 ms | ≤ 400 ms | ≤ 800 ms |
| Idle RSS (10 min) | ≤ 30 MB | ≤ 60 MB | ≤ 120 MB |
| Turn overhead (non-LLM) | ≤ 20 ms | ≤ 30 ms | ≤ 50 ms |
| Idle goroutines | ≤ 40 | ≤ 80 | ≤ 150 |

Note: today's 51.9 MB default binary already exceeds the ≤45 MB default budget — WS5's build-tag gating (moving whatsmeow/chromedp/aws/tsnet/otel/TUI behind features) is what brings it down. The `edge` preset (telegram + webhook only, no tsnet/chromedp/aws/TUI) is expected to land well under 25 MB.
