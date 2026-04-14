## Summary

- Describe what changed and why.

## Validation

- [ ] `go test ./internal/channels/adapter -count=1`
- [ ] `go test ./... -count=1`
- [ ] `go build -mod=vendor -tags fts5 ./cmd/assistclaw`

## Adapter Checklist (required for channel work)

If this PR touches channel adapters, confirm the checklist in:

- `CONTRIBUTING.md` -> `New Channel Adapter Checklist`

And mark applicable items:

- [ ] Adapter v1 contract + typed errors
- [ ] Shared reliability wrapper wiring (retry/backoff/breaker/DLQ)
- [ ] Capability registry + capability doc update
- [ ] Contract/property tests pass
- [ ] Changelog updated
- [ ] Rollback note included (for behavioral migrations)

## Operational / Manual Follow-ups

- [ ] Branch protection includes required checks (`CI`, `Adapter Contract Tests`)
