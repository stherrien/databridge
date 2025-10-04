# DataBridge CI/CD Pipeline Documentation

Complete guide for the continuous integration and deployment pipelines for DataBridge.

## Table of Contents

- [Overview](#overview)
- [Workflows](#workflows)
- [Setup Instructions](#setup-instructions)
- [Pipeline Details](#pipeline-details)
- [Deployment](#deployment)
- [Best Practices](#best-practices)

## Overview

DataBridge uses GitHub Actions for CI/CD automation, providing:

- **Automated Testing**: Unit, integration, and security tests
- **Multi-Platform Builds**: Linux, macOS, Windows binaries
- **Docker Images**: Automated container builds and publishing
- **Security Scanning**: CodeQL, Gosec, and dependency reviews
- **Automated Releases**: GitHub releases with artifacts
- **Kubernetes Deployment**: Automated deployment to staging/production

## Workflows

### 1. CI Pipeline (`ci.yml`)

**Triggers**: Push to `main`/`develop`, Pull requests

**Jobs**:
- Backend tests with coverage reporting
- Backend linting (golangci-lint)
- Security scanning (gosec)
- Frontend tests and linting
- Integration tests
- Docker image build

**Status Badges**:
```markdown
![CI](https://github.com/shawntherrien/databridge/workflows/CI%20Pipeline/badge.svg)
```

### 2. Release Pipeline (`release.yml`)

**Triggers**: Git tags matching `v*` (e.g., `v1.0.0`)

**Jobs**:
- Pre-release testing
- Multi-platform binary builds (6 platforms)
- Docker image build and push
- GitHub Release creation
- Staging deployment (optional)

**Supported Platforms**:
- `linux/amd64`
- `linux/arm64`
- `darwin/amd64`
- `darwin/arm64`
- `windows/amd64`

### 3. Docker Workflow (`docker.yml`)

**Triggers**: Push to `main`/`develop`, Manual dispatch

**Jobs**:
- Build and push production images
- Build and push development images
- Multi-architecture support (amd64, arm64)

**Image Tags**:
- `main` branch → `latest`
- `develop` branch → `develop`
- Commits → `<branch>-<sha>`

### 4. CodeQL Security Scan (`codeql.yml`)

**Triggers**: Push, PRs, Weekly schedule (Mondays)

**Languages Analyzed**:
- Go (backend)
- JavaScript/TypeScript (frontend)

**Queries**: `security-extended`, `security-and-quality`

### 5. Dependency Review (`dependency-review.yml`)

**Triggers**: Pull requests

**Features**:
- Vulnerability scanning
- License compliance checking
- Automatic PR comments with findings
- Fails on moderate+ severity

## Setup Instructions

### Prerequisites

1. **GitHub Repository Secrets** (Settings → Secrets and variables → Actions):

```bash
# Required for Docker registry
GITHUB_TOKEN  # Automatically provided

# Optional: For Kubernetes deployment
KUBE_CONFIG_STAGING      # Base64-encoded kubeconfig
KUBE_CONFIG_PRODUCTION   # Base64-encoded kubeconfig

# Optional: For Codecov integration
CODECOV_TOKEN            # From codecov.io
```

2. **GitHub Repository Settings**:

Enable these features:
- Actions: General → Allow all actions
- Pages: Optional for documentation
- Environments: Create `staging` and `production` environments

### Local Setup

Install tools for local CI validation:

```bash
# Install act (run GitHub Actions locally)
brew install act

# Install pre-commit hooks
pip install pre-commit
pre-commit install

# Install golangci-lint
brew install golangci-lint

# Install gosec
go install github.com/securecodewarrior/gosec/v2/cmd/gosec@latest
```

### Running Workflows Locally

```bash
# Run CI workflow locally
act -j backend-test

# Run specific job
act -j backend-lint

# List all workflows
act -l

# Run with secrets
act -s GITHUB_TOKEN=your_token
```

## Pipeline Details

### Backend Testing

```yaml
- Unit tests with race detection
- Coverage reporting (minimum 75%)
- Upload to Codecov
- HTML coverage reports as artifacts
```

**Coverage Requirements**:
- Minimum: 75%
- Target: 85%+

**Running Locally**:
```bash
make test-coverage
open coverage/coverage.html
```

### Frontend Testing

```yaml
- ESLint code quality checks
- TypeScript type checking
- Jest unit tests (when configured)
- Production build validation
```

**Running Locally**:
```bash
cd frontend
npm run lint
npm run type-check
npm test
npm run build
```

### Security Scanning

#### Gosec (Go Security)
```bash
# Run locally
gosec -fmt sarif -out gosec-results.sarif ./...
```

#### CodeQL
- Runs automatically on push/PR
- Weekly scheduled scans
- Results in Security tab

#### Dependency Review
- Automatic on PRs
- Checks for:
  - Known vulnerabilities
  - License issues
  - Supply chain risks

### Docker Image Building

**Build Arguments**:
```dockerfile
ARG VERSION=dev
ARG BUILD_DATE
ARG GIT_COMMIT
```

**Tags Strategy**:
- Release tags: `v1.0.0`, `v1.0`, `v1`, `latest`
- Branch commits: `main-abc123`, `develop-abc123`
- Development: `dev`, `dev-abc123`

**Multi-arch Builds**:
```bash
# GitHub Actions builds for:
- linux/amd64
- linux/arm64

# Local multi-arch build:
docker buildx build --platform linux/amd64,linux/arm64 -t databridge:multi .
```

### Integration Tests

Location: `./test/integration/`

```bash
# Run locally
make test-integration

# Or directly with Go
go test -v -tags=integration ./test/integration/...
```

## Deployment

### Staging Deployment

**Automatic**: On release tags (non-beta)

**Steps**:
1. Release workflow triggered by tag
2. Docker image built and pushed
3. Kubernetes deployment updated
4. Rollout status verified

**Manual Deployment**:
```bash
# Update image
kubectl set image statefulset/databridge \
  databridge=ghcr.io/shawntherrien/databridge:v1.0.0 \
  -n databridge-staging

# Watch rollout
kubectl rollout status statefulset/databridge -n databridge-staging
```

### Production Deployment

**Process**: Manual approval required

1. Create release tag: `git tag v1.0.0 && git push origin v1.0.0`
2. Release workflow runs automatically
3. Review release in GitHub
4. Approve production deployment (if configured)

**Manual Production Deploy**:
```bash
# Apply production config
kubectl apply -k deploy/kubernetes/overlays/production

# Or update image
kubectl set image statefulset/databridge \
  databridge=ghcr.io/shawntherrien/databridge:v1.0.0 \
  -n databridge-production
```

### Rollback

```bash
# Rollback to previous version
kubectl rollout undo statefulset/databridge -n databridge-production

# Rollback to specific version
kubectl rollout undo statefulset/databridge \
  -n databridge-production \
  --to-revision=2

# Check rollout history
kubectl rollout history statefulset/databridge -n databridge-production
```

## Creating a Release

### Semantic Versioning

DataBridge follows [SemVer](https://semver.org/):
- `MAJOR.MINOR.PATCH` (e.g., `1.2.3`)
- `vMAJOR.MINOR.PATCH-beta.N` for pre-releases

### Release Process

1. **Update Version Files**:
```bash
# Update CHANGELOG.md
vim CHANGELOG.md

# Commit changes
git add CHANGELOG.md
git commit -m "chore: prepare release v1.0.0"
git push origin main
```

2. **Create and Push Tag**:
```bash
# Create annotated tag
git tag -a v1.0.0 -m "Release v1.0.0"

# Push tag
git push origin v1.0.0
```

3. **Monitor Workflow**:
```bash
# GitHub Actions will automatically:
# - Run tests
# - Build binaries for all platforms
# - Build and push Docker images
# - Create GitHub Release with artifacts
```

4. **Verify Release**:
- Check GitHub Releases page
- Verify Docker images: `docker pull ghcr.io/shawntherrien/databridge:v1.0.0`
- Test binaries from release assets

### Pre-release (Beta/Alpha)

```bash
# Beta release
git tag -a v1.0.0-beta.1 -m "Beta release v1.0.0-beta.1"
git push origin v1.0.0-beta.1

# Alpha release
git tag -a v1.0.0-alpha.1 -m "Alpha release v1.0.0-alpha.1"
git push origin v1.0.0-alpha.1
```

## Dependabot Configuration

Automated dependency updates for:
- **Go modules**: Weekly (Mondays)
- **NPM packages**: Weekly (Mondays)
- **GitHub Actions**: Weekly (Mondays)
- **Docker base images**: Weekly (Mondays)

**Auto-merge Strategy**:
```bash
# Enable auto-merge for minor/patch updates
gh pr merge PR_NUMBER --auto --squash
```

## Status Checks

### Required Checks (for PRs)

Configure in: Settings → Branches → Branch protection rules

Recommended required checks:
- ✅ Backend Tests
- ✅ Backend Linting
- ✅ Security Scan
- ✅ Frontend Tests
- ✅ Docker Build
- ✅ CodeQL Analysis

### Branch Protection

Recommended settings for `main` branch:
```yaml
Require pull request before merging: true
Require approvals: 1
Dismiss stale PR approvals: true
Require status checks to pass: true
Require branches to be up to date: true
Require conversation resolution: true
```

## Monitoring and Alerts

### GitHub Actions Insights

View workflow metrics:
- Actions tab → Click workflow → Metrics
- Shows: Success rate, duration, frequency

### Notifications

Configure notifications:
1. Watch repository
2. Settings → Notifications
3. Enable: Actions workflow runs

### Webhook Integration

Send workflow events to external services:

```yaml
# Example: Slack notification
- name: Slack notification
  uses: 8398a7/action-slack@v3
  with:
    status: ${{ job.status }}
    webhook_url: ${{ secrets.SLACK_WEBHOOK }}
  if: always()
```

## Troubleshooting

### Workflow Failures

#### Backend Test Failures
```bash
# Check logs in Actions tab
# Run locally to reproduce:
make test-unit

# Check for race conditions:
go test -race ./...
```

#### Docker Build Failures
```bash
# Check build context size
du -sh .

# Verify .dockerignore
cat .dockerignore

# Test build locally:
docker build -t databridge:test .
```

#### Deployment Failures
```bash
# Check kubectl config
kubectl config current-context

# Verify image pull
kubectl describe pod <pod-name> -n databridge-staging

# Check rollout status
kubectl rollout status statefulset/databridge -n databridge-staging
```

### Common Issues

**Issue**: Coverage upload fails
```yaml
# Solution: Verify CODECOV_TOKEN secret
# Or disable upload temporarily:
# - name: Upload coverage to Codecov
#   if: false
```

**Issue**: Multi-arch build takes too long
```yaml
# Solution: Use cache more effectively
cache-from: type=gha
cache-to: type=gha,mode=max
```

**Issue**: Dependabot PRs failing
```bash
# Update dependencies locally first:
go get -u ./...
go mod tidy

# Test before committing:
make test
```

## Best Practices

### Commits and PRs

1. **Conventional Commits**:
```bash
feat: add new processor
fix: resolve race condition
docs: update README
chore: update dependencies
ci: improve workflow performance
```

2. **PR Size**: Keep PRs small and focused
3. **Tests**: Add tests for new features
4. **Documentation**: Update docs with changes

### Testing Strategy

1. **Unit Tests**: Test individual functions/methods
2. **Integration Tests**: Test component interactions
3. **E2E Tests**: Test full workflows (future)
4. **Performance Tests**: Benchmark critical paths

### Security

1. **No Secrets in Code**: Use GitHub Secrets
2. **Dependency Updates**: Review Dependabot PRs promptly
3. **Security Scans**: Monitor CodeQL results
4. **Least Privilege**: Limit workflow permissions

### Performance

1. **Caching**: Use Go mod cache and Docker cache
2. **Parallel Jobs**: Run independent jobs concurrently
3. **Matrix Builds**: Build multiple platforms in parallel
4. **Artifact Pruning**: Clean up old artifacts

## Additional Resources

- [GitHub Actions Documentation](https://docs.github.com/en/actions)
- [Docker Build Push Action](https://github.com/docker/build-push-action)
- [CodeQL Documentation](https://codeql.github.com/docs/)
- [Dependabot Configuration](https://docs.github.com/en/code-security/dependabot)

## Support

For CI/CD issues:
1. Check workflow logs in Actions tab
2. Review this documentation
3. Create an issue with workflow run URL
4. Contact maintainers
