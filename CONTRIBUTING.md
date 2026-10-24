# Contributing to Platform Engineering on Kubernetes

Thank you for your interest in contributing! This document provides guidelines for contributing to this learning resource.

## How to Contribute

### 1. Documentation Contributions

#### Adding New Documentation
1. Follow the existing structure in `docs/`
2. Use the naming convention: `NN-topic-name.md`
3. Include all required sections (see Documentation Standards below)
4. Add cross-references to related docs, labs, and code examples

#### Documentation Standards
Each document should include:
- **Learning Objectives** - What the reader will learn
- **Prerequisites** - Required knowledge/tools
- **Conceptual Explanation** - Theory with Mermaid diagrams
- **Code Examples** - Practical, runnable examples
- **Hands-on Exercises** - Step-by-step practice
- **Further Reading** - Links to official docs, books, articles
- **Troubleshooting** - Common issues and solutions

#### Formatting Guidelines
- Use clear, concise language
- Include code blocks with language annotations
- Use Mermaid diagrams for architecture
- Use tables for comparisons
- Keep line length reasonable (~100 chars)

### 2. Lab Contributions

#### Adding New Labs
1. Create directory under `labs/` with format `lab-NN-short-name/`
2. Include `README.md` with objectives, prerequisites, time estimate
3. Provide step-by-step instructions with verification steps
4. Include cleanup instructions
5. Add troubleshooting section

#### Lab Standards
- Test on clean cluster (Kind/k3d)
- Provide automated setup scripts (`setup.sh`, `bootstrap.sh`)
- Include expected time to complete
- Tag difficulty: Beginner/Intermediate/Advanced

### 3. Code Contributions

#### Adding Code Examples
1. Place in appropriate directory under `code/`
2. Include `Makefile` with standard targets
3. Add `Dockerfile` for containerization
4. Include Helm chart in `deploy/helm/`
5. Add comprehensive tests
6. Follow language-specific best practices

#### Code Standards
- **Go**: 1.21+, controller-runtime v0.17+, kubebuilder v4
- **Tests**: Unit + integration, >80% coverage
- **Linting**: golangci-lint, go vet, staticcheck
- **Security**: govulncheck, gosec
- **Documentation**: godoc comments, README with usage

### 4. Bug Reports
Use GitHub Issues with:
- Clear title and description
- Steps to reproduce
- Expected vs actual behavior
- Environment details (K8s version, tool versions)
- Logs/error messages

### 5. Feature Requests
Use GitHub Issues with:
- Clear problem statement
- Proposed solution
- Alternative approaches considered
- Impact on existing content

## Development Setup

### Prerequisites
- Go 1.21+
- kubectl 1.28+
- kind/k3d
- Helm 3.12+
- kubebuilder (for controller development)
- golangci-lint, goimports, staticcheck

### Local Development
```bash
# Clone repository
git clone https://github.com/your-org/platform-engineering-on-kubernetes
cd platform-engineering-on-kubernetes

# Install tools
make install-tools

# Run tests
make test

# Run linters
make lint
```

## Review Process

1. All changes require PR review
2. At least one approval required
3. CI must pass (tests, linting, security scans)
4. Documentation updated for new features
5. Changelog updated for significant changes

## Style Guides

### Markdown
- Use ATX headers (# ## ###)
- Use fenced code blocks with language
- Use relative links for internal references
- Use descriptive link text

### Go
- Follow Effective Go
- Use gofmt/goimports
- Error handling: wrap with context
- Context propagation throughout

### YAML
- 2-space indentation
- Use consistent key ordering
- Comment complex configurations
- Use anchors/aliases for reuse

## License

By contributing, you agree that your contributions will be licensed under the MIT License.

## Questions?

Open a GitHub Discussion or reach out to maintainers.
