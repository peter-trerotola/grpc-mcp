# Contributing to grpc-mcp

Thank you for your interest in contributing! This document covers how to set up, develop, and submit changes.

## Development Setup

1. **Prerequisites:** Go 1.23+, Docker (optional), a gRPC server with reflection enabled for testing

2. **Run tests:**

   ```bash
   go test ./... -race
   ```

3. **Build the binary:**

   ```bash
   CGO_ENABLED=0 go build -o grpc-mcp-server ./cmd/grpc-mcp-server
   ```

   Or via Docker:

   ```bash
   docker build -t grpc-mcp-server .
   ```

## Commit Messages

This project uses [Conventional Commits](https://www.conventionalcommits.org/) and [release-please](https://github.com/googleapis/release-please) for automated releases. Commit messages must follow this format:

```
<type>: <description>

[optional body]
```

Common types:
- `feat:` — new feature (triggers minor version bump)
- `fix:` — bug fix (triggers patch version bump)
- `test:` — adding or updating tests
- `docs:` — documentation changes
- `chore:` — maintenance tasks
- `refactor:` — code restructuring without behavior change

Breaking changes: add `!` after the type (e.g. `feat!: remove legacy endpoint`) or include `BREAKING CHANGE:` in the footer.

## Pull Request Workflow

1. Branch from `master`
2. Make your changes
3. Ensure all tests pass: `go test ./... -race`
4. Ensure code is formatted: `gofmt -w .`
5. Ensure lint passes: `golangci-lint run --timeout=5m`
6. Open a PR against `master`
7. CI must pass before merge

## Code Style

- Format with `gofmt`
- Follow standard Go conventions
- Keep changes focused — one concern per PR
