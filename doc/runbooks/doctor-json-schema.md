# `assistclaw doctor --json` — machine-readable output

Use **`assistclaw doctor --json`** in CI, scripts, and support playbooks. Valid JSON is written to **stdout** only. Config load failures (missing file permissions, invalid YAML) print a human-readable message to **stderr** and exit **2** without emitting the JSON envelope.

## Schema (`schema_version` 1)

Top-level object:

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `schema_version` | integer | yes | Current version is **1**. Bump when breaking changes are introduced. |
| `config_path` | string | no | Absolute or resolved path to the config file when one was read; omitted when doctor falls back to env-only defaults (no file). |
| `exit_code` | integer | yes | Same meaning as the process exit code: **0** all clear, **1** at least one warning, **2** at least one error. |
| `checks` | array | yes | Ordered list of check results. |

Each element of `checks`:

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `id` | string | yes | Stable identifier (e.g. `config.validate`, `provider.ollama`, `channel.telegram`). |
| `severity` | string | yes | One of: `ok`, `warn`, `error`, `skipped`. |
| `message` | string | yes | Human-readable summary (secrets are redacted where applicable). |
| `details` | object | no | Optional string map for structured metadata (e.g. `file`, `line`, `column` for YAML issues, or `config_path` for the validated file). |

## Backward compatibility

- **Additive** changes (new optional top-level fields, new optional keys inside `details`, new check `id` values) may ship without bumping `schema_version`.
- **Breaking** changes (renaming fields, changing types, redefining `severity` values) require incrementing **`schema_version`** and documenting the migration in this file and in release notes.

## Non-interactive use

- **`--no-input`** and **`--non-interactive`** are aliases. Today, doctor does not read stdin; the flags are reserved so future checks can fail fast in automation if interactive input were ever required.
- Combine with **`--skip-network`** in air-gapped CI to avoid outbound LLM and chat API calls.

## Example `jq` queries (support / CI)

Assume output is stored or piped from doctor:

```bash
# Fail CI if any check is an error (after doctor exits, also trust exit_code)
assistclaw doctor --config ./assistclaw.yaml --skip-network --json | jq -e '.checks | map(select(.severity == "error")) | length == 0'

# Print only failing or warning checks
assistclaw doctor --config ./assistclaw.yaml --skip-network --json | jq '.checks[] | select(.severity == "warn" or .severity == "error")'

# Schema version gate
assistclaw doctor --config ./assistclaw.yaml --skip-network --json | jq -e '.schema_version >= 1'

# Machine-readable exit from JSON (matches process exit when doctor completes JSON write)
assistclaw doctor --config ./assistclaw.yaml --skip-network --json | jq '.exit_code'

# Config path used (when set)
assistclaw doctor --config ./assistclaw.yaml --skip-network --json | jq -r '.config_path // empty'
```

## Related

- [doctor-config-validation.md](./doctor-config-validation.md) — what each check means and exit codes.
- Source types: `doctorOutput` / `doctorCheck` in `cmd/assistclaw/doctor_cmd.go`.
