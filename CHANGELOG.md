# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Fixed

- **Local-path / hostPath usage** (cluster-services-1381): kubelet has no volume stats for these
  volumes, and the exec fallback's `df` measured the whole node filesystem (every local-path
  cluster on a1-ops-prd read the same 73.7% of ~1Ti). When the filesystem under the mount is more
  than 10% larger than the PVC, usage is now `du -sx` of the volume's own directory against the
  PVC size.
- **Recovery-point check was inverted**: it alerted when the FIRST recoverability point (the start
  of the retention window, ~= retention) was older than `maxRecoveryPointAgeHours`, so it fired
  for every healthy cluster. It now checks the LATEST recoverable point: the later of the last
  archived WAL (`status.lastArchivedWALTime`, or `pg_stat_archiver` on the primary for
  barman-cloud plugin clusters) and the last successful backup. New status fields
  `lastArchivedWALTime` and `lastRecoverablePoint`; apply the updated CRD.
- **Alertmanager flapping**: alerts now carry `endsAt = now + 15m` (3x the 5m resend window), so
  they no longer resolve at Alertmanager's 5m `resolve_timeout` just before the next send.
  Drifting details (`usage_percent`, `issue_N`, `issue_count`) are annotations instead of labels,
  so an alert keeps one fingerprint; `threshold`, `alert_type` and `policy` stay labels. Storage
  alerts carry `alert_type=storage`, and storage and backup alerts of equal severity no longer
  suppress each other.
- **Suppressed alerts were logged as sent**: `SendAlert` returns `ErrAlertSuppressed` for them,
  and a send that no channel accepted is no longer suppressed (it is retried next reconcile).
- **New metric** `cnpg_storage_manager_alerts_failed_total{cluster,namespace,severity,channel}`.

- **Deterministic policy selection**: a cluster matched by several StoragePolicies was processed
  by every one of them, and its `policy-name` annotation flipped between them on each reconcile.
  Each cluster is now managed by one policy only: highest `spec.priority`, then the most specific
  selector, then `namespace/name` order. New optional field `spec.priority` (default 0); apply
  the updated CRD before setting it.

## [0.1.0] - 2026-02-08

### Added

- **StoragePolicy CRD**: Define per-cluster or per-namespace storage management policies
  - Configurable warning/critical/emergency thresholds
  - PVC expansion settings with increment and max size limits
  - WAL cleanup configuration with retain count
  - Circuit breaker for failure protection
  - Multi-channel alerting (Alertmanager, Slack, PagerDuty)

- **StorageEvent CRD**: Audit trail for storage-related events
  - Tracks expansion and WAL cleanup operations
  - Per-PVC status tracking
  - Detailed event metadata

- **CNPG Cluster Discovery**: Automatic discovery of CloudNativePG clusters
  - Unstructured client for dynamic resource access
  - Support for both legacy and barman-cloud plugin backup configurations
  - ObjectStore CRD integration for backup status

- **Metrics Collection**: Kubelet volume stats integration
  - Real-time PVC usage monitoring
  - Prometheus metrics exposition
  - Per-cluster and per-PVC metrics

- **Threshold Evaluation Engine**: Policy-based threshold evaluation
  - Multi-level threshold support (warning/critical/emergency)
  - Action recommendations based on severity
  - Backup status evaluation with ObjectStore fallback

- **Cluster Annotation Management**: Non-invasive CNPG coordination
  - Managed/paused state tracking
  - Expansion and WAL cleanup coordination
  - Circuit breaker state persistence

- **Alerting Integration**: Multi-channel alert delivery
  - Prometheus Alertmanager support
  - Slack webhook integration
  - PagerDuty Events API v2 integration
  - Alert suppression to prevent duplicates

- **PVC Expansion Engine**: Automated storage expansion
  - StorageClass validation for expansion support
  - Preflight checks before expansion
  - Dry-run mode for testing

- **WAL Cleanup Engine**: Automated WAL file management
  - pg_archivecleanup execution via pod exec
  - Configurable retain count
  - Safe cleanup with backup verification

- **Helm Chart**: Production-ready deployment
  - Complete RBAC configuration
  - ServiceMonitor for Prometheus integration
  - Configurable resource limits
  - Dry-run mode values

- **CI/CD Pipeline**: Comprehensive automation
  - golangci-lint for code quality
  - Unit and integration tests
  - Security scanning (Gosec, Trivy)
  - Multi-arch container builds (amd64, arm64, s390x, ppc64le)
  - Automated releases with changelog generation

### Fixed

- **Backup monitoring false positives**: Fixed issue where clusters using the barman-cloud plugin reported false-positive backup alerts. The controller now reads backup timestamps from ObjectStore CRD's `.status.serverRecoveryWindow` when the barman-cloud plugin is configured. (#1)

[0.1.0]: https://github.com/supporttools/cnpg-storage-manager/releases/tag/v0.1.0
