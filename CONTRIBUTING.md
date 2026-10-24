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

#### Topic Page Template
Every file in `docs/NN-section/` (other than the section `README.md`) follows this order:

1. `# Title` (one H1, title case, names the tools in it)
2. `## Learning Objectives`: 3 to 5 bullets starting with a verb
3. `## Prerequisites`: bullets that link to the earlier doc or lab that covers each one
4. Concept sections (`##`/`###`): short paragraphs, a Mermaid diagram for any architecture or flow
5. `## Hands-On Exercises`: numbered exercises. Each one states the goal, gives the commands,
   shows the expected output, and ends with a check the reader can run
6. `## Troubleshooting`: `### Symptom` headings, each with the cause and the fix
7. `## Further Reading`: official docs and primary sources only

#### Section README Template
`docs/NN-section/README.md` has: a one-paragraph introduction, a table of the section's docs
(file, what it covers), a suggested reading order, and the labs that use the section.

#### Formatting Guidelines
- Title Case headings below the H1, matching the template headings (for example, "Learning Objectives", "Hands-On Exercises"). Do not skip heading levels.
- Every code block has a language (`bash`, `yaml`, `go`, `mermaid`, `text`, and so on).
- Commands are copy-pasteable. Put placeholders in `<angle-brackets>` and say what to replace them with.
- Show expected output for any command the reader is meant to check.
- Link a term the first time a reader needs it, to the doc or lab that teaches it.
- Use tables for comparisons, bullets for lists of steps only when order does not matter.
- Write for a reader who has finished the previous doc: define acronyms once, then use them.
- Keep paragraphs short (three to five sentences) and wrap prose near 100 characters.
- Do not use "simply", "just", or "obviously". Do not use "etc." in a list of steps.
- Link only to docs that exist in this repo, or to official project documentation. Link to code under `code/` through its GitHub URL.

### 2. Lab Contributions

#### Adding New Labs
1. Create directory under `labs/` with format `lab-NN-short-name/`
2. Include `README.md` with objectives, prerequisites, time estimate
3. Provide step-by-step instructions with verification steps
4. Include cleanup instructions
5. Add troubleshooting section

#### Lab README Template
1. `# Lab NN: Title`, then a one-line summary
2. `## Objectives`
3. `## Prerequisites`: a table with the requirement, the version, and a note
4. `## Estimated Time`
5. `## Files in this lab`: a table of every file and what it does
6. `## Steps`: numbered. Each step gives the command, then the expected result
7. `## Verification`: commands with the expected output
8. `## Cleanup`: the exact commands, and what they leave running
9. `## Troubleshooting`: `### Symptom` headings
10. `## Next Steps`

#### Lab Standards
- Test on clean cluster (Kind/k3d)
- Provide automated setup scripts (`setup.sh`, `bootstrap.sh`) and a `teardown.sh` when the lab creates cluster state
- Include expected time to complete
- Tag difficulty: Beginner/Intermediate/Advanced
- Every path, file name, and version in a step must exist in the lab or in a file the lab links to

#### Checking the Docs Site Locally
```bash
./scripts/build-docs.sh serve    # live preview at http://127.0.0.1:8000
./scripts/build-docs.sh          # strict build; fails on broken links or nav problems
```
The site is built with MkDocs Material from `pyproject.toml` (`uv` group `docs`) and deployed by
`.github/workflows/docs.yml` on pushes to `main`.

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
