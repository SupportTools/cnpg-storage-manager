# Security Policy

## Supported Versions

We release patches for security vulnerabilities for the following versions:

| Version | Supported          |
| ------- | ------------------ |
| 1.x.x   | :white_check_mark: |
| < 1.0   | :x:                |

## Reporting a Vulnerability

If you discover a security vulnerability within CNPG Storage Manager, please follow responsible disclosure practices:

1. **Do NOT** open a public GitHub issue for security vulnerabilities
2. Email security concerns to: security@supporttools.io
3. Include the following in your report:
   - Description of the vulnerability
   - Steps to reproduce
   - Potential impact
   - Suggested fix (if any)

## Response Timeline

- **Acknowledgment**: Within 48 hours
- **Initial Assessment**: Within 5 business days
- **Resolution Timeline**: Depends on severity
  - Critical: 7 days
  - High: 14 days
  - Medium: 30 days
  - Low: 90 days

## Security Best Practices

When deploying CNPG Storage Manager:

### RBAC

- Use the principle of least privilege
- The controller only needs permissions defined in `config/rbac/`
- Review ClusterRole before deployment

### Network Policies

- Consider restricting egress to only required endpoints
- Limit ingress to metrics port (8080) if exposed

### Secrets

- Never commit credentials to the repository
- Use Kubernetes Secrets for sensitive data
- Consider using external secret management (Vault, AWS Secrets Manager)

### Container Security

- Run as non-root user (already configured)
- Use read-only root filesystem where possible
- Regularly update to latest versions

### Alerting Credentials

- Store webhook URLs and API keys as Kubernetes Secrets
- Use secretKeyRef in StoragePolicy for sensitive values
- Rotate credentials regularly

## Security Scanning

This project uses:

- **Gosec**: Static security analysis for Go
- **Trivy**: Vulnerability scanning for dependencies and container images
- **Dependabot**: Automated dependency updates

Security scans run on every pull request and can be viewed in the GitHub Actions tab.

## Known Security Considerations

### Pod Exec for WAL Cleanup

The WAL cleanup feature executes commands inside CNPG pods. This requires:
- `pods/exec` permission in the controller's ClusterRole
- Trust in the container image running in CNPG pods

Mitigations:
- Commands are strictly defined (no user input in exec)
- Only targets pods with CNPG labels
- Dry-run mode available for testing

### Storage Expansion

PVC expansion requires:
- `persistentvolumeclaims` update permission
- StorageClass must support volume expansion

The controller validates StorageClass capabilities before attempting expansion.
