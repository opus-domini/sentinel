# Contributing to Sentinel

Thank you for contributing to Sentinel. We prioritize reliability, clear UX, and predictable APIs.

## Before You Start

- Check existing issues and pull requests to avoid duplicate work.
- Open an issue first for large changes or API-affecting proposals.
- Keep changes focused: one logical improvement per PR.

## Local Setup

1. Fork and clone your fork:
   ```bash
   git clone https://github.com/<your-user>/sentinel.git
   cd sentinel
   ```
2. Create a feature branch:
   ```bash
   git checkout -b feat/short-descriptive-name
   ```

## Development Workflow

- Implement changes with minimal complexity and clear behavior.
- Add or update tests alongside code changes.
- Run the quality pipeline before opening or updating a PR:
  ```bash
  make fmt
  make lint
  make lint-client
  make test
  make test-client
  ```
- For broader verification, also run:
  ```bash
  make test-coverage
  make build
  ```
- `make ci` is the full local gate and should pass before merge.

## Coding and Testing Expectations

- Follow idiomatic Go and existing project conventions.
- Prefer explicit, readable APIs over clever abstractions.
- Keep tests deterministic and focused.
- Avoid unnecessary dependencies.
- Update docs when commands, APIs, or behavior change.

## Commit Message Guidelines

Use concise, imperative commits. Preferred prefixes:

- `feat: add ...`
- `fix: handle ...`
- `test: cover ...`
- `chore: ...`
- `ci: ...`
- `docs: ...`

## Pull Request Guidelines

- Describe what changed, why, and how it was validated.
- Link related issues (for example, `Closes #42`).
- Include examples or output snippets when behavior changes.
- Call out breaking changes clearly.

## Code Review

Maintainers review PRs as time permits. Please keep discussions technical, objective, and collaborative.

## License

By contributing, you agree that your contributions are licensed under the project [LICENSE](LICENSE).
