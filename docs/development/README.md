# Development Documentation

> **Development standards, guidelines, refactoring plans, and code quality documentation**

---

## 📑 Quick Navigation

- [Development Standards](#development-standards)
- [Code Quality & Refactoring](#code-quality--refactoring)
- [Release Management](#release-management)
- [Related Documentation](#related-documentation)

---

## Development Standards

### Core Guidelines
- **[CLAUDE.md](../../CLAUDE.md)** ✅ - **PRIMARY DEVELOPMENT GUIDE**
  - Go version requirements (1.24+)
  - API handler architecture standards
  - Database migration rules
  - Web UI-API synchronization
  - Repository cleanliness rules
  - SDK usage requirements
  - Task development workflow

### Error Handling
- **[Error Handling Guide](error-handling-guide.md)** ✅
  - Standard error patterns
  - Error response formats
  - Logging best practices
  - Error propagation
  - User-facing error messages

---

## Code Quality & Refactoring

### Static Analysis Fixes
- **[G104 Fix Plan](../archive/plans/g104-remediation-plan.md)** 📦
  - Unhandled error remediation (completed)
  - gosec G104 compliance
  - Error checking patterns
  - Implementation strategy

### Package Refactoring
- **[Package Naming Refactor Plan](../archive/plans/package-naming-refactor-plan-2024-10.md)** 📦
  - Package structure improvements (completed October 2024)
  - Naming convention updates
  - Import path standardization
  - Migration timeline

- **[Package Refactor Checklist](../archive/plans/package-naming-refactor-checklist.md)** 📦
  - Step-by-step refactoring tasks (completed)
  - Validation procedures
  - Testing requirements
  - Completion criteria

---

## Release Management

### Release Status
- **[Agent v1.6.3 Release Status](../archive/releases/agent-v1.6.3-release-status.md)** 📦
  - Production readiness assessment (completed)
  - Feature completion status
  - Known issues and limitations
  - Post-release monitoring

---

## Development Workflow

### Critical Rules (from CLAUDE.md)

#### Go Version Requirements
```bash
# ALWAYS use Go 1.24
go version  # Must show go1.24 or later
./scripts/validate-go-version.sh
```

#### Repository Cleanliness
```bash
# NEVER create files in repository root
# Files belong in proper directories:
# - Go code → api/, agent/, cmd/, pkg/
# - Scripts → scripts/
# - Tests → tests/
# - Docs → docs/

# Validate repository cleanliness
./scripts/validate-repo-cleanliness.sh
```

#### API Handler Architecture
**Single-function-per-file pattern:**
```
pkg/handlers/domain/subdomain/
├── helpers.go          # Logger, shared types, utilities
├── handler_one.go      # Individual handler function
└── handler_two.go      # Individual handler function
```

#### Database Migration Rules
**CRITICAL: Add models to BOTH locations:**
```go
// 1. api/pkg/utils/ConnectDB.go - modelsList array
// 2. migrations/pkg/migrations/migrator.go - getModels() function
```

#### SDK Usage Requirements
**MANDATORY: Use Go SDK for all API access**
```go
import "github.com/{{PROJECT_NAME}}/go-sdk"

// Create client
client, err := {{PROJECT_NAME}}.NewClient(&{{PROJECT_NAME}}.Config{
    BaseURL: "https://api.{{PROJECT_NAME}}.com",
    Auth: {{PROJECT_NAME}}.AuthConfig{
        APIKey:    os.Getenv("NEXMONYX_API_KEY"),
        APISecret: os.Getenv("NEXMONYX_API_SECRET"),
    },
})
```

---

## Task Development Workflow

### Mandatory Process (from CLAUDE.md)

#### Step 1: Task Selection and Research
- Select ONE task from current priority level using `mcp__taskforge__getTasks({projectId: 1, featureId: X})`
- Mark as in-progress: `mcp__taskforge__updateTask({taskId: X, status: "in_progress"})`
- Use TodoWrite for session tracking (detailed workflow steps)
- Research thoroughly before coding

#### Step 2: Implementation
- Follow master design document specifications
- Use established patterns from existing codebase
- Implement comprehensive logging (Trace/Debug)

#### Step 3: Build Verification (MANDATORY)
```bash
make build-api-local
# Must pass before proceeding
```

#### Step 4: QA Agent Verification (MANDATORY)
```bash
# Use Task tool with qa-engineer agent
# QA must verify no new problems introduced
```

#### Step 5: Deployment (MANDATORY)
```bash
make bump
# or
make bump-with-monitoring  # For 10-15 min builds
```

#### Step 6: Devils Advocate Verification (MANDATORY)
```bash
# Use devils advocate agent to verify completion
# Must confirm task fully completed
```

#### Step 7: Quality Assurance Cycle
```bash
# IF issues found: Fix → Build → QA → Deploy → Retest
# Stay in cycle until task successfully completed
```

---

## Code Quality Standards

### Testing Requirements
- **Unit tests:** 80% coverage target
- **Integration tests:** Critical paths
- **E2E tests:** Major user flows

### Static Analysis
```bash
# Run validation suite
./scripts/validate-all.sh

# Quick validation (no Docker)
./scripts/validate-all.sh --quick

# Component validation
./scripts/validate-api.sh
./scripts/validate-agent.sh
```

### Security Scanning
```bash
# gosec security scanner
gosec ./...
```

---

## Common Development Commands

### Building
```bash
# Build all components
make build-all-local

# Build specific components
make build-api-local
make build-agent-local
make build-ui-local
```

### Testing
```bash
# Run all tests
make test-local

# Component-specific tests
make test-api-local
make test-agent-local
```

### Swagger Documentation
```bash
# Generate API documentation
make generate-swagger
```

### Local Development
```bash
# Run entire stack
make run-local

# Docker Compose services
make up
make down
make restart
make logs
```

---

## Related Documentation

### Architecture & Design
- [Master Design Document](../../master-design-doc.md)
- [Handler Refactoring Standard](../../CLAUDE.md#api-handler-architecture-standards)
- [Design Documents](../design/)

### API Development
- [API Documentation](../api/)
- [API Handler Architecture](../../CLAUDE.md#api-handler-architecture-standards)
- [Swagger Documentation](../../api/docs/)

### Database
- [Database Migration Rules](../../CLAUDE.md#database-migration-rules)
- [Migration Guides](../migration/)
- [Schema Documentation](../../CLAUDE.md#database-schema)

### Testing
- [Validation System](../design/validation-system.md)
- [Test Results](../archive/analysis/test-results-summary.md)

### Deployment
- [Deployment Guides](../operations/deployment/)
- [CI/CD Pipeline](../../.github/workflows/)

---

## Development Best Practices

### Code Organization
- Single responsibility per file/function
- Clear naming conventions (snake_case for JSON, camelCase for Go)
- Comprehensive logging at all levels
- Complete Swagger documentation for all endpoints

### Error Handling
- Always check and handle errors
- Use consistent error response format
- Log errors with appropriate context
- Return user-friendly error messages

### Database Operations
- Use GORM for all database access
- Add proper indexes
- Use transactions for multi-step operations
- Implement proper connection pooling

### API Development
- Follow RESTful conventions
- Use standard HTTP status codes
- Implement proper pagination
- Version breaking changes

### Security
- Never log sensitive data
- Use parameterized queries
- Validate all input
- Implement rate limiting
- Use proper authentication/authorization

---

## Support & Resources

### Getting Help
- **Main Documentation:** [docs/README.md](../README.md)
- **Quick Reference:** [docs/quick-reference.md](../quick-reference.md)
- **Topic Index:** [docs/topics.md](../topics.md)

### Development Tools
- **Go SDK:** [github.com/{{PROJECT_NAME}}/go-sdk](https://github.com/{{PROJECT_NAME}}/go-sdk)
- **CLI Tool:** [cmd/cli/](../../cmd/cli/)
- **Validation Scripts:** [scripts/](../../scripts/)

---

**Last Updated:** 2024-10-26
**Status:** ✅ Current and Validated
**Next Review:** 2025-11-02
