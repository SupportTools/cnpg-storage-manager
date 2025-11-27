# Project Request Document: CNPG Storage Manager

**Document Version**: 1.1
**Date**: 2025-11-26
**Author**: Senior Product Manager
**Project Code**: CSM-2025-001
**Revision**: Technical design refinements based on architecture review

---

## Executive Summary

The CNPG Storage Manager is a Kubernetes controller designed to prevent database outages caused by storage exhaustion in CloudNativePG (CNPG) PostgreSQL clusters. This project addresses a critical operational gap where WAL archiving failures can lead to disk space exhaustion and database crashes requiring manual intervention.

### Business Impact
- **Problem Cost**: 4+ hours downtime per incident, requiring DevOps engineer intervention
- **Solution Value**: Automated prevention and remediation, reducing MTTR from hours to minutes
- **Risk Mitigation**: Prevents data loss scenarios and service degradation

---

## Problem Statement

### Incident Analysis
During a recent production incident, a CNPG PostgreSQL cluster experienced complete failure due to:
1. WAL archiving to S3 failed (connectivity/timeline issues)
2. WAL files accumulated on 10Gi PVC until disk was full
3. Database crashed and became unresponsive
4. Manual recovery required PVC expansion and S3 path reconfiguration

### Root Cause Categories
1. **External Dependencies**: S3 connectivity issues prevent WAL archiving
2. **Storage Constraints**: Fixed PVC sizes cannot handle burst write patterns
3. **Monitoring Gaps**: No automated detection of storage pressure
4. **Manual Remediation**: Recovery requires expert knowledge and manual steps

### Current State Pain Points
- No proactive monitoring of CNPG storage usage
- Manual PVC expansion during emergencies
- No automated WAL cleanup mechanisms
- Alert fatigue from generic disk space warnings
- Lack of CNPG-specific storage intelligence

---

## Proposed Solution

### Product Vision
A Kubernetes-native controller that provides intelligent, automated storage management for CNPG PostgreSQL clusters, preventing outages through proactive monitoring, alerting, and remediation.

### Core Value Proposition
**For DevOps teams** who manage PostgreSQL databases, **the CNPG Storage Manager** is a **Kubernetes controller** that **automatically prevents storage-related database outages** unlike **manual monitoring and intervention** our solution **provides intelligent automation and CNPG-specific remediation**.

### Key Capabilities

#### 1. Intelligent Monitoring
- Continuously monitors PVC usage for all CNPG clusters
- Tracks WAL file accumulation patterns
- Correlates storage metrics with CNPG cluster health
- Predicts storage exhaustion based on growth trends

#### 2. Automated Remediation
- Auto-expands PVCs when configurable thresholds are reached
- Triggers emergency WAL cleanup procedures
- Coordinates with CNPG operator for safe operations
- Supports rollback mechanisms for failed expansions

#### 3. Proactive Alerting
- Context-aware notifications with CNPG cluster details
- Multi-channel alerting (Prometheus/Alertmanager, Slack, PagerDuty)
- Escalation policies based on severity levels
- Alert suppression during active remediation

#### 4. Policy-Driven Operations
- Configurable per-cluster or per-namespace policies
- Support for different storage classes and expansion rules
- Maintenance windows and operational constraints
- Audit logging for all automated actions

---

## User Stories & Requirements

### Epic 1: Core Monitoring Infrastructure

#### Story 1.1: Storage Metrics Collection
**As a** cluster administrator
**I want** the controller to monitor PVC usage for all CNPG clusters
**So that** I have visibility into storage consumption patterns

**Acceptance Criteria:**
- Controller discovers all CNPG clusters automatically
- Collects volume metrics from kubelet every 30 seconds
- Stores metrics in controller's internal state
- Exposes custom metrics for Prometheus scraping
- Handles node failures and metric collection errors gracefully

#### Story 1.2: WAL File Tracking
**As a** database administrator
**I want** to monitor WAL file accumulation specifically
**So that** I can detect archiving failures early

**Acceptance Criteria:**
- Monitors WAL directory size within PostgreSQL pods
- Tracks WAL generation rate and archiving success rate
- Identifies when WAL files are not being archived
- Correlates WAL accumulation with PVC usage growth

### Epic 2: Automated Storage Expansion

#### Story 2.1: Threshold-Based PVC Expansion
**As a** production engineer
**I want** PVCs to expand automatically when storage reaches critical levels
**So that** databases don't crash due to disk space exhaustion

**Acceptance Criteria:**
- Supports configurable expansion thresholds (default: 85%)
- Expands PVC by configurable percentage (default: 50%)
- Works only with storage classes supporting online expansion
- Validates expansion limits and cluster resource constraints
- Updates CNPG cluster storage specifications automatically

#### Story 2.2: Emergency Storage Management
**As a** on-call engineer
**I want** automatic emergency actions when expansion fails
**So that** I have time to respond before total failure

**Acceptance Criteria:**
- Attempts WAL file cleanup when expansion is insufficient
- Triggers PostgreSQL checkpoint to reduce WAL files
- Sends critical alerts with remediation instructions
- Provides emergency runbook integration
- Implements circuit breaker to prevent action loops

### Epic 3: Configuration & Policy Management

#### Story 3.1: Declarative Policy Configuration
**As a** platform engineer
**I want** to configure storage policies via Kubernetes resources
**So that** I can manage policies with GitOps workflows

**Acceptance Criteria:**
- Supports StoragePolicy CRD for cluster-specific rules
- Allows namespace-level and cluster-level defaults
- Includes validation webhooks for policy conflicts
- Provides policy dry-run and testing capabilities
- Supports policy inheritance and override patterns

### Epic 4: Alerting & Observability

#### Story 4.1: Intelligent Alerting
**As a** database reliability engineer
**I want** contextual alerts about storage issues
**So that** I receive actionable information instead of generic disk alerts

**Acceptance Criteria:**
- Sends alerts with CNPG cluster context and remediation status
- Supports multiple notification channels (Slack, PagerDuty, email)
- Implements alert suppression during active remediation
- Provides alert escalation based on severity and response time
- Includes relevant metrics and trends in alert payload

---

## Technical Design Overview

### Key Architecture Decisions

| Decision | Choice | Rationale |
|----------|--------|-----------|
| CNPG Coordination | Annotation-based | Non-invasive; CNPG operator reads annotations for expansion hints |
| WAL Cleanup Method | pg_archivecleanup | PostgreSQL-native tool with proper retention awareness |
| Storage Backend | Generic CSI | Any storage class with `allowVolumeExpansion=true` |
| Multi-Instance Handling | Expand all PVCs | Ensures consistent storage across primary and replicas |
| Policy Conflicts | Reject overlapping | Validation webhook prevents ambiguous configurations |
| Automation Mode | Fully automated | No human approval gates; immediate action on threshold breach |
| State Persistence | Controller PVC | Historical metrics survive controller restarts |
| API Group | `cnpg.supporttools.io` | Clear ownership distinction from CNPG project |
| Pod Execution | client-go exec | Direct exec into pods for pg_archivecleanup |
| Resize Failure | Alert and wait | Conservative approach; no automatic pod restarts |
| Manual Override | Annotation + Policy | Both cluster annotation and policy exclusion supported |
| Kubernetes Version | 1.25+ | Stable CSI volume expansion support |

### Architecture Components

#### 1. CNPG Storage Manager Controller
```go
// Primary controller watching CNPG clusters and PVCs
type CNPGStorageController struct {
    client.Client
    Scheme       *runtime.Scheme
    Recorder     record.EventRecorder
    Metrics      *StorageMetrics
    PolicyMgr    *PolicyManager
    Alerter      *AlertManager
    StateStore   *PersistentStateStore  // Metrics history on PVC
    ExecClient   *PodExecutor           // For pg_archivecleanup
}

// PodExecutor handles secure command execution in CNPG pods
type PodExecutor struct {
    client    kubernetes.Interface
    config    *rest.Config
    rateLimit *rate.Limiter
}
```

#### 2. CNPG Operator Coordination

The controller coordinates with CNPG operator via annotations on the CNPG Cluster CR:

##### Annotation Schema
```yaml
# Annotations added to CNPG Cluster CR by this controller
metadata:
  annotations:
    # Management status
    storage.cnpg.supporttools.io/managed: "true"           # Controller is managing this cluster
    storage.cnpg.supporttools.io/paused: "false"           # Pause automation (manual override)

    # Expansion requests (read by expansion logic)
    storage.cnpg.supporttools.io/target-size: "15Gi"       # Requested expansion size
    storage.cnpg.supporttools.io/expansion-requested: "2025-11-26T10:30:00Z"
    storage.cnpg.supporttools.io/expansion-reason: "threshold-breach-85"

    # Status tracking
    storage.cnpg.supporttools.io/last-check: "2025-11-26T10:30:00Z"
    storage.cnpg.supporttools.io/current-usage-percent: "87"
    storage.cnpg.supporttools.io/wal-cleanup-last: "2025-11-26T09:00:00Z"
```

##### Manual Override Annotations
```yaml
# To pause management on a specific cluster:
metadata:
  annotations:
    storage.cnpg.supporttools.io/paused: "true"
    storage.cnpg.supporttools.io/pause-reason: "manual-maintenance"
    storage.cnpg.supporttools.io/pause-until: "2025-11-27T00:00:00Z"  # Optional auto-resume
```

#### 3. Custom Resource Definitions

##### StoragePolicy CRD (Full Specification)
```yaml
apiVersion: cnpg.supporttools.io/v1alpha1
kind: StoragePolicy
metadata:
  name: production-postgres
  namespace: database
spec:
  # Cluster selection - must not overlap with other policies
  selector:
    matchLabels:
      environment: "production"
    matchExpressions:
      - key: cnpg.io/cluster
        operator: Exists

  # Exclude specific clusters even if they match selector
  excludeClusters:
    - name: legacy-postgres
      namespace: database

  # Threshold configuration (percentage of PVC capacity)
  thresholds:
    warning: 70       # Generate warning alert
    critical: 80      # Generate critical alert
    expansion: 85     # Trigger automatic expansion
    emergency: 90     # Trigger WAL cleanup

  # Expansion settings
  expansion:
    enabled: true
    percentage: 50              # Expand by this percentage
    minIncrementGi: 5           # Minimum expansion size
    maxSize: 100Gi              # Hard limit - alert when reached
    cooldownMinutes: 30         # Minimum time between expansions

  # WAL cleanup settings (pg_archivecleanup)
  walCleanup:
    enabled: true
    retainCount: 10             # Keep at least N WAL files
    requireArchived: true       # Only clean files confirmed archived
    cooldownMinutes: 15         # Minimum time between cleanups

  # Circuit breaker configuration
  circuitBreaker:
    maxFailures: 3              # Failures before circuit opens
    resetMinutes: 60            # Time before circuit resets
    scope: "per-cluster"        # "per-cluster" or "global"

  # Alerting configuration
  alerting:
    channels:
      - type: alertmanager
        endpoint: "http://alertmanager:9093"
      - type: slack
        webhookSecret: "slack-webhook-secret"
        channel: "#db-alerts"
    suppressDuringRemediation: true
    escalationMinutes: 15       # Re-alert if unresolved

status:
  # Policy status (managed by controller)
  conditions:
    - type: Active
      status: "True"
      lastTransitionTime: "2025-11-26T10:00:00Z"
      reason: "PolicyApplied"
      message: "Policy is active and monitoring 5 clusters"
    - type: Conflicting
      status: "False"
      lastTransitionTime: "2025-11-26T10:00:00Z"
  managedClusters: 5
  lastEvaluated: "2025-11-26T10:30:00Z"
  observedGeneration: 1
```

##### StorageEvent CRD (Full Specification)
```yaml
apiVersion: cnpg.supporttools.io/v1alpha1
kind: StorageEvent
metadata:
  name: production-postgres-expansion-20251126-103000
  namespace: database
  labels:
    cnpg.supporttools.io/cluster: production-postgres
    cnpg.supporttools.io/event-type: expansion
  ownerReferences:
    - apiVersion: postgresql.cnpg.io/v1
      kind: Cluster
      name: production-postgres
      uid: <cluster-uid>
spec:
  clusterRef:
    name: production-postgres
    namespace: database
  eventType: expansion          # expansion | wal-cleanup | alert | circuit-breaker
  trigger: threshold-breach     # threshold-breach | manual | scheduled

  # For expansion events
  expansion:
    originalSize: 10Gi
    requestedSize: 15Gi
    affectedPVCs:
      - name: production-postgres-1
        node: worker-1
      - name: production-postgres-2
        node: worker-2
      - name: production-postgres-3
        node: worker-3

  # For WAL cleanup events
  walCleanup:
    filesRemoved: 25
    spaceFreedBytes: 419430400
    oldestRetained: "000000010000000000000050"

status:
  phase: Completed              # Pending | InProgress | Completed | Failed
  startTime: "2025-11-26T10:30:00Z"
  completionTime: "2025-11-26T10:31:30Z"

  # Per-PVC status for expansion
  pvcStatuses:
    - name: production-postgres-1
      phase: Completed
      originalSize: 10Gi
      newSize: 15Gi
      filesystemResized: true
    - name: production-postgres-2
      phase: Completed
      originalSize: 10Gi
      newSize: 15Gi
      filesystemResized: true
    - name: production-postgres-3
      phase: Failed
      originalSize: 10Gi
      error: "filesystem resize failed: timeout waiting for resize"

  conditions:
    - type: Complete
      status: "False"
      reason: "PartialFailure"
      message: "2/3 PVCs expanded successfully"

  # Retry information
  retryCount: 0
  nextRetryTime: null
```

#### 4. Core Components

##### Metrics Collector
- Integrates with kubelet volume stats API (`/stats/summary`)
- Collects CNPG-specific metrics from pods via CNPG status
- Persists metrics history to controller PVC for trend analysis
- Provides Prometheus metrics endpoint at `/metrics`
- Handles metric aggregation across all instances in a cluster
- Rate-limited API calls to prevent kubelet overload

##### Policy Engine
- Evaluates storage policies against current state
- Validates no policy overlap via admission webhook
- Determines required actions based on thresholds
- Implements safety checks (storage class capability, maxSize limits)
- Supports policy dry-run mode via `spec.dryRun: true`
- Handles policy inheritance: cluster-specific > namespace-default

##### Remediation Engine
- Executes PVC expansion via `PersistentVolumeClaim` patch
- Expands ALL PVCs in a cluster when any instance hits threshold
- Sequential expansion with configurable parallelism
- Coordinates filesystem resize verification
- If filesystem resize fails: alert and wait (no pod restart)
- Implements idempotent operation guarantees via StorageEvent tracking

##### WAL Cleanup Engine
- Executes `pg_archivecleanup` via client-go pod exec
- Verifies WAL archiving status before cleanup
- Respects `retainCount` minimum from policy
- Only cleans WAL files confirmed as archived
- Triggers PostgreSQL CHECKPOINT before cleanup if beneficial

##### Alert Manager
- Integrates with Prometheus Alertmanager via webhook
- Supports direct Slack/PagerDuty webhooks
- Implements alert suppression during active remediation
- Provides alert deduplication by cluster+type
- Escalation on unresolved issues after configurable interval

##### Circuit Breaker
- Prevents action loops on persistently failing clusters
- Configurable failure threshold (default: 3 failures)
- Per-cluster scope (one cluster failing doesn't affect others)
- Auto-reset after configurable interval
- Manual reset via annotation: `storage.cnpg.supporttools.io/reset-circuit-breaker: "true"`

### Controller State Persistence

The controller uses a PVC for persistent state:

```yaml
# Controller StatefulSet PVC
apiVersion: v1
kind: PersistentVolumeClaim
metadata:
  name: cnpg-storage-manager-state
spec:
  accessModes: ["ReadWriteOnce"]
  resources:
    requests:
      storage: 1Gi
  storageClassName: standard  # Configurable via Helm
```

**Stored Data:**
- Metrics history (last 24 hours, 30-second intervals)
- Trend analysis data for growth prediction
- Circuit breaker state
- In-progress operation state for crash recovery

**Retention Policy:**
- Metrics older than 24 hours are aggregated to hourly
- Hourly data older than 7 days is purged
- StorageEvent CRs are retained for 30 days (configurable)

### Technology Stack

#### Primary Technologies
- **Language**: Go 1.21+
- **Framework**: controller-runtime v0.16+
- **Build Tool**: kubebuilder v3.12+
- **Testing**: Ginkgo v2 + Gomega + envtest
- **Deployment**: Helm chart

#### Dependencies
- **kubernetes/client-go**: Kubernetes API interaction
- **prometheus/client_golang**: Metrics exposition
- **go-logr/logr**: Structured logging (JSON format)
- **cnpg.io/cloudnative-pg**: CNPG API types (v1.20+)

#### Integration Points
- **CloudNativePG Operator**: Cluster status via CR, coordination via annotations
- **Any CSI Driver**: Storage expansion via `allowVolumeExpansion: true`
- **Prometheus**: Metrics collection and alerting
- **Alertmanager**: Alert routing and notification

---

## RBAC Requirements

The controller requires the following Kubernetes RBAC permissions:

### ClusterRole Definition
```yaml
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  name: cnpg-storage-manager
rules:
  # CNPG Cluster access
  - apiGroups: ["postgresql.cnpg.io"]
    resources: ["clusters"]
    verbs: ["get", "list", "watch", "patch", "update"]
  - apiGroups: ["postgresql.cnpg.io"]
    resources: ["clusters/status"]
    verbs: ["get"]

  # PVC management
  - apiGroups: [""]
    resources: ["persistentvolumeclaims"]
    verbs: ["get", "list", "watch", "patch", "update"]

  # Pod access for metrics and exec
  - apiGroups: [""]
    resources: ["pods"]
    verbs: ["get", "list", "watch"]
  - apiGroups: [""]
    resources: ["pods/exec"]
    verbs: ["create"]

  # Node access for kubelet metrics
  - apiGroups: [""]
    resources: ["nodes"]
    verbs: ["get", "list"]
  - apiGroups: [""]
    resources: ["nodes/proxy"]
    verbs: ["get"]

  # Events for audit trail
  - apiGroups: [""]
    resources: ["events"]
    verbs: ["create", "patch"]

  # StorageClass validation
  - apiGroups: ["storage.k8s.io"]
    resources: ["storageclasses"]
    verbs: ["get", "list", "watch"]

  # Own CRDs
  - apiGroups: ["cnpg.supporttools.io"]
    resources: ["storagepolicies", "storageevents"]
    verbs: ["get", "list", "watch", "create", "update", "patch", "delete"]
  - apiGroups: ["cnpg.supporttools.io"]
    resources: ["storagepolicies/status", "storageevents/status"]
    verbs: ["get", "update", "patch"]

  # Secrets for alert webhook credentials
  - apiGroups: [""]
    resources: ["secrets"]
    verbs: ["get"]
    resourceNames: []  # Restricted to specific secrets via Helm values

  # Leader election
  - apiGroups: ["coordination.k8s.io"]
    resources: ["leases"]
    verbs: ["get", "list", "watch", "create", "update", "patch", "delete"]
```

### Security Considerations
- `pods/exec` permission is required for pg_archivecleanup - this is a privileged operation
- Node proxy access is needed for kubelet metrics API
- Secret access should be restricted to specific webhook credential secrets
- Consider namespace-scoped deployment for restricted environments

---

## Prometheus Metrics Specification

### Controller Metrics

All metrics are prefixed with `cnpg_storage_manager_`.

#### Storage Metrics
```
# PVC usage metrics (per PVC)
cnpg_storage_manager_pvc_usage_bytes{cluster, namespace, pvc, instance}
cnpg_storage_manager_pvc_capacity_bytes{cluster, namespace, pvc, instance}
cnpg_storage_manager_pvc_usage_percent{cluster, namespace, pvc, instance}

# WAL-specific metrics
cnpg_storage_manager_wal_directory_bytes{cluster, namespace, instance}
cnpg_storage_manager_wal_files_count{cluster, namespace, instance}
cnpg_storage_manager_wal_archiving_lag_seconds{cluster, namespace, instance}
```

#### Operation Metrics
```
# Expansion operations
cnpg_storage_manager_expansion_total{cluster, namespace, result}  # result: success|failure
cnpg_storage_manager_expansion_bytes_total{cluster, namespace}
cnpg_storage_manager_expansion_duration_seconds{cluster, namespace}
cnpg_storage_manager_expansion_in_progress{cluster, namespace}

# WAL cleanup operations
cnpg_storage_manager_wal_cleanup_total{cluster, namespace, result}
cnpg_storage_manager_wal_cleanup_files_removed_total{cluster, namespace}
cnpg_storage_manager_wal_cleanup_bytes_freed_total{cluster, namespace}
```

#### Controller Health Metrics
```
# Controller status
cnpg_storage_manager_clusters_managed_total{namespace}
cnpg_storage_manager_policies_active_total{namespace}
cnpg_storage_manager_reconcile_total{controller, result}
cnpg_storage_manager_reconcile_duration_seconds{controller}

# Circuit breaker status
cnpg_storage_manager_circuit_breaker_open{cluster, namespace}
cnpg_storage_manager_circuit_breaker_failures_total{cluster, namespace}

# Error tracking
cnpg_storage_manager_errors_total{type, cluster, namespace}
```

#### Alert Metrics
```
cnpg_storage_manager_alerts_sent_total{cluster, namespace, severity, channel}
cnpg_storage_manager_alerts_suppressed_total{cluster, namespace, reason}
```

### Grafana Dashboard

The Helm chart includes a ConfigMap with a Grafana dashboard providing:
- Cluster storage overview (usage %, capacity, growth rate)
- Expansion history timeline
- WAL accumulation trends
- Alert history
- Controller health status

---

## Helm Chart Specification

### values.yaml Structure
```yaml
# Image configuration
image:
  repository: ghcr.io/supporttools/cnpg-storage-manager
  tag: ""  # Defaults to chart appVersion
  pullPolicy: IfNotPresent

imagePullSecrets: []

# Controller configuration
controller:
  replicas: 1  # Single replica recommended (leader election enabled)

  resources:
    limits:
      cpu: 200m
      memory: 256Mi
    requests:
      cpu: 100m
      memory: 128Mi

  # Metrics collection interval
  metricsInterval: 30s

  # Log configuration
  logLevel: info  # debug, info, warn, error
  logFormat: json

  # Leader election
  leaderElection:
    enabled: true
    leaseDuration: 15s
    renewDeadline: 10s
    retryPeriod: 2s

# State persistence
persistence:
  enabled: true
  storageClass: ""  # Uses default if empty
  size: 1Gi
  accessMode: ReadWriteOnce

# Default policy settings (can be overridden per StoragePolicy)
defaults:
  thresholds:
    warning: 70
    critical: 80
    expansion: 85
    emergency: 90
  expansion:
    enabled: true
    percentage: 50
    minIncrementGi: 5
    cooldownMinutes: 30
  walCleanup:
    enabled: true
    retainCount: 10
    cooldownMinutes: 15
  circuitBreaker:
    maxFailures: 3
    resetMinutes: 60

# Alerting configuration
alerting:
  alertmanager:
    enabled: false
    endpoint: "http://alertmanager:9093"
  slack:
    enabled: false
    webhookSecretName: ""
    webhookSecretKey: "webhook-url"
    defaultChannel: "#alerts"
  pagerduty:
    enabled: false
    routingKeySecretName: ""
    routingKeySecretKey: "routing-key"

# Prometheus ServiceMonitor
serviceMonitor:
  enabled: false
  interval: 30s
  scrapeTimeout: 10s
  labels: {}

# Grafana dashboard ConfigMap
grafanaDashboard:
  enabled: false
  labels:
    grafana_dashboard: "1"

# RBAC configuration
rbac:
  create: true

serviceAccount:
  create: true
  name: ""
  annotations: {}

# Pod configuration
podAnnotations: {}
podLabels: {}
nodeSelector: {}
tolerations: []
affinity: {}

# Security context
podSecurityContext:
  runAsNonRoot: true
  runAsUser: 65534
  fsGroup: 65534

securityContext:
  allowPrivilegeEscalation: false
  readOnlyRootFilesystem: true
  capabilities:
    drop:
      - ALL

# Namespace restrictions (empty = all namespaces)
watchNamespaces: []

# Admission webhook for policy validation
webhook:
  enabled: true
  port: 9443
  certManager:
    enabled: true
    issuerRef:
      name: selfsigned-issuer
      kind: Issuer
```

### Installation Examples
```bash
# Basic installation
helm install cnpg-storage-manager ./charts/cnpg-storage-manager \
  --namespace cnpg-system \
  --create-namespace

# With Alertmanager integration
helm install cnpg-storage-manager ./charts/cnpg-storage-manager \
  --namespace cnpg-system \
  --set alerting.alertmanager.enabled=true \
  --set alerting.alertmanager.endpoint="http://prometheus-alertmanager:9093"

# With Slack alerts
helm install cnpg-storage-manager ./charts/cnpg-storage-manager \
  --namespace cnpg-system \
  --set alerting.slack.enabled=true \
  --set alerting.slack.webhookSecretName=slack-webhook \
  --set alerting.slack.defaultChannel="#db-alerts"

# Namespace-restricted deployment
helm install cnpg-storage-manager ./charts/cnpg-storage-manager \
  --namespace cnpg-system \
  --set watchNamespaces="{database,production}"
```

---

## Testing Strategy

### Test Pyramid

```
                    ┌─────────────┐
                    │   E2E Tests │  (~10%)
                    │  Real K8s   │
                    └─────────────┘
               ┌─────────────────────┐
               │  Integration Tests  │  (~30%)
               │     envtest         │
               └─────────────────────┘
          ┌───────────────────────────────┐
          │         Unit Tests            │  (~60%)
          │    Standard Go testing        │
          └───────────────────────────────┘
```

### Unit Tests
- **Framework**: Standard Go `testing` package with testify assertions
- **Coverage Target**: >85%
- **Focus Areas**:
  - Policy evaluation logic
  - Threshold calculations
  - Circuit breaker state machine
  - Metrics aggregation
  - Alert deduplication

```bash
# Run unit tests
go test -v -race -coverprofile=coverage.out ./pkg/...

# View coverage report
go tool cover -html=coverage.out
```

### Integration Tests (envtest)
- **Framework**: Ginkgo v2 + Gomega + controller-runtime envtest
- **Coverage Target**: All controller reconciliation paths
- **Focus Areas**:
  - StoragePolicy CRUD and validation
  - StorageEvent lifecycle
  - Policy conflict detection
  - Webhook validation

```bash
# Run integration tests
make test

# Run specific test suite
ginkgo -v -focus="StoragePolicy" ./controllers/...
```

### E2E Tests
- **Framework**: Ginkgo v2 with kind cluster
- **Environment**: kind cluster with CNPG operator installed
- **Focus Areas**:
  - Full expansion workflow
  - WAL cleanup execution
  - Alert delivery
  - Multi-cluster scenarios

```bash
# Setup test cluster
kind create cluster --config=test/e2e/kind-config.yaml
kubectl apply -f https://raw.githubusercontent.com/cloudnative-pg/cloudnative-pg/main/releases/cnpg-1.20.0.yaml

# Run E2E tests
make test-e2e
```

### Mock Implementations

#### Mock Storage Class
```yaml
# For testing expansion without real storage
apiVersion: storage.k8s.io/v1
kind: StorageClass
metadata:
  name: mock-expandable
provisioner: fake.csi.k8s.io
allowVolumeExpansion: true
```

#### Mock CNPG Cluster
```yaml
# Minimal CNPG cluster for testing
apiVersion: postgresql.cnpg.io/v1
kind: Cluster
metadata:
  name: test-cluster
spec:
  instances: 1
  storage:
    size: 1Gi
    storageClass: mock-expandable
```

### CI/CD Pipeline

```yaml
# .github/workflows/test.yaml
name: Test
on: [push, pull_request]
jobs:
  unit-test:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with:
          go-version: '1.21'
      - run: make test
      - uses: codecov/codecov-action@v3

  integration-test:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with:
          go-version: '1.21'
      - run: make test-integration

  e2e-test:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with:
          go-version: '1.21'
      - uses: helm/kind-action@v1
      - run: make test-e2e
```

---

## Success Criteria & KPIs

### Primary Success Metrics

#### 1. Incident Reduction
- **Target**: 90% reduction in storage-related database outages
- **Measurement**: Count of manual interventions required per month
- **Baseline**: 2-3 incidents per month requiring 4+ hour recovery time

#### 2. Response Time Improvement
- **Target**: MTTR < 5 minutes for storage-related issues
- **Measurement**: Time from threshold breach to resolution
- **Baseline**: 4+ hours for manual PVC expansion and recovery

#### 3. Automation Success Rate
- **Target**: 95% successful automatic remediation
- **Measurement**: Percentage of threshold breaches resolved without manual intervention
- **Baseline**: 0% (currently all manual)

### Secondary Success Metrics

#### 4. Alert Quality
- **Target**: <5% false positive alert rate
- **Measurement**: Alerts that don't require action / total alerts
- **Baseline**: 30-40% false positives from generic disk monitoring

#### 5. Storage Utilization Efficiency
- **Target**: Maintain 75-80% average storage utilization
- **Measurement**: Average PVC utilization across all CNPG clusters
- **Baseline**: 60% utilization due to over-provisioning for safety

### User Adoption Metrics

#### 6. Policy Coverage
- **Target**: 100% of production CNPG clusters managed by policies
- **Measurement**: Clusters with StoragePolicy / total CNPG clusters
- **Baseline**: 0% automated policy management

#### 7. Developer Self-Service
- **Target**: 80% of storage policy changes via GitOps (no ticket required)
- **Measurement**: Policy updates via Git commits vs. manual tickets
- **Baseline**: 100% manual ticket-based changes

---

## Implementation Phases

### Phase 1: MVP - Core Monitoring (4-6 weeks)
**Goal**: Establish foundation for storage monitoring and basic alerting

**Deliverables:**
- CNPG cluster discovery and PVC monitoring
- Basic threshold detection and alerting
- StoragePolicy CRD with validation
- Prometheus metrics exposition
- Basic Helm chart for deployment

**Acceptance Criteria:**
- Monitors all CNPG clusters in target namespaces
- Generates alerts at configurable thresholds
- Provides storage utilization metrics in Grafana
- Deployed and operational in staging environment

### Phase 2: Automated Expansion (6-8 weeks)
**Goal**: Implement automatic PVC expansion capabilities

**Deliverables:**
- PVC expansion engine with safety checks
- Integration with CSI storage classes supporting online expansion
- Expansion history and audit logging
- Enhanced alerting with expansion status
- Rollback mechanisms for failed expansions

**Acceptance Criteria:**
- Successfully expands PVCs automatically in staging
- Handles expansion failures gracefully
- Maintains database availability during expansion
- Provides detailed expansion audit trail

### Phase 3: Advanced Features (4-6 weeks)
**Goal**: Add emergency actions and enhanced policy management

**Deliverables:**
- WAL file cleanup and emergency actions
- Multi-cluster policy inheritance
- Advanced alerting with escalation
- Performance optimization and caching
- Comprehensive monitoring dashboard

**Acceptance Criteria:**
- Handles emergency storage situations automatically
- Supports complex policy scenarios
- Integrates with existing alerting infrastructure
- Meets performance requirements under load

### Phase 4: Production Hardening (3-4 weeks)
**Goal**: Prepare for production deployment and operations

**Deliverables:**
- Production-ready Helm chart with RBAC
- Comprehensive documentation and runbooks
- Load testing and performance validation
- Security scanning and compliance review
- Operator training materials

**Acceptance Criteria:**
- Passes security and compliance review
- Handles production scale (100+ CNPG clusters)
- Complete documentation for operations team
- Ready for production deployment

---

## Risk Assessment & Mitigation

### High-Risk Areas

#### Risk 1: Storage Expansion Failures
**Impact**: High - Could cause service disruption if expansion fails during critical situation
**Probability**: Medium - Storage classes and CSI drivers can have bugs
**Mitigation**:
- Implement comprehensive pre-flight checks before expansion
- Provide manual override capabilities for emergency situations
- Test expansion operations extensively in staging environments
- Implement gradual rollout with circuit breaker patterns

#### Risk 2: Controller Performance Impact
**Impact**: Medium - Controller consuming excessive resources could affect cluster performance
**Probability**: Low - Well-designed controllers typically have minimal overhead
**Mitigation**:
- Implement efficient caching and rate limiting
- Conduct thorough load testing with 100+ clusters
- Monitor controller resource usage in production
- Provide resource limits and QoS configurations

#### Risk 3: Integration Compatibility
**Impact**: High - Breaking changes in CNPG or storage systems could break automation
**Probability**: Medium - Dependencies can introduce breaking changes
**Mitigation**:
- Pin specific versions of dependencies with testing
- Implement extensive integration test suite
- Monitor upstream projects for breaking changes
- Provide compatibility matrix and upgrade procedures

### Medium-Risk Areas

#### Risk 4: Configuration Complexity
**Impact**: Medium - Complex configuration could lead to misuse or misconfiguration
**Probability**: Medium - Users may not understand all configuration options
**Mitigation**:
- Provide sensible defaults for common use cases
- Implement configuration validation and warnings
- Create comprehensive documentation with examples
- Provide configuration templates for different scenarios

#### Risk 5: Alert Fatigue
**Impact**: Medium - Too many alerts could reduce response effectiveness
**Probability**: Medium - Alerting systems often generate noise over time
**Mitigation**:
- Implement intelligent alert suppression and deduplication
- Provide alert tuning capabilities and guidelines
- Monitor alert response rates and effectiveness
- Regular alert review and optimization processes

### Additional Risks (Identified in Architecture Review)

#### Risk 6: Storage Class Incompatibility
**Impact**: High - Expansion attempts on incompatible storage classes will fail
**Probability**: Medium - Many storage classes don't support online expansion
**Mitigation**:
- Pre-flight check: verify `allowVolumeExpansion: true` on storage class
- Alert with clear message when storage class doesn't support expansion
- Document supported storage classes in compatibility matrix
- Graceful degradation: monitoring-only mode for incompatible storage

#### Risk 7: Filesystem Resize Failure
**Impact**: High - PVC expanded but filesystem not resized leaves cluster in inconsistent state
**Probability**: Medium - Some CSI drivers require pod restart for filesystem resize
**Mitigation**:
- Monitor PVC capacity vs filesystem capacity after expansion
- Alert with specific instructions when filesystem resize doesn't complete
- Document storage classes that require pod restart
- Do NOT automatically restart pods (conservative approach per design decision)

#### Risk 8: WAL Cleanup Data Loss
**Impact**: Critical - Incorrect WAL cleanup could cause data loss or break replication
**Probability**: Low - pg_archivecleanup is designed to be safe
**Mitigation**:
- Only clean WAL files confirmed archived (check archive_status)
- Respect minimum retention count from policy
- Verify replication slots before cleanup
- Comprehensive logging of all WAL cleanup operations
- Circuit breaker on repeated cleanup failures

#### Risk 9: Cluster Deletion During Operations
**Impact**: Medium - In-progress expansion on deleted cluster causes orphaned resources
**Probability**: Low - Race condition requires specific timing
**Mitigation**:
- Use owner references on StorageEvent CRs (auto-deleted with cluster)
- Check cluster existence before each operation step
- Implement finalizers for graceful cleanup
- Timeout stale operations after configurable interval

#### Risk 10: Maximum PVC Size Reached
**Impact**: High - No further expansion possible, database will crash if usage continues
**Probability**: Medium - Eventually all clusters approach their limits
**Mitigation**:
- Alert at 80% of maxSize with "approaching limit" warning
- Alert at 100% of maxSize with critical severity and manual action required
- Include capacity planning recommendations in alert payload
- Document procedure for increasing maxSize limit

#### Risk 11: Controller State Loss
**Impact**: Medium - Lost metrics history affects trend analysis and may cause duplicate actions
**Probability**: Low - PVC-backed state survives normal restarts
**Mitigation**:
- StatefulSet deployment ensures PVC persistence
- Graceful shutdown writes in-memory state to disk
- Startup recovery rebuilds state from StorageEvent CRs
- Idempotent operations prevent duplicate expansions

#### Risk 12: CNPG Operator Conflict
**Impact**: Medium - Both controllers modifying same resources could cause thrashing
**Probability**: Low - Annotation-based coordination minimizes direct conflicts
**Mitigation**:
- Use annotations rather than direct resource modification
- Respect CNPG operator's reconciliation ownership of PVCs
- Implement backoff when detecting CNPG operator activity
- Document clear separation of responsibilities

---

## Resource Requirements

### Development Team
- **Lead Developer**: 1 FTE - Senior Go developer with Kubernetes experience
- **Backend Developer**: 1 FTE - Go developer familiar with controller-runtime
- **DevOps Engineer**: 0.5 FTE - Kubernetes and CNPG expertise for testing
- **Product Manager**: 0.25 FTE - Requirements refinement and stakeholder management

### Infrastructure Requirements
- **Development Clusters**: 2 Kubernetes clusters for development and testing
- **Staging Environment**: 1 production-like cluster with CNPG and CSI storage supporting expansion
- **CI/CD Pipeline**: GitHub Actions with sufficient runner capacity
- **Container Registry**: Harbor or similar for image storage

### Timeline & Budget
- **Development Phase**: 18-22 weeks
- **Total Engineering Cost**: ~$150K-200K (based on team composition)
- **Infrastructure Cost**: ~$2K-3K/month during development
- **Ongoing Maintenance**: 0.5 FTE after initial release

---

## Dependencies & Prerequisites

### Internal Dependencies
- **CNPG Operator**: Version 1.20+ with stable API
- **CSI Storage Driver**: Any storage class with `allowVolumeExpansion: true`
- **Prometheus Stack**: kube-prometheus-stack for metrics and alerting (optional but recommended)
- **ArgoCD/GitOps**: For policy deployment and management (optional)

### External Dependencies
- **controller-runtime**: v0.16+ for controller framework
- **kubebuilder**: v3.12+ for CRD generation and scaffolding
- **Kubernetes**: v1.25+ with stable CSI volume expansion

### Team Prerequisites
- Go development expertise (required)
- Kubernetes operator development experience (required)
- CNPG/PostgreSQL operational knowledge (preferred)
- Storage systems and CSI expertise (preferred)

---

## Quality Gates & Acceptance Criteria

### Code Quality Standards
- **Test Coverage**: >85% for all core components
- **Static Analysis**: Clean results from golangci-lint, gosec
- **Performance**: <100MB memory usage, <50ms response time for API calls
- **Security**: No high/critical vulnerabilities in dependencies

### Functional Acceptance
- **Monitoring**: Successfully monitors 100+ CNPG clusters
- **Expansion**: 95% success rate for automatic PVC expansion
- **Alerting**: <5% false positive rate for storage alerts
- **Recovery**: <5 minute MTTR for storage-related issues

### Operational Acceptance
- **Deployment**: Single-command Helm deployment with GitOps
- **Documentation**: Complete operator guide and troubleshooting runbook
- **Observability**: Comprehensive metrics and dashboards
- **Reliability**: 99.9% controller uptime in production

---

## Go-to-Market Strategy

### Deployment Plan
1. **Alpha Release**: Internal testing with development clusters
2. **Beta Release**: Staging deployment with non-critical databases
3. **Production Rollout**: Gradual rollout starting with low-risk clusters
4. **Full Deployment**: All production CNPG clusters under management

### Training & Documentation
- **Operator Training**: 2-hour training session for DevOps team
- **Documentation**: Comprehensive operator guide and API reference
- **Runbooks**: Emergency procedures and troubleshooting guides
- **Best Practices**: Configuration guidelines and policy examples

### Success Metrics Tracking
- **Weekly Monitoring**: Track KPIs and user feedback during rollout
- **Monthly Reviews**: Assess automation success rate and incident reduction
- **Quarterly Assessment**: Evaluate ROI and plan enhancement features

---

## Next Steps & Approval Process

### Immediate Actions Required
1. **Technical Review**: Engineering architecture approval (Week 1)
2. **Resource Allocation**: Confirm team assignments and timeline (Week 1)
3. **Environment Setup**: Provision development and staging infrastructure (Week 2)
4. **Stakeholder Alignment**: Final requirements review with operations team (Week 2)

### Approval Gates
- [ ] **Technical Approval**: Lead Architect and Engineering Manager
- [ ] **Resource Approval**: Engineering Director for team allocation
- [ ] **Business Approval**: Product Director for project prioritization
- [ ] **Security Approval**: Security team for approach and dependencies

### Communication Plan
- **Kick-off Meeting**: Week 1 with full project team
- **Weekly Standups**: Progress updates and blocker resolution
- **Bi-weekly Demos**: Stakeholder demos for feedback and alignment
- **Monthly Reviews**: Business impact assessment and metrics review

---

**Document Owner**: Senior Product Manager
**Next Review Date**: 2025-12-10
**Stakeholder Distribution**: Engineering, DevOps, Security, Product Leadership

---

*This document serves as the single source of truth for the CNPG Storage Manager project. All decisions and changes should be tracked through version control with appropriate stakeholder review.*