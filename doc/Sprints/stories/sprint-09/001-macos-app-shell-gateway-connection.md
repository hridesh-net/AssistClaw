# STORY-050 — macOS companion: app shell and gateway connection

| Field | Value |
|-------|--------|
| **Sprint** | sprint-09 |
| **Type** | Feature / Client |
| **Priority** | P0 |
| **Estimate** | L |

## Summary

Deliver **macOS menu bar app** MVP: configure gateway URL/token, connection status, reconnect with exponential backoff, local secure storage for credentials (Keychain).

## User story

**As a** macOS user**  
**I want** a native companion**  
**So that** I can monitor and talk to AssistClaw without the terminal.

## Scope

### In scope

- SwiftUI or agreed stack; signing pipeline documented.
- TLS verification; optional self-signed CA import guidance.

### Out of scope

- Mac App Store release (stretch).

## Acceptance criteria

1. **Connect** to gateway WebSocket/HTTP with same auth model as web UI.
2. **Keychain** stores token; never prints to logs.
3. **Offline** state visible; user guidance to start gateway.
4. **Tests**: unit tests for networking client; UI tests for connect/disconnect.
5. **Crash reporting** integrated (Sentry or similar) behind opt-in.

## Definition of Done

- [ ] Beta build distributed to internal testers (STORY-055).

## Dependencies

- Gateway protocol stability.

## Risks

- Code signing; document dev vs prod builds.
