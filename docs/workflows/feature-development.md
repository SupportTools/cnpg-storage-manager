# Feature Development Workflow

Complete lifecycle workflow for developing and deploying a new feature.

## Purpose

Systematically develop a feature from requirements through production deployment with mandatory quality gates at each stage.

## When to Use

**Triggers**:
- "start development"
- "implement feature"
- "begin coding"
- New feature from PRD
- Sprint planning assigns feature

**Use Cases**:
- New functionality implementation
- Major feature additions
- System capability expansion

## Prerequisites

- [ ] Feature requirements defined (PRD created)
- [ ] Design approved by solution-architect
- [ ] TaskForge task created (if using)
- [ ] Development environment configured
- [ ] Git worktree/branch created (if multi-repo)

## Agents Involved

- **product-manager**: Requirements clarification
- **solution-architect**: Design and architecture
- **senior-backend-engineer**: Backend implementation
- **senior-frontend-engineer**: Frontend implementation (if applicable)
- **qa-engineer**: Test planning and execution
- **devils-advocate-qa**: Adversarial testing
- **go-kubernetes-skeptic**: Code review (Go/K8s)
- **web-application-skeptic**: Frontend review (if applicable)
- **database-skeptic**: Schema review (if database changes)
- **devops-engineer**: Deployment

## Workflow Steps

### Step 1: Task Selection & Research (MANDATORY)

**Agent**: Current developer + specialized agents as needed

**Actions**:
1. Review feature requirements from PRD
2. Research existing codebase for similar patterns
3. Identify affected components and dependencies
4. Review related documentation
5. Identify potential risks and blockers

**Quality Gate**: Cannot proceed without research phase

**Deliverable**: Research summary documenting findings

---

### Step 2: Design & Planning

**Agent**: solution-architect + relevant specialists

**Actions**:
1. Present implementation plan to architect
2. Design API contracts (if applicable)
3. Design database schema changes (if applicable)
4. Plan testing approach
5. Identify deployment strategy
6. Document risks and mitigations

**Quality Gate**: Architect must approve design before coding

**Deliverable**:
- Implementation plan document
- API specification (OpenAPI/Swagger)
- Database migration plan
- Test plan outline

---

### Step 3: Approval Gate

**Agent**: solution-architect + stakeholders

**Actions**:
1. Present complete design to architect
2. Address feedback and concerns
3. Revise design if needed
4. Obtain explicit approval

**Quality Gate**: **BLOCKING** - No code until approval

**Deliverable**: Approved design document with sign-off

---

### Step 4: Implementation

**Agent**: senior-backend-engineer / senior-frontend-engineer

**Actions**:

#### Backend Implementation (if applicable)
1. Create feature branch
2. Implement API handlers following [API Handler Standards](../development/api-handler-architecture-standards.md)
3. Write unit tests (target 80%+ coverage)
4. Update OpenAPI/Swagger documentation
5. Implement database migrations (if needed)
6. Add integration tests

#### Frontend Implementation (if applicable)
1. Implement UI components
2. Connect to API endpoints
3. Add client-side validation
4. Implement error handling
5. Add accessibility features
6. Write component tests

#### Quality During Implementation
- Follow [Task Execution Workflow](../development/task-execution-workflow.md)
- Commit frequently with descriptive messages
- Keep changes focused on feature scope
- Update documentation inline with code

**Quality Gate**: Build must succeed (`make build-local`)

**Deliverable**:
- Implemented code with tests
- Updated documentation
- Passing build

---

### Step 5: Quality Assurance

**Sub-step 5a: Build Verification**

**Actions**:
1. Run `make build-local` - must succeed
2. Run `make test-local` - all tests must pass
3. Run `make validate-pipeline-local` - all validations must pass
4. Fix any failures and repeat

**Quality Gate**: **BLOCKING** - Cannot proceed until build succeeds

---

**Sub-step 5b: QA Engineer Review**

**Agent**: qa-engineer

**Actions**:
1. Review test coverage
2. Execute test plan
3. Perform exploratory testing
4. Verify acceptance criteria
5. Test error scenarios
6. Document findings

**Quality Gate**: QA must approve before deployment

**Deliverable**:
- QA test report
- List of issues (if any)
- Approval or rejection

**If Rejected**: Return to Step 4, fix issues, repeat from 5a

---

**Sub-step 5c: Code Review**

**Agents**: Conditional based on code type
- go-kubernetes-skeptic (Go/K8s code)
- web-application-skeptic (Frontend code)
- database-skeptic (Database changes)

**Actions**:
1. Review code for anti-patterns
2. Verify best practices followed
3. Check security implications
4. Assess performance impact
5. Validate error handling

**Quality Gate**: Skeptics can block merge

**Deliverable**: Code review with approval/concerns

**If Concerns Raised**: Address issues, repeat review

---

### Step 6: Deployment

**Agent**: devops-engineer

**Sub-step 6a: Prepare for Deployment**

**Actions**:
1. Merge feature branch to main
2. Run `make bump` to increment version and trigger CI/CD
3. Monitor GitHub Actions pipeline
4. Verify Docker images built and pushed
5. Verify Helm chart published

**Quality Gate**: CI/CD must complete successfully

---

**Sub-step 6b: Deploy to Development**

**Actions**:
1. ArgoCD detects new version
2. Applies changes to dev environment
3. Monitor rollout status
4. Verify health checks pass
5. Perform smoke tests

**Quality Gate**: Development deployment must succeed

**If Failed**: Investigate, fix, redeploy

---

**Sub-step 6c: Deploy to Staging (if applicable)**

**Actions**:
1. Tag release for staging
2. ArgoCD applies to staging
3. Run full test suite against staging
4. Verify integration with other services
5. Performance testing

**Quality Gate**: Staging must be stable

---

**Sub-step 6d: Deploy to Production**

**Actions**:
1. Schedule production deployment
2. Create production release tag
3. Monitor ArgoCD rollout
4. Verify health checks
5. Monitor logs and metrics
6. Verify feature functionality

**Quality Gate**: ⚠️ **CRITICAL DECISION REQUIRED**

```
⚠️ CRITICAL DECISION: Deploy to Production
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
What: Deploy [feature name] to production
Why: Feature tested and approved in staging
Risk: [Describe potential impacts]

Do you approve? (y/n):
```

**If Issues Occur**: Rollback procedure
1. Identify issue
2. Decision: Hotfix or rollback
3. If rollback: Revert to previous version
4. If hotfix: Emergency fix following abbreviated workflow

**Deliverable**:
- Production deployment confirmation
- Post-deployment verification results

---

### Step 7: Devils Advocate Verification

**Agent**: devils-advocate-qa

**Actions**:
1. Challenge implementation decisions
2. Test edge cases not covered
3. Attempt to break the feature
4. Verify error handling
5. Check performance under stress
6. Validate security implications
7. Verify rollback capability

**Quality Gate**: **CRITICAL** - Devils Advocate must approve

**Deliverable**: Adversarial test report with final approval

**If Critical Issues Found**:
- Assess severity
- Create follow-up tasks if minor
- Rollback if critical
- Fix and redeploy if needed

---

### Step 8: Task Completion

**Agent**: Current developer

**Actions**:
1. Update TaskForge (if using):
   ```javascript
   mcp__taskforge__updateTask({
     taskId: TASK_ID,
     status: "done",
     actualHours: HOURS_SPENT
   })
   ```

2. Update PROJECT_STATUS.md:
   - Mark feature complete
   - Document deployment details
   - Note any follow-up tasks
   - Update metrics

3. Create completion summary:
   - What was delivered
   - What was learned
   - What could be improved
   - Follow-up items

4. Notify stakeholders

5. Commit all documentation updates

**Quality Gate**: Task not complete until all documentation updated

**Deliverable**:
- Updated TaskForge task (status: done)
- Updated PROJECT_STATUS.md
- Completion summary
- Notification sent

---

## Multi-Repository Considerations

If feature spans multiple repositories:

1. **Dependency Order**: nexmonyx → go-sdk → linux-agent/web-ui/cli
2. **Synchronization**: All repos must be updated before task completion
3. **Version Tagging**: Tag all repos with synchronized versions
4. **Testing**: Integration tests across all affected repos

See [Task Execution Workflow](../development/task-execution-workflow.md) for multi-repo procedures.

## Quality Gates Summary

| Gate | Agent | Can Block | Phase |
|------|-------|-----------|-------|
| Research Complete | Self | Yes | Pre-coding |
| Design Approved | solution-architect | Yes | Pre-coding |
| Build Success | Automated | Yes | Post-coding |
| QA Approval | qa-engineer | Yes | Pre-deployment |
| Code Review | Skeptics | Yes | Pre-merge |
| CI/CD Success | Automated | Yes | Deployment |
| Production Deploy | Manual | Yes | Production |
| Devils Advocate | devils-advocate-qa | Yes | Post-deployment |

## Common Issues

### Issue: Feature Scope Creep

**Symptoms**: Feature grows beyond original requirements

**Solution**:
1. Stop and reassess
2. Engage product-manager
3. Create separate tasks for additional scope
4. Complete original feature first

---

### Issue: Build Failures

**Symptoms**: `make build-local` or CI/CD fails

**Solution**:
1. Review error logs
2. Run `make validate-pipeline-local` locally
3. Fix issues systematically
4. Don't commit broken code

---

### Issue: QA Finds Critical Bugs

**Symptoms**: QA engineer rejects feature

**Solution**:
1. Document all issues
2. Prioritize fixes
3. Return to Step 4 (Implementation)
4. Fix issues
5. Re-run full QA cycle

---

### Issue: Devils Advocate Blocks Deployment

**Symptoms**: Adversarial testing finds critical issues

**Solution**:
1. **Do not argue with Devils Advocate**
2. Address concerns systematically
3. Create follow-up tasks if appropriate
4. Re-engage Devils Advocate after fixes

---

## Success Criteria

Feature is complete when:

- [ ] All quality gates passed
- [ ] Deployed to production successfully
- [ ] Devils Advocate approved
- [ ] TaskForge task marked "done"
- [ ] Documentation updated
- [ ] No critical issues outstanding
- [ ] Stakeholders notified

## Time Estimates

Typical feature development timeline:

- Research: 10% of total time
- Design & Approval: 15% of total time
- Implementation: 40% of total time
- QA & Review: 20% of total time
- Deployment: 10% of total time
- Devils Advocate: 5% of total time

**Plan accordingly and add buffer for iterations.**

## Related Workflows

- [Task Execution Workflow](../development/task-execution-workflow.md) - Mandatory process
- [PRD Creation](prd-creation.md) - Feature requirements
- [Deployment](deployment.md) - Deployment procedures
- [Testing Strategy](testing-strategy.md) - Testing approach

## Remember

**ONE task at a time. No shortcuts. No skipped gates.**

The workflow exists to prevent production issues. Trust the process.
