# Time-to-first-message (TTFM) measurement

**TTFM** here means wall-clock time from a **clean tree** (fresh clone or release tarball) through **install** and **first successful agent response** (`assistclaw agent --message "…"` or equivalent non-interactive path).

## What we measure (phases)

| Phase | Start | End | Notes |
|-------|--------|-----|--------|
| **A — Source ready** | `git clone` finished (or archive extracted) | `go` / toolchain usable | Optional: include dependency download time. |
| **B — Install** | Start `install.sh` (or `FORCE_BUILD=1` source path) | `assistclaw version` succeeds | Use `SKIP_NODE=1 SKIP_VENV=1 SKIP_SENSING=1 SKIP_SERVICE=1` for Go-only smoke (see `scripts/ci-fresh-install.sh`). |
| **C — Config** | First read of `assistclaw.yaml` | Valid config for your provider | Minimal fixture: `internal/config/testdata/doctor/valid_minimal.yaml` (Ollama) is not enough for a real LLM reply unless Ollama is running. |
| **D — First message** | Invoke `assistclaw agent --message "ping" --no-stream` (or documented equivalent) | Non-empty assistant reply, exit 0 | Requires a **reachable** provider (API keys, local Ollama, etc.). |

**Full TTFM** = A + B + C + D (same machine, sequential, no manual pauses).

**CI baseline (partial)** = A + B + `assistclaw doctor --skip-network` on the minimal fixture (no LLM call). This is what `fresh-install` workflow runs; it catches install regressions without flaking on external APIs.

## Baseline (recorded)

| Date | Environment | Full TTFM (A–D) | Partial (A–B + doctor) | Notes |
|------|-------------|-----------------|---------------------------|--------|
| 2026-04-16 | GitHub Actions `ubuntu-latest` (fresh-install job) | Not measured in CI | Job wall time only; see workflow run | Hardware: GitHub-hosted standard runner. |
| 2026-04-16 | GitHub Actions `macos-latest` (fresh-install job) | Not measured in CI | Job wall time only | Same. |

Update this table when you re-benchmark on a release candidate or new hardware profile (document OS, CPU, RAM, disk, region).

### How to measure locally (full TTFM)

```bash
# From empty state (example)
/usr/bin/time -p bash -c '
  t0=$(date +%s)
  git clone <repo-url> assistclaw && cd assistclaw
  t1=$(date +%s)
  export FORCE_BUILD=1 SKIP_NODE=1 SKIP_VENV=1 SKIP_SENSING=1 SKIP_SERVICE=1
  bash install.sh
  t2=$(date +%s)
  assistclaw doctor --skip-network
  t3=$(date +%s)
  assistclaw agent --message "Reply with exactly: pong" --no-stream
  t4=$(date +%s)
  echo clone=$((t1-t0))s install=$((t2-t1))s doctor=$((t3-t2))s agent=$((t4-t3))s total=$((t4-t0))s
'
```

Ensure `PATH` includes the install directory printed by `install.sh`. Configure a real provider before the agent step.

## Flake policy (CI)

- **Fresh-install / doctor** jobs: if a failure is **not** clearly tied to a code change (network, GitHub runner disk, transient Go module hiccup), **retry the job once** on the same commit.
- If it fails twice, **open a tracked issue** with logs, runner OS, and timestamps; do not silence with unbounded retries.
- **Snapshot tests**: intentional output changes require `UPDATE_SNAPSHOTS=1 go test ./cmd/assistclaw/ -run TestDoctorTextSnapshot_ValidMinimalSkipNetwork -count=1` and a committed diff (see `.github/workflows/update-doctor-snapshot.yml`).

## Windows

Fresh-install CI covers **Linux and macOS** only. Windows install paths are not in this matrix; track separately if added.

## Related

- `scripts/ci-fresh-install.sh` — scripted smoke used in CI.
- [doctor-config-validation.md](./doctor-config-validation.md) — doctor behavior.
- [doctor-json-schema.md](./doctor-json-schema.md) — JSON output for automation.
