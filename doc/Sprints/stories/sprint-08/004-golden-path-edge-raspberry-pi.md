# STORY-046 — Golden path documentation: edge (Raspberry Pi)

| Field | Value |
|-------|--------|
| **Sprint** | sprint-08 |
| **Type** | Documentation |
| **Priority** | P1 |
| **Estimate** | M |

## Summary

**Raspberry Pi** edge guide: ARM64 binary, memory limits, optional sensing build, systemd, SD card wear, thermal throttling, power loss, remote access via Tailscale/VPN (reference only).

## User story

**As a** hobbyist or edge operator**  
**I want** Pi-specific guidance**  
**So that** the assistant runs reliably 24/7.

## Acceptance criteria

1. **Hardware** requirements table (Pi 4/5, RAM).
2. **Performance** expectations: latency, concurrent sessions.
3. **Sensing** optional module called out with `SKIP_SENSING` (README alignment).
4. **Common failures**: OOM, slow disk, Wi-Fi drops.
5. **Tested** on real Pi hardware (evidence).

## Definition of Done

- [ ] Linked from README “Platforms.”

## Dependencies

- Hardware sensing docs if any.

## Risks

- User expectations vs device limits.
