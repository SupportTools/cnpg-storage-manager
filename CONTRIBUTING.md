# Contributing to CNPG Storage Manager

Thank you for your interest in contributing to CNPG Storage Manager! This document provides guidelines for contributing to the project.

## Code of Conduct

By participating in this project, you agree to abide by our code of conduct. Please be respectful and constructive in all interactions.

## Getting Started

### Prerequisites

- Go 1.21 or later
- Docker (for building container images)
- kubectl and access to a Kubernetes cluster
- make

### Development Setup

1. Fork and clone the repository:
   ```bash
   git clone https://github.com/YOUR-USERNAME/cnpg-storage-manager.git
   cd cnpg-storage-manager
   ```

2. Install dependencies:
   ```bash
   go mod download
   ```

3. Generate manifests and code:
   ```bash
   make manifests generate
   ```

4. Run tests:
   ```bash
   make test
   ```

### Running Locally

1. Install CRDs in your cluster:
   ```bash
   make install
   ```

2. Run the controller locally:
   ```bash
   make run
   ```

## Development Workflow

### Branching Strategy

- `main` - Production-ready code
- `feature/*` - New features
- `fix/*` - Bug fixes
- `docs/*` - Documentation updates

### Making Changes

1. Create a new branch:
   ```bash
   git checkout -b feature/your-feature-name
   ```

2. Make your changes and ensure tests pass:
   ```bash
   make test
   make lint
   ```

3. Commit your changes:
   ```bash
   git commit -m "feat: description of your changes"
   ```

4. Push and create a pull request:
   ```bash
   git push origin feature/your-feature-name
   ```

### Commit Message Format

We follow [Conventional Commits](https://www.conventionalcommits.org/):

- `feat:` - A new feature
- `fix:` - A bug fix
- `docs:` - Documentation changes
- `style:` - Code style changes (formatting, etc.)
- `refactor:` - Code refactoring
- `test:` - Adding or updating tests
- `chore:` - Maintenance tasks

### Pull Request Guidelines

1. **Title**: Use a clear, descriptive title following conventional commits
2. **Description**: Explain what changes you made and why
3. **Tests**: Include tests for new functionality
4. **Documentation**: Update documentation as needed
5. **Breaking Changes**: Clearly document any breaking changes

### Code Standards

- Follow Go best practices and idioms
- Run `make lint` before submitting
- Keep functions small and focused
- Add comments for complex logic
- Include unit tests for new code

## Testing

### Unit Tests

```bash
make test
```

### Integration Tests

```bash
make test-integration
```

### E2E Tests

Requires a Kubernetes cluster with CNPG installed:

```bash
make test-e2e
```

### Test Coverage

Generate coverage report:

```bash
go test -coverprofile=coverage.out ./pkg/...
go tool cover -html=coverage.out
```

## Building

### Binary

```bash
make build
```

### Docker Image

```bash
make docker-build
```

### Helm Chart

```bash
helm lint charts/cnpg-storage-manager
helm package charts/cnpg-storage-manager
```

## Release Process

Releases are automated via GitHub Actions when a version tag is pushed:

```bash
git tag -a v1.0.0 -m "Release v1.0.0"
git push origin v1.0.0
```

## Getting Help

- **Issues**: Open a GitHub issue for bugs or feature requests
- **Discussions**: Use GitHub Discussions for questions
- **Documentation**: Check the README and docs folder

## License

By contributing, you agree that your contributions will be licensed under the Apache 2.0 License.
