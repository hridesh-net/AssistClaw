# Contributing to AssistClaw

Thank you for your interest in AssistClaw! We welcome contributions to make this edge intelligence system even better.

## 🛠️ Development Environment

### Prerequisites
- **Go**: 1.24+
- **Python**: 3.10+ (for the Autonomous Tool Factory)
- **CGO**: Required for `sqlite-vec` and hardware sensing (OpenCV, PortAudio, pigpio).
- **Libraries**: `libsqlite3-dev` (Linux) or `sqlite` (Homebrew/macOS).

### Recommended Setup
1. Clone the repo: `git clone https://github.com/hridesh-net/AssistClaw.git`
2. Install dependencies: `go mod tidy`
3. Build the binary: `make build`

## 🏗️ Architecture Layout

- `cmd/assistclaw/`: CLI entrypoints and daemon management.
- `internal/agent/`: Core reasoning loop (Planning, Execution, Reflection).
- `internal/channels/`: Integration layers (WhatsApp, Telegram, Discord, Slack).
- `internal/channels/adapter/`: Versioned channel contract + reliability wrappers + capability registry.
- `internal/provider/`: LLM client implementations.
- `internal/memory/`: Semantic (vector) and Episodic (FTS5) persistence.
- `bridge/`: C++ layer for hardware sensing (Camera, GPIO).

## 🔌 Channel Capabilities

- Built-in capability matrix: `doc/channels/capability-registry.md`
- Runtime lookup: `adapter.CapabilitiesFor(channelType)`
- New channel registration hook: `adapter.RegisterCapabilities(channelType, caps)`

## 🧪 Coding Standards

- **Go**: Follow standard `gofmt` and `golangci-lint` patterns.
- **Privacy**: Never log raw API keys or user message content unless `DEBUG` is explicitly enabled.
- **Performance**: Use buffered channels and non-blocking I/O for hardware-sensing loops.

## Adapter Contract + Property Tests

- Run locally: `go test ./internal/channels/adapter -run 'TestAdapterContract|TestProperty' -count=1`
- Full adapter package tests: `go test ./internal/channels/adapter -count=1`
- Property test settings are deterministic in CI:
  - Seed: `20260414`
  - Minimum fuzz/property cases: `300`

Example snippet for adding a new adapter to the contract suite:

```go
func TestAdapterContract_MyAdapter(t *testing.T) {
	runAdapterContractSuite(t, "my-adapter", func(t *testing.T) *contractAdapter {
		return &contractAdapter{
			name:          "my-adapter",
			providerMsgID: "my-msg-1",
		}
	})
}
```

## New Channel Adapter Checklist

Use this checklist for any channel adapter PR. A merge-ready PR should satisfy all items below.

- [ ] **Implement Adapter v1 contract (STORY-001)**
  - Interface + methods: `internal/channels/adapter/adapter.go`
  - Shared types: `internal/channels/adapter/types.go`
  - Error taxonomy/classification: `internal/channels/adapter/errors.go`
  - Versioning/ADR reference: `doc/architecture/channel-adapter-v1.md`

- [ ] **Use shared outbound reliability (STORY-002)**
  - Retry/backoff/circuit-breaker/DLQ wrapper: `internal/channels/adapter/reliability.go`
  - Wire channel outbound through `adapter.NewReliableSender(...)` from startup path (`cmd/assistclaw/main.go`) and channel reply path (`WithReliableOutbound` style hooks where present).
  - DLQ operator workflow documented in: `doc/runbooks/dlq-inspection.md`

- [ ] **Register channel capabilities (STORY-003)**
  - Capability registration/lookup API: `internal/channels/adapter/capability_registry.go`
  - Update capability docs table: `doc/channels/capability-registry.md`
  - Ensure optional unsupported features degrade gracefully (for example threading metadata).

- [ ] **Provide migration parity evidence (STORY-004)**
  - Channel package tests include parsing/splitting + adapter send behavior.
  - If config behavior changed, include migration note in PR.
  - Add rollback notes in PR description (what to revert and where).

- [ ] **Pass contract/property suite (STORY-005)**
  - Contract/property tests: `internal/channels/adapter/contract_property_test.go`
  - CI gate workflow: `.github/workflows/adapter-contract-tests.yml`
  - Deterministic property settings:
    - seed: `20260414`
    - count: `300`

- [ ] **Security and logging baseline (Sprint-04 forward reference)**
  - Respect channel allowlist and mention-gating config where applicable.
  - Use classified adapter errors (`ErrorKindRetryable`, `ErrorKindPermanent`, `ErrorKindRateLimited`) rather than string matching.

- [ ] **Release hygiene**
  - Add changelog entry under `CHANGELOG.md` (`[Unreleased]`).
  - Include test commands and outputs in PR body.

### Suggested verification commands

- `go test ./internal/channels/adapter -count=1`
- `go test ./internal/channels/<your-channel> -count=1`
- `go test ./... -count=1`
- `go build -mod=vendor -tags fts5 ./cmd/assistclaw`

### Related references

- Capability matrix: `doc/channels/capability-registry.md`
- DLQ incident workflow: `doc/runbooks/dlq-inspection.md`
- Sprint story docs: `doc/Sprints/stories/sprint-01/`

## 🚀 Pull Request Process

1. Create a descriptive branch: `feat/new-sensing-protocol` or `fix/whatsapp-timeout`.
2. Ensure `go build ./...` passes.
3. Update `CHANGELOG.md` under the `[Unreleased]` section.
4. Submit your PR and describe the testing steps (e.g., "Tested on RPi 5 with Bedrock").

---

*AssistClaw — Empowering Edge Intelligence Together.*
