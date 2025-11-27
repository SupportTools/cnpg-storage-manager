# Task Execution Workflow

**⭐ Core Framework Document** - This is a production-tested workflow extracted from the ${project} project.

This document defines the mandatory workflow for executing all development tasks in your project.

## 📝 Template Customization

**After initialization**:
- All `{{PROJECT_NAME}}` references will be replaced with your project name
- Architecture-specific examples (SDK, database patterns, etc.) should be adapted to your project
- Multi-repository sections apply only if your project uses that structure
- TaskForge references can be replaced with your task management system

**This workflow is technology-agnostic** at its core - the 8-step process applies regardless of your stack.

## Available Resources

### Task Management System

The project can use PostgreSQL-backed TaskForge for persistent task management:
- **Tasks organized by project** (P1-P5, Documentation, Infrastructure, Security, Testing)
- **Priority-based**: Urgent (🔥), High (🔴), Medium (🟡), Low (🟢)
- **Persistent across sessions**: All Claude Code instances share the same task state
- **MCP Tools**: `mcp__taskforge__listProjects`, `mcp__taskforge__getTasks`, `mcp__taskforge__updateTask`, `mcp__taskforge__completeTask`
- See CLAUDE.md "TaskForge Task Management" section for complete usage guide
- **Note**: TaskForge is optional. Projects can use GitHub Issues, Jira, or other task management systems

### Development Environment
- You have access to local development environment with Docker Compose for testing
- Kubernetes cluster access available via configured kubeconfig (see makefile KUBECONFIG_* variables)
- Use this environment to:
  - Test microservice interactions and controller behavior
  - Validate database migrations and schema changes
  - Run integration tests with live services
  - Verify Helm chart deployments and configurations
  - Debug service-specific issues
- Always verify environment context before running destructive operations
- Use appropriate namespaces to isolate test resources ({{PROJECT_NAME}}-dev, {{PROJECT_NAME}}-staging, {{PROJECT_NAME}}-prod)

### Multi-Repository Structure

**Note**: This section applies if your project uses multiple repositories. For monorepo projects, adapt accordingly.

The {{PROJECT_NAME}} platform may consist of multiple repositories that must be kept in sync:

| Repository | Example Path | Purpose | Update Trigger |
|------------|--------------|---------|----------------|
| **{{PROJECT_NAME}}** | `~/projects/{{PROJECT_NAME}}` | Main API server, controllers, core services | Primary development repository |
| **{{PROJECT_NAME}}-sdk** | `~/projects/{{PROJECT_NAME}}-sdk` | Official SDK for API communication | New API endpoints, model changes |
| **{{PROJECT_NAME}}-agent** | `~/projects/{{PROJECT_NAME}}-agent` | Monitoring or client agent | New metrics, API changes, agent features |
| **{{PROJECT_NAME}}-ui** | `~/projects/{{PROJECT_NAME}}-ui` | Frontend dashboard | API changes, new features, UI updates |
| **{{PROJECT_NAME}}-cli** | `~/projects/{{PROJECT_NAME}}-cli` | Command-line interface tool | New commands, API changes |

**CRITICAL RULE**: When changes in one repository require updates to other repositories, ALL affected repositories must be updated in the same task workflow before marking the task complete.

---

## Workflow Steps

### 1. Receive and Initiate Task

- I will provide you with a specific task from the TaskForge system or by name/description
- Retrieve tasks using `mcp__taskforge__getTasks` with appropriate project ID (P1-P5, Documentation, etc.)
- Verify the task priority level and ensure no higher priority tasks are pending
- Check for task dependencies (look for blockedByTaskId in task data)
- If task has unmet dependencies or is already completed, report this and await further instruction
- Update task status to "in_progress" using `mcp__taskforge__updateTask` when starting work

**TodoWrite Integration (MANDATORY for session tracking)**:
```
Use TodoWrite tool to create initial task breakdown:
- Research and design phase
- Implementation phase (broken down by repository if multi-repo)
- Build verification (for each affected repository)
- QA review
- Testing
- Deployment
- Devils advocate verification
- Task completion

Mark first item as "in_progress" immediately.

NOTE: TodoWrite tracks workflow steps WITHIN the current session.
TaskForge tracks project-level task completion ACROSS all sessions.
Both are mandatory - use TodoWrite for workflow progress, TaskForge for task completion.
```

### 2. Research & Design Phase (MANDATORY - NO CODING WITHOUT THIS)

**Thoroughly research the task requirements:**
- Review master-design-doc.md for architecture specifications
- Identify all integration points and service dependencies
- Review existing codebase for similar patterns
- Understand business logic and user requirements

**Present a comprehensive implementation plan that includes:**

#### Task Overview
Clear summary aligned with master design document

#### Design Approach
Technical strategy following your project's established patterns. Examples may include:
- Single-function-per-file handler pattern (if using this architecture)
- Domain-driven directory structure
- SDK usage for inter-service communication (if applicable)
- Database model placement following your migration strategy
- **Customize these patterns** based on your project's architecture standards

#### Files Affected
Complete list with brief explanation (adapt to your project structure):
- New handler/controller files (following your project's patterns)
- Helper files with logger setup (if applicable)
- Database models in correct locations (following your migration strategy)
- SDK updates if new API endpoints required (if using SDK pattern)
- Helm chart modifications if deployment changes needed (if using Kubernetes)
- Documentation updates

**Cross-Repository Impact Analysis** (if using multi-repo structure):
- **SDK Repository**: List SDK methods that need to be added/updated
- **Agent Repository**: Identify agent code changes for new features/API endpoints
- **UI Repository**: List UI components/API client updates needed
- **CLI Repository**: Identify CLI commands/flags that need updates
- **Note**: Adapt repository list to your actual project structure

#### Dependencies
Prerequisites and blockers:
- Service dependencies (list your actual services)
- Database schema requirements
- SDK/client capabilities (if applicable)
- External service integrations
- Infrastructure requirements

#### Success Criteria
Specific, measurable conditions:
- Project builds successfully (e.g., `make build-local`)
- All tests pass (e.g., `make test-local`)
- QA agent verification passes
- Deployment succeeds (`make bump`)
- Devils advocate verification passes

#### Risks & Mitigations
Potential issues and fallbacks

**TodoWrite Update**: Mark "Research and design phase" as completed, mark next phase as in_progress.

### 3. Wait for Approval (MANDATORY GATE)

**You must pause here and await explicit instruction.** Do not proceed to coding without approval.

Based on feedback:
- `✅ Approve`: Proceed immediately to Step 4 (Implementation)
- `🔄 Request changes`: Revise plan according to feedback and re-present
- `⏭️ Skip`: Mark task as skipped and await new assignment

**TodoWrite Update**: When approval received, mark "Implementation phase" (or first repository) as in_progress.

### 4. Implementation Phase

**IMPORTANT**: Work on repositories in the correct order to maintain dependency flow:

1. **{{PROJECT_NAME}}** (API/Core) - Make API changes first
2. **go-sdk** - Update SDK immediately after API changes
3. **linux-agent** - Update agent to use new SDK/API features
4. **web-ui** - Update frontend to use new API endpoints
5. **cli** - Update CLI to use new SDK/API features

Implement the approved design with strict adherence to:

#### Go Version
- **ALWAYS use Go 1.24** - verify go.mod specifies "go 1.24"

#### Logging Standards
```go
// In helpers.go
var domainLogger = logging.NewLogger("handlers", "domain")

// In each handler
if domainLogger.ShouldTrace("FunctionName") {
    domainLogger.Trace(c, "FunctionName", "Processing request")
}
// ... business logic with Debug logging
if domainLogger.ShouldTrace("FunctionName") {
    domainLogger.Trace(c, "FunctionName", "Operation completed")
}
```

#### Handler Architecture
- One function per file (e.g., `get_user.go` contains only `GetUser`)
- Domain-driven directory structure
- helpers.go with logger and shared types
- Complete Swagger documentation for all endpoints

#### Database Changes
- Add models to BOTH locations: ConnectDB.go AND migrator.go
- NEVER run migrations in API startup code
- Test with `make build-api-local`

#### SDK Usage
- Use {{PROJECT_NAME}} Go SDK for all inter-service communication
- Update SDK if creating new API endpoints
- NEVER make direct HTTP calls or database access from controllers

#### Repository Cleanliness
- NO files in repository root (except allowed config/docs)
- Proper file placement in appropriate directories
- Run `./scripts/validate-repo-cleanliness.sh` to verify

#### Multi-Repository Updates (WHEN REQUIRED)

**When to Update Each Repository:**

**TodoWrite Update**: If multi-repo task, mark primary repo implementation as completed, mark next repo as in_progress.

#### go-sdk Updates (MANDATORY for API changes)

**Decision Matrix: When to Update SDK**

| API Change Type | SDK Update Required | Version Bump | Dependencies to Update |
|-----------------|---------------------|--------------|------------------------|
| New API endpoint | ✅ Yes | Minor | linux-agent, cli |
| Endpoint signature change | ✅ Yes | Minor/Major* | linux-agent, cli, web-ui |
| New model fields (additive) | ✅ Yes | Minor | linux-agent, cli, web-ui |
| Model field removal | ✅ Yes | Major | linux-agent, cli, web-ui |
| New authentication method | ✅ Yes | Minor | linux-agent, cli |
| Error format changes | ✅ Yes | Major | linux-agent, cli, web-ui |
| Internal optimization | ❌ No | N/A | None |
| Documentation update | ❌ No | Patch (optional) | None |

*Use Major version bump if change breaks existing SDK consumers

**SDK Update Procedure:**

```bash
# STEP 1: Navigate to SDK repository
cd /home/mmattox/go/src/github.com/{{PROJECT_NAME}}/go-sdk

# STEP 2: Create or update service file
# Example: For new billing endpoints, update billing_usage.go
# Follow existing patterns:
#   - Create service methods with context.Context
#   - Use StandardResponse or custom response types
#   - Add proper error handling
#   - Document authentication requirements

# STEP 3: Update models if API models changed
# Add to models.go:
#   - Sync struct definitions with API models
#   - Use json tags matching API snake_case
#   - Add godoc comments

# STEP 4: Update client.go if new service added
# Add service field to Client struct
# Add service type declaration
# Initialize service in NewClient()

# STEP 5: Write comprehensive tests
# Create/update *_test.go file:
#   - Test happy paths with mock server
#   - Test error responses (401, 403, 404, 500)
#   - Test query parameters and request bodies
#   - Test response parsing
#   - Test pagination if applicable

# STEP 6: Run tests and verify coverage
go test -v ./...
go test -cover -coverprofile=coverage.out ./...
go tool cover -html=coverage.out
# TARGET: >80% coverage for new code

# STEP 7: Update documentation
# Edit README.md:
#   - Add service to services table
#   - Add usage examples showing authentication
#   - Document all new methods with code samples
#   - Include both self-service and admin examples (if applicable)

# STEP 8: Build verification
go build ./...
# MUST complete with zero errors

# STEP 9: Commit changes
git add .
git commit -m "feat: add support for [feature]

- Add [Service].Method1() for GET /v1/endpoint
- Add [Service].Method2() for POST /v1/endpoint
- Update [Model] with new fields
- Add comprehensive unit tests (coverage: X%)
- Related to {{PROJECT_NAME}} API task: [task description]

🤖 Generated with [Claude Code](https://claude.com/claude-code)
Co-Authored-By: Claude <noreply@anthropic.com>"

# STEP 10: Tag new version (semantic versioning)
# Determine version bump:
#   - MAJOR: Breaking changes (v2.x.x → v3.0.0)
#   - MINOR: New features, backward compatible (v2.3.x → v2.4.0)
#   - PATCH: Bug fixes, documentation (v2.3.4 → v2.3.5)

git tag v2.x.x
git push origin main --tags

# STEP 11: Update dependent repositories
# See "Dependent Repository Updates" section below
```

**SDK Quality Gates:**
- [ ] All new methods implemented with proper signatures
- [ ] Models synchronized with API (field names, types, json tags)
- [ ] Unit tests written with >80% coverage
- [ ] All tests passing: `go test -v ./...`
- [ ] Build successful: `go build ./...`
- [ ] README.md updated with examples
- [ ] Version tagged with semantic versioning
- [ ] No breaking changes unless major version bump
- [ ] Dependent repositories updated and tested

**Dependent Repository Updates:**

After SDK is tagged, update dependent repositories:

```bash
# linux-agent (if agent uses new endpoints)
cd /home/mmattox/go/src/github.com/{{PROJECT_NAME}}/linux-agent
go get github.com/{{PROJECT_NAME}}/go-sdk@v2.x.x
go mod tidy
# Update agent code to use new SDK methods
make build && make test
git commit -m "chore: upgrade SDK to v2.x.x for [feature]"

# cli (if CLI uses new endpoints)
cd /home/mmattox/go/src/github.com/{{PROJECT_NAME}}/cli
go get github.com/{{PROJECT_NAME}}/go-sdk@v2.x.x
go mod tidy
# Add/update CLI commands
make build && go test -v ./...
git commit -m "feat: add [command] using SDK v2.x.x"
```

**TodoWrite Integration:**
When SDK update is part of task workflow, track these phases:
- [ ] Implement API changes in {{PROJECT_NAME}}
- [ ] Update go-sdk with new methods/models
- [ ] Tag SDK version
- [ ] Update linux-agent (if needed)
- [ ] Update cli (if needed)
- [ ] Verify integration across all repos

Mark each phase complete in TodoWrite as you progress

**linux-agent Updates (for monitoring changes):**
```bash
cd /home/mmattox/go/src/github.com/{{PROJECT_NAME}}/linux-agent

# 1. Update agent to collect new metrics
# Modify collector code in pkg/collectors/

# 2. Update SDK import if SDK was updated
go get github.com/{{PROJECT_NAME}}/go-sdk@v2.x.x
go mod tidy

# 3. Add new configuration options if needed
# Update config structures and default configs

# 4. Run tests
make test

# 5. Update agent documentation
# Document new metrics/features in README.md

# 6. Commit changes
git add .
git commit -m "feat: add [metric/feature] collection"
git push origin main
```

**Triggers for linux-agent updates:**
- New metrics endpoints available in API
- New agent configuration required
- Agent authentication changes
- New SDK version with required updates
- New data collection requirements

**web-ui Updates (for frontend changes):**
```bash
cd /home/mmattox/go/src/github.com/{{PROJECT_NAME}}/web-ui

# 1. Sync API client with latest OpenAPI spec
npm run sync-api-full

# 2. Update UI components for new features
# Create/modify components in src/components/

# 3. Add new routes if needed
# Update routing in src/app/

# 4. Run tests
npm run test
npm run type-check

# 5. Verify build
npm run build

# 6. Commit changes
git add .
git commit -m "feat: add UI for [feature]"
git push origin main
```

**Triggers for web-ui updates:**
- API endpoints added/changed (auto-sync triggered)
- New UI features required
- Dashboard updates needed
- Breaking API changes (manual review required)

**cli Updates (for CLI changes):**
```bash
cd /home/mmattox/go/src/github.com/{{PROJECT_NAME}}/cli

# 1. Update SDK import if SDK was updated
go get github.com/{{PROJECT_NAME}}/go-sdk@v2.x.x
go mod tidy

# 2. Add new commands or flags
# Create/update command files in cmd/

# 3. Update command documentation
# Update help text and README.md

# 4. Run tests
go test -v ./...

# 5. Build and verify
make build

# 6. Commit changes
git add .
git commit -m "feat: add [command/feature]"
git push origin main
```

**Triggers for cli updates:**
- New SDK version with new capabilities
- New API endpoints available
- New CLI commands requested
- CLI authentication changes

**Multi-Repository Update Checklist:**

Before marking task complete, verify:
- [ ] Primary repository ({{PROJECT_NAME}}) changes committed
- [ ] go-sdk updated if API changes made
- [ ] linux-agent updated if metrics/collection changed
- [ ] web-ui synced if API changes made
- [ ] cli updated if new SDK capabilities added
- [ ] All repositories build successfully
- [ ] Integration between repositories tested
- [ ] Version tags updated where appropriate
- [ ] Documentation updated in all affected repos

**Repository Dependency Chain:**
```
{{PROJECT_NAME}} (API) → go-sdk → linux-agent
                      ↓
                   web-ui
                      ↓
                    cli
```

**CRITICAL**: Never leave repositories in an inconsistent state. If API changes break the SDK, agent, UI, or CLI, they MUST be updated in the same task.

#### Write Tests as You Code (MANDATORY)

**Critical Rule: NEVER write code without corresponding tests. Tests are not optional or an afterthought.**

**Unit Tests (Go):**
- For EVERY handler/function, write corresponding tests in `*_test.go` files
- Write tests BEFORE or ALONGSIDE implementation - not after
- Follow Go testing conventions with table-driven tests
- Minimum coverage requirements:
  - **Happy path**: Valid inputs with expected behavior
  - **Edge cases**: Boundary conditions, empty/nil values, maximum limits
  - **Error cases**: Error handling, validation failures, and recovery
  - **Integration points**: Service dependencies, SDK calls, database operations
  - **Concurrent operations**: Race conditions and thread safety where applicable
- Run tests incrementally as you code: `go test -v ./path/to/package/...`
- Maintain or improve existing code coverage percentage
- Update existing tests when modifying code
- Use test helpers and mocks for external dependencies

**Integration Tests:**
- Write integration tests for multi-service interactions
- Test API endpoints with real database connections (test DB)
- Verify SDK client interactions with actual API calls
- Test message queue and event processing flows
- Place in `*_integration_test.go` files or `integration/` directory
- Run with build tags: `go test -tags=integration -v ./...`

**End-to-End (E2E) Tests:**
- Required for user-facing features and critical workflows
- Test complete user journeys across multiple services
- Examples:
  - Agent registration → metric collection → API storage → UI display
  - Alert rule creation → condition trigger → notification delivery
  - User signup → organization creation → infrastructure provisioning
- Use realistic test data and scenarios
- Verify cross-repository functionality works end-to-end
- Place in `e2e/` directory or component test directories
- Run with: `go test -tags=e2e -v ./e2e/...`
- Document test scenarios and expected outcomes

**Testing Workflow Integration:**
```bash
# 1. Write test first (TDD) or alongside code
# 2. Implement feature
# 3. Run unit tests
go test -v ./pkg/handlers/...

# 4. Run integration tests
go test -tags=integration -v ./...

# 5. Run E2E tests for affected workflows
go test -tags=e2e -v ./e2e/...

# 6. Check coverage
go test -cover -coverprofile=coverage.out ./...
go tool cover -html=coverage.out
```

**Quality Gates for Testing:**
- [ ] Every new function has corresponding unit tests
- [ ] All edge cases and error paths covered
- [ ] Integration tests pass for service interactions
- [ ] E2E tests pass for complete user workflows
- [ ] Code coverage maintained or improved (target: 80%+)
- [ ] No flaky tests - all tests must be deterministic
- [ ] Tests run in reasonable time (<5min for unit, <15min for full suite)

**If implementation reveals issues with approved plan, stop and report immediately**

**TodoWrite Update**: Mark all implementation tasks as completed, mark "Build verification" as in_progress.

### 5. Quality Assurance Cycle (MANDATORY)

#### Step 5.1: Build Verification (BLOCKER)

**Primary Repository ({{PROJECT_NAME}}):**
```bash
make build-api-local
# OR for specific components:
make build-agent-local
make build-ui-local
```

**Additional Repositories (if updated):**
```bash
# go-sdk
cd /home/mmattox/go/src/github.com/{{PROJECT_NAME}}/go-sdk
go build ./...
go test -v ./...

# linux-agent
cd /home/mmattox/go/src/github.com/{{PROJECT_NAME}}/linux-agent
make build
make test

# web-ui
cd /home/mmattox/go/src/github.com/{{PROJECT_NAME}}/web-ui
npm run build
npm run type-check

# cli
cd /home/mmattox/go/src/github.com/{{PROJECT_NAME}}/cli
make build
go test -v ./...
```

- **MUST PASS**: Zero compilation errors, zero linting errors in ALL modified repositories
- If any build fails, fix immediately before proceeding
- All repositories must build successfully before moving to QA

**TodoWrite Update**: Mark "Build verification" as completed, mark "QA review" as in_progress.

#### Step 5.2: QA Agent Review (BLOCKER)

- Invoke QA agent using Task tool with qa-engineer subagent
- Provide QA agent with:
  - Changed files and context (ALL repositories)
  - Original task requirements
  - Success criteria
  - Master design document references
  - Cross-repository impact analysis

- QA agent will verify:
  - Code quality and standards compliance
  - Handler architecture patterns followed
  - Logging implementation complete
  - Database migrations in correct locations
  - SDK usage proper
  - **SDK synchronization** (if API changes made):
    - [ ] go-sdk updated with new/changed endpoints
    - [ ] SDK models synchronized with API models
    - [ ] SDK unit tests written and passing
    - [ ] SDK version tagged appropriately (semantic versioning)
    - [ ] Dependent repositories updated (linux-agent, cli, web-ui)
    - [ ] Integration tests pass across repositories
  - No repository cleanliness violations
  - Integration with existing systems
  - Security considerations
  - **Multi-repo consistency**: All affected repos updated correctly
  - **Version compatibility**: SDK versions match across repos
  - **Integration points**: Repos work together correctly

- **BLOCKER**: Address ALL QA findings in ALL repositories before proceeding

**TodoWrite Update**: Mark "QA review" as completed, mark "Testing" as in_progress.

#### Step 5.3: Run Full Test Suite

```bash
# Component tests
make test-api-local
make test-agent-local
make test-ui-local

# Full validation (mirrors CI/CD pipeline exactly)
make validate-pipeline-local

# Quick validation (format, vet, lint only - no tests)
make validate-quick

# Validate specific component
make validate-component COMPONENT=api
```

**Validation checks:**
- Code formatting (gofmt)
- Static analysis (go vet)
- Advanced linting (staticcheck)
- Security scanning (gosec)
- Tests with race detection

- Report test coverage and results
- **If tests fail**: Fix issues, re-run QA review if needed, re-test
- Test results must include: files tested, pass/fail counts, coverage %

See [Local CI/CD Validation Guide](local-cicd-validation-guide.md) for troubleshooting

**TodoWrite Update**: Mark "Testing" as completed, mark "Deployment" as in_progress.

### 6. Deployment (MANDATORY)

```bash
# Standard deployment
make bump

# Enhanced deployment with monitoring (recommended for long builds)
make bump-with-monitoring

# Monitor progress
make gh-watch              # Real-time monitoring
make gh-status             # Check build status
make gh-logs               # View build logs
```

#### Deployment Success Criteria
- Command executes without errors
- GitHub Actions build completes successfully
- Services restart successfully in target environment
- Health checks pass
- No deployment rollbacks

**If deployment fails, enter fix cycle (fix → build → QA → deploy)**

**TodoWrite Update**: Mark "Deployment" as completed, mark "Devils advocate verification" as in_progress.

### 7. Devils Advocate Verification (MANDATORY)

- Use Task tool with appropriate verification agent
- Provide complete context:
  - Original task specification
  - Implementation details
  - Test results
  - Deployment status

- Devils advocate will verify:
  - Original request was fully fulfilled
  - Business requirements met
  - No edge cases missed
  - Implementation matches task specification
  - All success criteria satisfied

#### If issues found: Enter Quality Cycle
- Fix identified issues
- Build verification (`make build-api-local`)
- QA agent re-review
- Re-deploy (`make bump`)
- Devils advocate re-verification
- **STAY IN CYCLE** until fully resolved

**TodoWrite Update**: When devils advocate approves, mark "Devils advocate verification" as completed, mark "Task completion" as in_progress.

### 8. Task Completion (ONLY AFTER DEVILS ADVOCATE APPROVAL)

#### Provide completion summary:
- **Primary Repository ({{PROJECT_NAME}})**:
  - Files changed/created/deleted
  - Tests written/updated with coverage metrics
  - Test results (all passing)
  - Deployment confirmation
  - **For API changes**: SDK synchronization verified (see below)

- **SDK Synchronization Verification (MANDATORY for API changes)**:
  - [ ] go-sdk updated with new/changed endpoints
  - [ ] SDK models match API models (field names, types, json tags)
  - [ ] SDK unit tests written with >80% coverage
  - [ ] All SDK tests passing: `go test -v ./...`
  - [ ] SDK build successful: `go build ./...`
  - [ ] README.md updated with usage examples
  - [ ] SDK version tagged (v2.x.x) with semantic versioning
  - [ ] No breaking changes unless major version bump
  - [ ] Dependent repositories updated and tested
  - **BLOCKER**: Cannot mark task complete if SDK update required but not done

- **Additional Repositories (if updated)**:
  - **go-sdk**: Version bumped to v2.x.x, new methods added, tests passing
  - **linux-agent**: New collectors/features, tests passing, build successful
  - **web-ui**: API client synced, components updated, build successful
  - **cli**: New commands added, tests passing, build successful

- Any warnings or notes
- Cross-repository integration tested and verified

#### Update TaskForge task system:
- Mark task as completed using `mcp__taskforge__updateTask({taskId: <id>, status: "completed"})`
- This automatically records completion timestamp in database
- Task is now marked complete across all sessions
- Note any discoveries or deviations from design in task notes
- All repositories modified are documented in commit messages

#### Commit with proper format in ALL modified repositories:

**Primary Repository ({{PROJECT_NAME}}):**
```bash
cd /home/mmattox/go/src/github.com/{{PROJECT_NAME}}/{{PROJECT_NAME}}
git add .
git commit -m "feat(component): brief description

- Detailed changes
- Task completion: [task description]
- Related repos: go-sdk, linux-agent, web-ui, cli (as applicable)

🤖 Generated with [Claude Code](https://claude.com/claude-code)
Co-Authored-By: Claude <noreply@anthropic.com>"
git push origin main
```

**go-sdk (if updated):**
```bash
cd /home/mmattox/go/src/github.com/{{PROJECT_NAME}}/go-sdk
git add .
git commit -m "feat: add support for [feature]

- Add [service] client methods
- Update models for [feature]
- Related to {{PROJECT_NAME}} task: [task description]

🤖 Generated with [Claude Code](https://claude.com/claude-code)
Co-Authored-By: Claude <noreply@anthropic.com>"
git tag v2.x.x
git push origin main --tags
```

**linux-agent (if updated):**
```bash
cd /home/mmattox/go/src/github.com/{{PROJECT_NAME}}/linux-agent
git add .
git commit -m "feat: add [metric/feature] collection

- Update collectors for [feature]
- Upgrade SDK to v2.x.x
- Related to {{PROJECT_NAME}} task: [task description]

🤖 Generated with [Claude Code](https://claude.com/claude-code)
Co-Authored-By: Claude <noreply@anthropic.com>"
git push origin main
```

**web-ui (if updated):**
```bash
cd /home/mmattox/go/src/github.com/{{PROJECT_NAME}}/web-ui
git add .
git commit -m "feat: add UI for [feature]

- Sync API client with latest spec
- Add [components] for [feature]
- Related to {{PROJECT_NAME}} task: [task description]

🤖 Generated with [Claude Code](https://claude.com/claude-code)
Co-Authored-By: Claude <noreply@anthropic.com>"
git push origin main
```

**cli (if updated):**
```bash
cd /home/mmattox/go/src/github.com/{{PROJECT_NAME}}/cli
git add .
git commit -m "feat: add [command/feature]

- Add [commands] for [feature]
- Upgrade SDK to v2.x.x
- Related to {{PROJECT_NAME}} task: [task description]

🤖 Generated with [Claude Code](https://claude.com/claude-code)
Co-Authored-By: Claude <noreply@anthropic.com>"
git push origin main
```

**TodoWrite Update**: Mark "Task completion" as completed. Clean up todo list - all workflow steps done.

**Await next task assignment**

---

## {{PROJECT_NAME}}-Specific Requirements

### ABSOLUTE RULES

1. **ONE TASK AT A TIME** - Never work on multiple tasks simultaneously
2. **RESEARCH BEFORE CODING** - Mandatory research phase, no exceptions
3. **DESIGN COMPLIANCE** - Must follow master-design-doc.md specifications
4. **GO 1.24 ONLY** - Verify version in all go.mod files
5. **BUILD VERIFICATION** - Must pass before QA review
6. **QA VALIDATION** - Must pass before deployment
7. **DEPLOYMENT MANDATORY** - Must deploy with make bump
8. **DEVILS ADVOCATE** - Must pass before completion
9. **QUALITY CYCLE** - Stay in cycle until fully resolved
10. **PRIORITY ENFORCEMENT** - Never skip priority levels
11. **SDK SYNCHRONIZATION** - Must update go-sdk before marking API tasks complete

### FORBIDDEN ACTIONS

- Starting new task before current fully completed
- Skipping workflow steps
- Proceeding with failed builds
- Ignoring QA findings
- Bypassing deployment
- Marking complete without devils advocate approval
- Creating files in repository root
- Using Go version < 1.24
- Adding database models to only one location
- Making direct API calls instead of using SDK
- Marking API task complete without updating go-sdk
- Creating new API endpoints without corresponding SDK methods
- Changing API models without updating SDK models
- Deploying API changes before SDK is tagged and available

### WEB UI-API SYNCHRONIZATION

- If making API changes, automated notification triggers to Web UI
- Monitor web-ui repository for sync PR creation
- Breaking changes require manual approval of migration guide
- Never bypass synchronization system

### EMERGENCY PROCEDURES

If workflow cannot be completed due to infrastructure issues:

1. Document blocker in task description or create new task for blocker resolution
2. Use TaskForge to track blocker: create task using `mcp__taskforge__createTask` with high/urgent priority
3. Link blocker using `blockedByTaskId` field to prevent dependent work
4. Do not start new tasks until blocker resolved
5. Escalate for assistance

---

## Common Multi-Repository Scenarios

### Scenario 1: Adding New API Endpoint

**Repositories affected**: {{PROJECT_NAME}}, go-sdk, web-ui

**Workflow**:
1. Create endpoint in {{PROJECT_NAME}} with handler, model, routes
2. Update Swagger documentation
3. Add SDK method in go-sdk for new endpoint
4. Tag new SDK version
5. Sync web-ui API client (`npm run sync-api-full`)
6. Test integration between all three repos
7. Deploy {{PROJECT_NAME}}, verify SDK clients work

### Scenario 2: Adding New Metric Collection

**Repositories affected**: {{PROJECT_NAME}}, go-sdk, linux-agent, web-ui

**Workflow**:
1. Create metric storage endpoint in {{PROJECT_NAME}}
2. Add database model for new metric
3. Update go-sdk with metric submission method
4. Tag new SDK version
5. Update linux-agent to collect new metric
6. Update linux-agent SDK dependency
7. Sync web-ui for new metric visualization
8. Test end-to-end: agent collects → API stores → UI displays

### Scenario 3: New CLI Command

**Repositories affected**: {{PROJECT_NAME}}, go-sdk, cli

**Workflow**:
1. Create API endpoint(s) for CLI functionality in {{PROJECT_NAME}}
2. Add SDK methods in go-sdk
3. Tag new SDK version
4. Add CLI command in cli using new SDK methods
5. Update CLI SDK dependency
6. Test CLI command against live API

### Scenario 4: Breaking API Change

**Repositories affected**: ALL ({{PROJECT_NAME}}, go-sdk, linux-agent, web-ui, cli)

**Workflow**:
1. Make API changes in {{PROJECT_NAME}} (with deprecation warnings if possible)
2. Update go-sdk to support both old and new (if possible), or just new
3. Tag new SDK version with major version bump
4. Update linux-agent for compatibility
5. Update web-ui (breaking changes require manual review)
6. Update cli for compatibility
7. Create migration guide
8. Coordinate rollout strategy

### Scenario 5: Bug Fix in SDK

**Repositories affected**: go-sdk, (potentially linux-agent, cli)

**Workflow**:
1. Fix bug in go-sdk
2. Tag patch version
3. Update linux-agent if it uses affected SDK code
4. Update cli if it uses affected SDK code
5. Test that fix resolves issue in dependent repos

### Multi-Repository Testing Checklist

Before marking task complete, verify:
- [ ] Each repository builds successfully individually
- [ ] Integration tests pass across repository boundaries
- [ ] SDK version dependencies are correct and consistent
- [ ] No circular dependencies introduced
- [ ] Documentation updated in all affected repos
- [ ] Each repo has appropriate commit messages
- [ ] Version tags applied where appropriate (especially SDK)
- [ ] No breaking changes without migration path
- [ ] All repos deployed/released in correct order

---

## Quality Gates Summary

| Phase | Gate | Blocker | Tool |
|-------|------|---------|------|
| Research | Design approved | Yes | Manual approval |
| Build | Binary compiles | Yes | `make build-api-local` |
| QA | Agent review passes | Yes | Task tool (qa-engineer) |
| Test | All tests pass | Yes | `make test-api-local` |
| Deploy | Deployment succeeds | Yes | `make bump` |
| Verify | Devils advocate approves | Yes | Task tool (verification) |
| Complete | All gates passed | Yes | Manual confirmation |

---

## Workflow Diagram

```
┌─────────────────────────────────────────────────────────────┐
│ 1. Receive Task                                             │
│    - Verify priority & dependencies                         │
│    - Mark [IN-PROGRESS]                                     │
└────────────────────────┬────────────────────────────────────┘
                         │
                         ▼
┌─────────────────────────────────────────────────────────────┐
│ 2. Research & Design Phase (MANDATORY)                      │
│    - Review master design doc                               │
│    - Identify dependencies                                  │
│    - Analyze cross-repository impact                        │
│    - Present implementation plan                            │
└────────────────────────┬────────────────────────────────────┘
                         │
                         ▼
┌─────────────────────────────────────────────────────────────┐
│ 3. Wait for Approval (GATE)                                 │
│    ✅ Approve → Continue                                     │
│    🔄 Revise → Back to Step 2                               │
│    ⏭️ Skip → Await new task                                 │
└────────────────────────┬────────────────────────────────────┘
                         │ (approved)
                         ▼
┌─────────────────────────────────────────────────────────────┐
│ 4. Implementation Phase                                     │
│    - Work in dependency order (API→SDK→Agent/UI/CLI)        │
│    - Follow all coding standards                            │
│    - Write tests alongside code                             │
│    - Update ALL affected repositories                       │
└────────────────────────┬────────────────────────────────────┘
                         │
                         ▼
┌─────────────────────────────────────────────────────────────┐
│ 5.1 Build Verification (BLOCKER)                            │
│     Build ALL modified repositories                         │
│     ✓ {{PROJECT_NAME}}, go-sdk, linux-agent, web-ui, cli           │
│     ❌ Any failed → Fix → Retry                              │
└────────────────────────┬────────────────────────────────────┘
                         │ (all passed)
                         ▼
┌─────────────────────────────────────────────────────────────┐
│ 5.2 QA Agent Review (BLOCKER)                               │
│     Review ALL modified repositories                        │
│     Verify multi-repo consistency                           │
│     ❌ Issues found → Fix → Retry                            │
└────────────────────────┬────────────────────────────────────┘
                         │ (passed)
                         ▼
┌─────────────────────────────────────────────────────────────┐
│ 5.3 Run Test Suite (BLOCKER)                                │
│     Test ALL modified repositories                          │
│     Verify integration between repos                        │
│     ❌ Tests failed → Fix → Back to 5.2                      │
└────────────────────────┬────────────────────────────────────┘
                         │ (passed)
                         ▼
┌─────────────────────────────────────────────────────────────┐
│ 6. Deployment (MANDATORY)                                   │
│    Deploy primary repo: make bump                           │
│    Commit/tag other repos in dependency order               │
│    ❌ Failed → Fix → Back to 5.1                             │
└────────────────────────┬────────────────────────────────────┘
                         │ (deployed)
                         ▼
┌─────────────────────────────────────────────────────────────┐
│ 7. Devils Advocate Verification (MANDATORY)                 │
│    Verify ALL repositories updated correctly                │
│    Test cross-repository integration                        │
│    ❌ Issues found → Back to 5.1 (Quality Cycle)            │
└────────────────────────┬────────────────────────────────────┘
                         │ (approved)
                         ▼
┌─────────────────────────────────────────────────────────────┐
│ 8. Task Completion                                          │
│    - Provide summary for ALL repos                          │
│    - Mark [COMPLETED - YYYY-MM-DD]                          │
│    - Commit ALL repositories                                │
│    - Verify no inconsistent state                           │
│    - Await next task                                        │
└─────────────────────────────────────────────────────────────┘
```

### Multi-Repository Flow

```
Task Starts
     │
     ▼
┌─────────────────────────────────────────────────────────────┐
│                  Primary Repository                         │
│                    ({{PROJECT_NAME}})                               │
│  - API changes                                              │
│  - Database models                                          │
│  - Core services                                            │
└────────────────────────┬────────────────────────────────────┘
                         │
                         ▼
┌─────────────────────────────────────────────────────────────┐
│              SDK Repository (go-sdk)                        │
│  - Add/update client methods                                │
│  - Sync models                                              │
│  - Tag new version                                          │
└─────────┬───────────────────────┬───────────────────────────┘
          │                       │
          ▼                       ▼
┌──────────────────┐    ┌──────────────────────────────────────┐
│   linux-agent    │    │          web-ui                      │
│  - New collectors│    │  - Sync API client                   │
│  - Update SDK    │    │  - Update components                 │
│  - Test metrics  │    │  - Build verification                │
└──────────────────┘    └──────────────────────────────────────┘
          │                       │
          └───────────┬───────────┘
                      ▼
            ┌──────────────────┐
            │       cli        │
            │  - New commands  │
            │  - Update SDK    │
            │  - Test locally  │
            └──────────────────┘
                      │
                      ▼
              All repos in sync
              Task complete
```

---

This workflow ensures systematic, quality-driven development aligned with {{PROJECT_NAME}} architecture and standards.
