# Contributing to DataBridge

Thank you for your interest in contributing to DataBridge! This document provides guidelines and instructions for contributing.

## Code of Conduct

Please be respectful and constructive in all interactions with the community.

## Getting Started

1. **Fork the repository** on GitHub
2. **Clone your fork** locally:
   ```bash
   git clone https://github.com/YOUR_USERNAME/databridge.git
   cd databridge
   ```
3. **Add upstream remote**:
   ```bash
   git remote add upstream https://github.com/stherrien/databridge.git
   ```
4. **Create a branch** for your changes:
   ```bash
   git checkout -b feature/your-feature-name
   ```

## Development Setup

### Prerequisites
- Go 1.21 or higher
- Node.js 18 or higher
- Make

### Installing Dependencies

```bash
# Go dependencies
go mod download

# Frontend dependencies
cd frontend
npm install
```

### Running Locally

```bash
# Terminal 1: Backend
make dev

# Terminal 2: Frontend
cd frontend
npm run dev
```

## Making Changes

### Backend (Go)

1. Follow Go best practices and idioms
2. Run tests: `go test ./...`
3. Run linter: `golangci-lint run` (or will run in CI)
4. Format code: `go fmt ./...`

### Frontend (Next.js/React)

1. Follow TypeScript and React best practices
2. Run linter: `npm run lint`
3. Run type check: `npm run type-check`
4. Format code: Use Prettier/ESLint

### Commit Messages

Follow conventional commit format:
- `feat: Add new processor type`
- `fix: Resolve connection error`
- `docs: Update API documentation`
- `test: Add tests for flow handlers`
- `refactor: Simplify processor logic`
- `chore: Update dependencies`

## Testing

### Backend Tests
```bash
go test ./...
go test -v ./internal/api
go test -race ./...
```

### Frontend Tests
```bash
cd frontend
npm test
npm run type-check
```

## Submitting Changes

1. **Commit your changes**:
   ```bash
   git add .
   git commit -m "feat: Add your feature"
   ```

2. **Keep your fork updated**:
   ```bash
   git fetch upstream
   git rebase upstream/dev
   ```

3. **Push to your fork**:
   ```bash
   git push origin feature/your-feature-name
   ```

4. **Create a Pull Request** on GitHub:
   - Target the `dev` branch (not `main`)
   - Fill out the PR template completely
   - Link any related issues
   - Add screenshots if UI changes

## Pull Request Guidelines

- Target the `dev` branch for all PRs
- Include tests for new features
- Update documentation as needed
- Ensure CI passes (all tests, linting)
- Keep PRs focused (one feature/fix per PR)
- Respond to review feedback promptly

## Code Review Process

1. Automated checks must pass (CI/CD)
2. At least one approval required
3. Code owner will review (@stherrien)
4. Address all feedback
5. Squash commits if requested
6. Merge will be performed by maintainer

## Areas for Contribution

### High Priority
- Additional processor implementations
- Enhanced monitoring and metrics
- Performance optimizations
- Documentation improvements
- Test coverage

### Good First Issues
- Look for issues tagged `good-first-issue`
- Documentation updates
- Simple bug fixes
- UI improvements

### Feature Requests
- Open an issue first to discuss
- Provide use case and rationale
- Wait for maintainer feedback before implementing

## Project Structure

```
databridge/
├── cmd/databridge/        # Main entry point
├── internal/
│   ├── api/              # REST API handlers
│   ├── core/             # Core business logic
│   ├── processor/        # Processor implementations
│   └── storage/          # Data persistence
├── frontend/             # Next.js UI
└── .github/              # CI/CD and templates
```

## Style Guidelines

### Go
- Follow [Effective Go](https://golang.org/doc/effective_go)
- Use `gofmt` for formatting
- Keep functions focused and small
- Add comments for exported functions
- Handle errors explicitly

### TypeScript/React
- Use TypeScript strict mode
- Functional components with hooks
- Proper type annotations
- ESLint and Prettier for formatting

## Documentation

- Update README.md for user-facing changes
- Add inline code comments for complex logic
- Update API documentation for endpoint changes
- Include examples where helpful

## Release Process

Releases are managed by maintainers:
1. Version tagging follows semantic versioning
2. Release notes auto-generated from commits
3. Binaries built automatically via GitHub Actions

## Getting Help

- Open an issue for bugs
- Start a discussion for questions
- Tag maintainers for urgent issues

## Recognition

Contributors will be recognized in:
- GitHub contributors list
- Release notes
- Project documentation

Thank you for contributing to DataBridge!
