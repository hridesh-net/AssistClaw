# Sprint stories index

Stories live under `sprint-NN/` as Markdown tickets. Naming: `NNN-kebab-case-title.md`. Global story IDs **STORY-001**–**STORY-074** are used for cross-references between tickets.

| Sprint | Theme | Stories |
|--------|--------|---------|
| [sprint-01](sprint-01/) | Adapter foundations & reliability | STORY-001–006 |
| [sprint-02](sprint-02/) | Observability & SLOs | STORY-007–012 |
| [sprint-03](sprint-03/) | Onboarding v2 & doctor | STORY-013–018 |
| [sprint-04](sprint-04/) | Security & enterprise prep | STORY-019–024 |
| [sprint-05](sprint-05/) | Microsoft Teams channel | STORY-025–030 |
| [sprint-06](sprint-06/) | Google Chat & parity matrix | STORY-031–036 |
| [sprint-07](sprint-07/) | Web / gateway operator UX | STORY-037–042 |
| [sprint-08](sprint-08/) | Docs golden paths & CI | STORY-043–049 |
| [sprint-09](sprint-09/) | macOS companion (beta) | STORY-050–055 |
| [sprint-10](sprint-10/) | Mobile companion (alpha) | STORY-056–060 |
| [sprint-11](sprint-11/) | Enterprise SSO & RBAC | STORY-061–064 |
| [sprint-12](sprint-12/) | Policy engine & tenant isolation | STORY-065–068 |
| [sprint-13](sprint-13/) | HA, DR, SIEM, compliance | STORY-069–074 |

Parent roadmap: [doc/Sprints/future-updates-and-releases.md](../../doc/Sprints/future-updates-and-releases.md).

## sprint-01 — files

- `001-channel-adapter-interface.md` (STORY-001)
- `002-outbound-reliability-retries-dlq.md` (STORY-002)
- `003-channel-capability-registry.md` (STORY-003)
- `004-migrate-first-channel-to-adapter.md` (STORY-004)
- `005-adapter-contract-and-property-tests.md` (STORY-005)
- `006-adapter-checklist-and-contributor-docs.md` (STORY-006)

## sprint-02 — files

- `001-structured-logging-correlation-ids.md` (STORY-007)
- `002-metrics-slo-indicators.md` (STORY-008)
- `003-opentelemetry-tracing-optional.md` (STORY-009)
- `004-load-test-and-failure-injection.md` (STORY-010)
- `005-slo-document-and-budgets.md` (STORY-011)
- `006-on-call-runbook-dashboards-alerts.md` (STORY-012)

## sprint-03 — files

- `001-doctor-config-validation.md` (STORY-013)
- `002-doctor-provider-reachability.md` (STORY-014)
- `003-doctor-channel-token-checks.md` (STORY-015)
- `004-doctor-noninteractive-json.md` (STORY-016)
- `005-preflight-before-start-daemon.md` (STORY-017)
- `006-fresh-install-ci-and-ttfm-measurement.md` (STORY-018)

## sprint-04 — files

- `001-per-channel-allowlists-and-pairing.md` (STORY-019)
- `002-audit-channel-lifecycle-and-policy-denials.md` (STORY-020)
- `003-secrets-redaction-in-logs.md` (STORY-021)
- `004-secrets-management-strategy-doc.md` (STORY-022)
- `005-security-regression-injection-suite.md` (STORY-023)
- `006-pilot-enterprise-security-checklist-signoff.md` (STORY-024)

## sprint-05 — files

- `001-teams-bot-foundation-auth.md` (STORY-025)
- `002-teams-messaging-and-threading-phase-1.md` (STORY-026)
- `003-teams-attachments-phase-1.md` (STORY-027)
- `004-teams-config-onboarding-and-yaml-schema.md` (STORY-028)
- `005-teams-e2e-sandbox-and-failure-scenarios.md` (STORY-029)
- `006-teams-user-documentation-and-limitations.md` (STORY-030)

## sprint-06 — files

- `001-google-chat-app-and-oauth-scopes.md` (STORY-031)
- `002-google-chat-message-mapping-and-threading.md` (STORY-032)
- `003-google-chat-group-space-policy-model.md` (STORY-033)
- `004-google-chat-e2e-and-permission-denial-tests.md` (STORY-034)
- `005-cross-channel-parity-matrix.md` (STORY-035)
- `006-google-chat-operator-documentation.md` (STORY-036)

## sprint-07 — files

- `001-web-operator-session-list-and-detail.md` (STORY-037)
- `002-web-channel-health-dashboard.md` (STORY-038)
- `003-web-recent-errors-and-log-stream.md` (STORY-039)
- `004-web-model-usage-and-cost-footprint.md` (STORY-040)
- `005-web-safe-actions-placeholder-rbac-future.md` (STORY-041)
- `006-web-e2e-accessibility-and-support-handoff.md` (STORY-042)

## sprint-08 — files

- `001-golden-path-small-team.md` (STORY-043)
- `002-golden-path-enterprise-pilot.md` (STORY-044)
- `003-golden-path-air-gapped.md` (STORY-045)
- `004-golden-path-edge-raspberry-pi.md` (STORY-046)
- `005-golden-path-cloud-hybrid.md` (STORY-047)
- `006-docs-ci-link-check-and-snippet-validation.md` (STORY-048)
- `007-docs-release-process-and-quarterly-review.md` (STORY-049)

## sprint-09 — files

- `001-macos-app-shell-gateway-connection.md` (STORY-050)
- `002-macos-session-status-and-quick-actions.md` (STORY-051)
- `003-macos-notifications-inbound-messages.md` (STORY-052)
- `004-macos-secure-device-pairing.md` (STORY-053)
- `005-macos-beta-distribution-crash-reporting.md` (STORY-054)
- `006-macos-reliability-sleep-vpn-reconnect-tests.md` (STORY-055)

## sprint-10 — files

- `001-mobile-auth-and-device-pairing.md` (STORY-056)
- `002-mobile-chat-list-and-send-receive.md` (STORY-057)
- `003-mobile-push-notifications.md` (STORY-058)
- `004-mobile-attachments-and-policy-limits.md` (STORY-059)
- `005-mobile-device-lab-and-alpha-exit-criteria.md` (STORY-060)

## sprint-11 — files

- `001-oidc-sso-for-web-admin.md` (STORY-061)
- `002-rbac-roles-and-resource-scopes.md` (STORY-062)
- `003-audit-admin-actions-and-config-changes.md` (STORY-063)
- `004-sso-testing-and-revocation-scenarios.md` (STORY-064)

## sprint-12 — files

- `001-policy-engine-v1-tool-and-channel-rules.md` (STORY-065)
- `002-multi-tenant-isolation-data-stores.md` (STORY-066)
- `003-policy-evaluation-performance-tests.md` (STORY-067)
- `004-isolation-guarantees-and-customer-facing-doc.md` (STORY-068)

## sprint-13 — files

- `001-ha-deployment-guide-stateless-gateway.md` (STORY-069)
- `002-backup-restore-and-dr-drill.md` (STORY-070)
- `003-siem-export-splunk-datadog-jsonl.md` (STORY-071)
- `004-secrets-vault-and-cloud-kms-integrations.md` (STORY-072)
- `005-security-pipeline-pen-test-and-dependency-audits.md` (STORY-073)
- `006-soc2-path-sla-mttr-and-enterprise-skus.md` (STORY-074)
