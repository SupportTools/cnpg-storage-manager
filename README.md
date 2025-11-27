# CNPG Storage Manager

A Kubernetes controller for automated storage management of CloudNativePG (CNPG) clusters. It monitors storage usage, automatically expands PVCs when thresholds are breached, performs WAL cleanup in emergencies, and sends alerts through multiple channels.

## Features

- **Storage Monitoring**: Continuously monitors PVC usage across CNPG clusters
- **Automated PVC Expansion**: Automatically expands PVCs when configurable thresholds are breached
- **WAL Cleanup**: Performs PostgreSQL WAL file cleanup in emergency situations
- **Multi-Channel Alerting**: Sends alerts via Prometheus Alertmanager, Slack, and PagerDuty
- **Circuit Breaker Protection**: Prevents action loops with configurable failure thresholds
- **Dry-Run Mode**: Test policies without taking actual actions
- **Per-Cluster Cooldowns**: Configurable cooldown periods between operations
- **Prometheus Metrics**: Comprehensive metrics for monitoring and observability

## Architecture

```
┌─────────────────────────────────────────────────────────────────┐
│                    CNPG Storage Manager                         │
├─────────────────────────────────────────────────────────────────┤
│  StoragePolicy Controller                                       │
│  ┌─────────────┐  ┌─────────────┐  ┌─────────────────────────┐ │
│  │  Discovery  │→ │  Metrics    │→ │  Policy Evaluator       │ │
│  │  (CNPG)     │  │  Collector  │  │  (Threshold Checks)     │ │
│  └─────────────┘  └─────────────┘  └───────────┬─────────────┘ │
│                                                 │               │
│  ┌─────────────────────────────────────────────▼─────────────┐ │
│  │                    Action Handlers                        │ │
│  │  ┌──────────────┐ ┌──────────────┐ ┌──────────────────┐  │ │
│  │  │  Expansion   │ │  WAL Cleanup │ │  Alert Manager   │  │ │
│  │  │  Engine      │ │  Engine      │ │  (Multi-Channel) │  │ │
│  │  └──────────────┘ └──────────────┘ └──────────────────┘  │ │
│  └───────────────────────────────────────────────────────────┘ │
└─────────────────────────────────────────────────────────────────┘
```

## Getting Started

### Prerequisites

- Kubernetes v1.26+
- CloudNativePG operator installed
- go version v1.24+ (for development)
- docker version 17.03+
- kubectl version v1.26+

### Installation

**Install the CRDs:**

```sh
make install
```

**Deploy the controller:**

```sh
make deploy IMG=ghcr.io/supporttools/cnpg-storage-manager:latest
```

### Quick Start

1. Create a StoragePolicy to monitor your CNPG clusters:

```yaml
apiVersion: cnpg.supporttools.io/v1alpha1
kind: StoragePolicy
metadata:
  name: production-postgres
  namespace: database
spec:
  selector:
    matchLabels:
      environment: production

  thresholds:
    warning: 70      # Alert at 70%
    critical: 80     # Critical alert at 80%
    expansion: 85    # Auto-expand at 85%
    emergency: 90    # WAL cleanup at 90%

  expansion:
    enabled: true
    percentage: 50        # Expand by 50%
    minIncrementGi: 5     # Minimum 5Gi expansion
    maxSize: 100Gi        # Maximum size limit
    cooldownMinutes: 30

  walCleanup:
    enabled: true
    retainCount: 10
    requireArchived: true
    cooldownMinutes: 15

  alerting:
    channels:
      - type: alertmanager
        endpoint: "http://alertmanager:9093"
      - type: slack
        webhookSecret: "default/slack-webhook"
        channel: "#db-alerts"
```

2. Apply the policy:

```sh
kubectl apply -f storagepolicy.yaml
```

3. Monitor with:

```sh
kubectl get storagepolicies
kubectl get storageevents
```

## Testing with Dry-Run Mode

Before enabling actual remediation actions, you can deploy with global dry-run mode to test and validate the controller's behavior:

### Using Helm with Dry-Run

```sh
# Deploy with dry-run mode enabled
helm install cnpg-storage-manager ./charts/cnpg-storage-manager \
  --namespace cnpg-storage-manager \
  --create-namespace \
  --set dryRun=true \
  --set logging.level=debug \
  --set logging.development=true

# Or use the pre-configured dry-run values file
helm install cnpg-storage-manager ./charts/cnpg-storage-manager \
  --namespace cnpg-storage-manager \
  --create-namespace \
  -f ./charts/cnpg-storage-manager/values-dryrun.yaml
```

### Using kubectl/kustomize

Set the `--dry-run` flag or `DRY_RUN=true` environment variable:

```yaml
# In your manager deployment
containers:
  - name: manager
    args:
      - --dry-run
    # Or via environment variable
    env:
      - name: DRY_RUN
        value: "true"
```

### What Dry-Run Mode Does

When dry-run is enabled (globally or per-policy), the controller will:

- **Continue to do:**
  - Discover and monitor CNPG clusters
  - Collect storage metrics from kubelet
  - Evaluate thresholds against configured policies
  - Log what actions **would** be taken
  - Send alerts (alerts are still sent in dry-run)
  - Update Prometheus metrics

- **NOT do:**
  - Expand PVCs (no actual size changes)
  - Delete WAL files (no actual file deletions)
  - Create StorageEvent audit records for remediation

### Monitoring Dry-Run Behavior

Watch the controller logs to see what actions would be taken:

```sh
kubectl logs -f deployment/cnpg-storage-manager -n cnpg-storage-manager
```

Example dry-run log output:
```
INFO  DryRun: Would expand PVCs  {"cluster": "my-postgres", "globalDryRun": true, "policyDryRun": false}
INFO  DryRun: Would cleanup WAL  {"cluster": "my-postgres", "globalDryRun": true, "policyDryRun": false}
```

### Transitioning to Live Mode

Once you're confident the controller is behaving as expected:

```sh
# Upgrade with dry-run disabled
helm upgrade cnpg-storage-manager ./charts/cnpg-storage-manager \
  --namespace cnpg-storage-manager \
  --set dryRun=false
```

## Configuration

### StoragePolicy Spec

| Field | Description | Default |
|-------|-------------|---------|
| `selector` | Label selector for matching CNPG clusters | Required |
| `thresholds.warning` | Warning alert threshold (%) | 70 |
| `thresholds.critical` | Critical alert threshold (%) | 80 |
| `thresholds.expansion` | Auto-expansion threshold (%) | 85 |
| `thresholds.emergency` | WAL cleanup threshold (%) | 90 |
| `expansion.enabled` | Enable automatic PVC expansion | true |
| `expansion.percentage` | Percentage to expand by | 50 |
| `expansion.minIncrementGi` | Minimum expansion size (Gi) | 5 |
| `expansion.maxSize` | Maximum PVC size limit | - |
| `expansion.cooldownMinutes` | Time between expansions | 30 |
| `walCleanup.enabled` | Enable WAL cleanup | true |
| `walCleanup.retainCount` | Minimum WAL files to keep | 10 |
| `walCleanup.requireArchived` | Only clean archived WALs | true |
| `circuitBreaker.maxFailures` | Failures before circuit opens | 3 |
| `circuitBreaker.resetMinutes` | Time before circuit resets | 60 |
| `dryRun` | Enable dry-run mode | false |

### Alert Channels

**Alertmanager:**
```yaml
- type: alertmanager
  endpoint: "http://alertmanager:9093"
```

**Slack:**
```yaml
- type: slack
  webhookSecret: "namespace/secret-name"  # Secret with 'webhook-url' key
  channel: "#alerts"
```

**PagerDuty:**
```yaml
- type: pagerduty
  routingKeySecret: "namespace/secret-name"  # Secret with 'routing-key' key
```

## Metrics

The controller exposes Prometheus metrics on `:8080/metrics`:

| Metric | Description |
|--------|-------------|
| `cnpg_storage_manager_pvc_usage_bytes` | Current PVC usage in bytes |
| `cnpg_storage_manager_pvc_capacity_bytes` | Total PVC capacity in bytes |
| `cnpg_storage_manager_pvc_usage_percent` | PVC usage percentage |
| `cnpg_storage_manager_wal_directory_bytes` | WAL directory size |
| `cnpg_storage_manager_wal_files_count` | Number of WAL files |
| `cnpg_storage_manager_expansion_total` | Total expansion operations |
| `cnpg_storage_manager_wal_cleanup_total` | Total WAL cleanup operations |
| `cnpg_storage_manager_alerts_sent_total` | Total alerts sent |
| `cnpg_storage_manager_circuit_breaker_open` | Circuit breaker state |
| `cnpg_storage_manager_threshold_breaches_total` | Threshold breach count |

## Storage Events

The controller creates StorageEvent resources to track all operations:

```sh
# List all events
kubectl get storageevents -A

# View expansion events
kubectl get storageevents -l cnpg.supporttools.io/event-type=expansion

# View WAL cleanup events
kubectl get storageevents -l cnpg.supporttools.io/event-type=wal-cleanup
```

## Annotations

Override policy settings per-cluster using annotations:

```yaml
apiVersion: postgresql.cnpg.io/v1
kind: Cluster
metadata:
  name: my-cluster
  annotations:
    # Override thresholds
    cnpg.supporttools.io/warning-threshold: "75"
    cnpg.supporttools.io/expansion-threshold: "90"

    # Disable specific features
    cnpg.supporttools.io/expansion-enabled: "false"
    cnpg.supporttools.io/wal-cleanup-enabled: "false"

    # Set expansion parameters
    cnpg.supporttools.io/expansion-percentage: "100"
    cnpg.supporttools.io/max-size: "200Gi"
```

## Development

### Building

```sh
# Build binary
make build

# Run tests
make test

# Run tests with coverage
make test-coverage

# Run locally
make run-local
```

### Testing

```sh
# Unit tests
make test

# Quick tests (no manifests regeneration)
make test-quick

# E2E tests (requires Kind)
make test-e2e
```

## Uninstall

```sh
# Remove sample resources
make undeploy-samples

# Remove controller
make undeploy

# Remove CRDs
make uninstall
```

## Contributing

1. Fork the repository
2. Create a feature branch
3. Make your changes with tests
4. Run `make test` and `make lint`
5. Submit a pull request

## License

Copyright 2025 SupportTools.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
