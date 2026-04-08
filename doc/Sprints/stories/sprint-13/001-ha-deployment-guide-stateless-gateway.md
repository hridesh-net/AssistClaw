# STORY-069 — HA deployment guide: stateless gateway, queues, scaling

| Field | Value |
|-------|--------|
| **Sprint** | sprint-13 |
| **Type** | Enterprise / Ops |
| **Priority** | P0 |
| **Estimate** | L |

## Summary

Document and validate a **high availability** deployment: multiple gateway instances behind load balancer, sticky requirements for WebSockets, shared durable queue/DB for session state, graceful shutdown.

## User story

**As an** SRE**  
**I want** an HA reference**  
**So that** we meet uptime targets.

## Acceptance criteria

1. **Architecture** doc with diagrams and component list.
2. **Session affinity** strategy documented for WS vs HTTP.
3. **Failure scenarios**: single node loss, DB failover expectations.
4. **Load test** evidence for N replicas (define N).
5. **Sizing** guidance: CPU/RAM per 1k concurrent sessions (estimate ranges).

## Definition of Done

- [ ] Reviewed by someone who deployed it on k8s or VMs.

## Dependencies

- Prior sprints for metrics and session model clarity.

## Risks

- Stateful WS complexity; may require sticky sessions or session migration.
