# Development Workflows

This directory contains standardized workflow templates for common development activities.

## Available Workflows

### Core Workflows
- **[project-initialization.md](project-initialization.md)** - Project setup and initialization
- **[prd-creation.md](prd-creation.md)** - Product Requirements Document creation
- **[feature-development.md](feature-development.md)** - Complete feature development lifecycle
- **[sprint-management.md](sprint-management.md)** - Agile sprint ceremonies and tracking
- **[deployment.md](deployment.md)** - Deployment pipeline and release process

### Supporting Workflows
- **[integration-setup.md](integration-setup.md)** - Third-party integration configuration
- **[dev-environment.md](dev-environment.md)** - Development environment setup
- **[cicd-setup.md](cicd-setup.md)** - CI/CD pipeline configuration
- **[architecture-review.md](architecture-review.md)** - Architecture design review process
- **[testing-strategy.md](testing-strategy.md)** - Testing approach and strategy

## How to Use Workflows

1. **Identify the workflow** - Determine which workflow applies to your task
2. **Read the workflow** - Understand the steps and prerequisites
3. **Follow sequentially** - Complete each step before proceeding
4. **Engage agents** - Delegate to appropriate specialized agents
5. **Track progress** - Update PROJECT_STATUS.md regularly
6. **Document decisions** - Record rationale and outcomes

## Workflow Structure

Each workflow document includes:

- **Purpose**: What this workflow accomplishes
- **When to Use**: Triggers and use cases
- **Prerequisites**: Required setup or conditions
- **Agents Involved**: Which agents participate
- **Steps**: Detailed step-by-step process
- **Quality Gates**: Required approvals and validations
- **Artifacts**: Deliverables produced
- **Common Issues**: Known problems and solutions

## Customizing Workflows

Workflows can be customized for your project:

1. Copy the template workflow
2. Modify steps for your process
3. Add/remove quality gates as needed
4. Document project-specific requirements
5. Update agent involvement

## Integration with Task Execution

All workflows follow the mandatory [Task Execution Workflow](../development/task-execution-workflow.md):

1. Task Selection & Research
2. Design & Planning
3. Approval Gate
4. Implementation
5. Quality Assurance
6. Deployment
7. Devils Advocate Verification
8. Task Completion

## Quality Gates

Each workflow enforces quality gates:
- Build verification
- QA review
- Devils Advocate verification
- Deployment validation
- Final approval

See individual workflows for specific gate requirements.

## Contributing

When adding new workflows:

1. Use the workflow template format
2. Document all steps clearly
3. Identify agent involvement
4. Define quality gates
5. Include examples
6. Test with real scenarios

## Best Practices

- **One workflow at a time**: Don't mix workflows
- **Complete all steps**: Don't skip steps
- **Engage quality gates**: Let agents review
- **Document deviations**: Note any workflow changes
- **Update regularly**: Keep workflows current with practices
