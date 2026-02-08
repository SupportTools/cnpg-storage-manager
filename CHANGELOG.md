# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

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
