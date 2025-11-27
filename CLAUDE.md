# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

CNPG Storage Manager is a Kubernetes controller that prevents database outages caused by storage exhaustion in CloudNativePG (CNPG) PostgreSQL clusters. It provides automated monitoring, alerting, and remediation for storage-related issues including WAL file accumulation and PVC expansion.

## Technology Stack

- **Language**: Go 1.21+
- **Framework**: controller-runtime v0.16+
- **Scaffolding**: kubebuilder v3.12+
- **Testing**: Ginkgo v2 + Gomega + envtest
- **Deployment**: Helm chart + Kustomize

## Build Commands

```bash
# Generate manifests (CRDs, RBAC, webhooks)
make manifests

# Generate code (DeepCopy, etc.)
make generate

# Build controller binary
make build

# Run tests
make test

# Run unit tests only
make test-unit

# Run integration tests (envtest)
make test-integration

# Run E2E tests (requires kind cluster)
make test-e2e

# Build Docker image
make docker-build IMG=ghcr.io/supporttools/cnpg-storage-manager:dev

# Push Docker image
make docker-push IMG=ghcr.io/supporttools/cnpg-storage-manager:dev

# Install CRDs to cluster
make install

# Deploy controller to cluster
make deploy IMG=ghcr.io/supporttools/cnpg-storage-manager:dev

# Run controller locally (outside cluster)
make run

# Run linter
make lint

# Full validation (mirrors CI/CD)
make validate-pipeline-local
```

## Architecture

### Key Design Decisions

| Decision | Choice |
|----------|--------|
| API Group | `cnpg.supporttools.io` |
| CNPG Coordination | Annotation-based (non-invasive) |
| WAL Cleanup | pg_archivecleanup via pod exec |
| Storage Backend | Any CSI with `allowVolumeExpansion=true` |
| Multi-Instance | Expand all PVCs when any hits threshold |
| Policy Conflicts | Validation webhook rejects overlapping |
| State Persistence | Controller uses PVC for metrics history |
| Resize Failure | Alert and wait (no auto pod restart) |
| Kubernetes Version | 1.25+ |

### Custom Resource Definitions

1. **StoragePolicy** (`cnpg.supporttools.io/v1alpha1`): Defines per-cluster or per-namespace storage management policies including thresholds, expansion settings, WAL cleanup config, circuit breaker, and alerting channels.

2. **StorageEvent** (`cnpg.supporttools.io/v1alpha1`): Records storage-related events (expansion, WAL cleanup) with detailed status tracking per-PVC.

### Core Components

- **CNPGStorageController**: Main reconciliation loop watching CNPG clusters and PVCs
- **Metrics Collector**: Integrates with kubelet volume stats API, persists to PVC
- **Policy Engine**: Evaluates policies with validation webhook for conflict detection
- **Remediation Engine**: Executes PVC expansion (all PVCs in cluster)
- **WAL Cleanup Engine**: Executes pg_archivecleanup via client-go pod exec
- **Alert Manager**: Prometheus Alertmanager, Slack, PagerDuty integration
- **Circuit Breaker**: Per-cluster failure tracking with auto-reset

### Annotation Schema

Controller uses annotations on CNPG Cluster CR for coordination:
```
storage.cnpg.supporttools.io/managed: "true"
storage.cnpg.supporttools.io/paused: "false"
storage.cnpg.supporttools.io/target-size: "15Gi"
storage.cnpg.supporttools.io/current-usage-percent: "87"
```

### Project Structure

```
.
├── api/v1alpha1/           # CRD type definitions
│   ├── storagepolicy_types.go
│   ├── storageevent_types.go
│   └── groupversion_info.go
├── cmd/main.go             # Controller entrypoint
├── config/                 # Kubernetes manifests
│   ├── crd/bases/         # Generated CRD YAML
│   ├── rbac/              # RBAC configuration
│   ├── manager/           # Controller deployment
│   └── default/           # Kustomize overlays
├── controllers/            # Controller implementations
│   ├── storagepolicy_controller.go
│   └── suite_test.go
├── pkg/                    # Internal packages
│   ├── metrics/           # Prometheus metrics collector
│   ├── policy/            # Policy evaluation engine
│   ├── remediation/       # Expansion and cleanup logic
│   ├── alerting/          # Alert integrations
│   └── exec/              # Pod exec for pg_archivecleanup
├── charts/                 # Helm chart
│   └── cnpg-storage-manager/
├── test/e2e/              # E2E tests
└── hack/                  # Build scripts
```

## Development Workflow

### Creating a New CRD

```bash
# Create new API
kubebuilder create api --group cnpg --version v1alpha1 --kind NewResource

# Implement types in api/v1alpha1/newresource_types.go
# Implement controller in controllers/newresource_controller.go

# Regenerate
make manifests generate
```

### Testing Strategy

1. **Unit Tests** (`./pkg/...`): Standard Go tests for business logic
2. **Integration Tests** (`./controllers/...`): Use envtest for controller tests
3. **E2E Tests** (`./test/e2e/...`): Use kind cluster with real CNPG operator

### Running Tests

```bash
# All tests
make test

# Unit tests with coverage
go test -v -race -coverprofile=coverage.out ./pkg/...

# Integration tests
make test-integration

# Specific test
go test -v ./controllers/... -run TestStoragePolicyController

# E2E (setup kind first)
kind create cluster --config=test/e2e/kind-config.yaml
make test-e2e
```

## Quality Standards

- Test coverage: >85% for core components
- Memory usage: <100MB
- API response time: <50ms
- Clean golangci-lint and gosec results

### Pre-Push Validation

Always run before pushing:
```bash
make validate-pipeline-local
```

This runs: fmt, vet, lint, unit tests, integration tests.

## Agent Squad

This project uses a multi-agent development approach. See `.claude/AGENT_GUIDE.md` for details.

### Key Agents for This Project

- **go-kubernetes-skeptic**: Review Go/K8s code quality
- **kubernetes-specialist**: K8s troubleshooting and best practices
- **database-skeptic**: PostgreSQL and data integrity concerns
- **qa-engineer**: Test strategy and coverage
- **devops-engineer**: CI/CD and deployment

### Quality Gates

All changes must pass:
1. `make validate-pipeline-local` (technical validation)
2. QA Engineer review (test coverage)
3. Devils Advocate review (edge cases)
4. Skeptic review (domain-specific concerns)

## Key Integration Points

- **CloudNativePG Operator** (v1.20+): Cluster status via CR, coordination via annotations
- **Any CSI Driver**: Storage expansion via `allowVolumeExpansion: true`
- **Prometheus**: Metrics exposition (`/metrics`)
- **Alertmanager**: Alert routing

## Common Tasks

### Add a New Metric

1. Define in `pkg/metrics/metrics.go`
2. Register in controller setup
3. Update in reconcile loop
4. Document in design.md

### Add Alert Channel

1. Implement interface in `pkg/alerting/`
2. Add configuration to StoragePolicy spec
3. Update Helm values.yaml
4. Add tests

### Modify CRD

1. Update types in `api/v1alpha1/`
2. Run `make manifests generate`
3. Update validation webhook if needed
4. Update Helm CRD templates
5. Add migration notes

## Task Tracking

This project uses TaskForge for task management (Project ID: 82).

### Features (Phases)
1. **Project Foundation & Infrastructure** - kubebuilder scaffolding, CRD types, build system
2. **Core Monitoring Infrastructure** - CNPG discovery, kubelet metrics, threshold evaluation
3. **Automated Storage Expansion** - PVC expansion engine, storage class validation
4. **Configuration & Policy Management** - Policy selectors, validation webhook, conflict detection
5. **Alerting & Observability** - Alertmanager, Slack, PagerDuty integration
6. **WAL Cleanup & Emergency Actions** - pg_archivecleanup, circuit breaker
7. **Testing & Quality Assurance** - Unit, integration, E2E test suites
8. **Deployment & Operations** - Helm chart, security scanning, release automation

## Documentation

- `design.md`: Technical design document with full CRD specifications
- `docs/development/`: Development guides
- `docs/workflows/`: Process workflows
- `.claude/AGENT_GUIDE.md`: Agent usage guide

## Project Status

**Phase 1: Project Foundation & Infrastructure** - COMPLETE
- kubebuilder scaffolding initialized
- StoragePolicy and StorageEvent CRDs implemented
- Build infrastructure and CI/CD configured
- All tests passing

**Phase 2: Core Monitoring Infrastructure** - COMPLETE
- CNPG cluster discovery (`pkg/cnpg/discovery.go`)
- Kubelet metrics collector (`pkg/metrics/collector.go`)
- Prometheus metrics exposition (`pkg/metrics/prometheus.go`)
- Threshold evaluation engine (`pkg/policy/evaluation.go`) - 85.6% coverage
- Cluster annotation management (`pkg/annotations/annotations.go`) - 97.1% coverage
- StoragePolicy controller reconciler (`internal/controller/storagepolicy_controller.go`)

**Next Phases**:
- Phase 3: Automated Storage Expansion - In progress (PVC expansion logic implemented)
- Phase 4: Configuration & Policy Management
- Phase 5: Alerting & Observability
- Phase 6: WAL Cleanup & Emergency Actions
- Phase 7: Testing & Quality Assurance
- Phase 8: Deployment & Operations

### Key Files Implemented

| File | Description |
|------|-------------|
| `api/v1alpha1/storagepolicy_types.go` | StoragePolicy CRD with thresholds, expansion, WAL cleanup, circuit breaker |
| `api/v1alpha1/storageevent_types.go` | StorageEvent audit trail CRD |
| `internal/controller/storagepolicy_controller.go` | Main controller reconciliation logic |
| `pkg/cnpg/discovery.go` | CNPG cluster discovery via unstructured client |
| `pkg/metrics/collector.go` | Kubelet volume stats collection |
| `pkg/metrics/prometheus.go` | Prometheus metrics registration and helpers |
| `pkg/policy/evaluation.go` | Threshold evaluation and action recommendations |
| `pkg/annotations/annotations.go` | Cluster annotation helpers |

### Documentation

- `design.md`: Technical design document with full CRD specifications
- `docs/development/`: Development guides
- `docs/workflows/`: Process workflows
- `.claude/AGENT_GUIDE.md`: Agent usage guide
