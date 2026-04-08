# STORY-045 — Golden path documentation: air-gapped / offline

| Field | Value |
|-------|--------|
| **Sprint** | sprint-08 |
| **Type** | Documentation |
| **Priority** | P1 |
| **Estimate** | M |

## Summary

Document **offline** deployment: Ollama/local models, no provider doctor checks, artifact transfer, hash verification, update policy without internet.

## User story

**As a** defense/industry customer**  
**I want** air-gapped guidance**  
**So that** we can deploy safely.

## Acceptance criteria

1. **Procedure** for binary install without `curl | bash` if required alternative documented.
2. **`doctor --skip-network`** behavior explained (Sprint 3).
3. **Threat** considerations: USB supply chain, signature verification.
4. **Limitations**: what features require internet (explicit).
5. **Validation** checklist.

## Definition of Done

- [ ] Legal/compliance review if targeting regulated industries (optional).

## Dependencies

- STORY-016.

## Risks

- Incomplete feature set; set expectations clearly.
