# Contributing to cody-core-go

Thank you for your interest in contributing! This guide will help you get started.

## Development Setup

```bash
# Clone the repository
git clone https://github.com/codycode/cody-core-go.git
cd cody-core-go

# Verify everything works
make test
```

**Requirements**: Go 1.24+

## Making Changes

1. Fork the repository and create a feature branch from `main`
2. Make your changes
3. Run checks before submitting:

```bash
make check    # Runs: go vet, golangci-lint, go test -race
```

Or individually:

```bash
make test       # Run tests
make test-race  # Run tests with race detector
make lint       # Run linter
make vet        # Run go vet
make fmt        # Format code
```

## Code Guidelines

- Follow standard Go conventions ([Effective Go](https://go.dev/doc/effective_go), [Go Code Review Comments](https://go.dev/wiki/CodeReviewComments))
- All exported types, functions, and methods must have doc comments
- Use struct tags (`json`, `description`, `enum`, `required`) for schema generation
- All tests use `testutil.TestModel` or `testutil.FunctionModel` -- no real API calls or credentials needed

## Testing

```bash
go test ./...              # Run all tests
go test -race ./...        # Run all tests with race detector
go test ./agent/...        # Run tests for a single package
go test ./agent/ -run TestName  # Run a specific test
```

When writing tests:

- Use `testutil.NewTestModel(responses...)` with pre-configured response sequences
- Use `testutil.NewFunctionModel(fn)` for custom generate logic
- Use the `testutil.Assert*` helpers for common assertions
- Never make real API calls in tests

## Pull Request Process

1. Update documentation if you change public APIs
2. Add tests for new functionality
3. Ensure `make check` passes
4. Keep commits focused -- one logical change per commit
5. Write clear commit messages:
   - `feat: add support for streaming tools`
   - `fix: handle nil pointer in output parser`
   - `refactor: simplify model settings merge logic`
   - `docs: update quickstart example`
   - `test: add coverage for union type edge cases`

## Package Overview

| Package | Purpose |
|---------|---------|
| `agent/` | Core `Agent[D,O]`, `RunContext`, `Conversation`, union types |
| `output/` | Structured output: schema generation, output tool, validator |
| `direct/` | Lightweight one-shot model calls without agent loop |
| `deps/` | Dependency injection helpers (re-exports from `agent`) |
| `testutil/` | `TestModel`, `FunctionModel`, assertion helpers |
| `examples/` | Runnable examples (quickstart, union-types, validator, conversation) |

## Reporting Issues

- Use GitHub Issues with the provided templates
- Include Go version, OS, and a minimal reproduction case
- For security issues, please email security@codycode.dev instead of opening a public issue

## License

By contributing, you agree that your contributions will be licensed under the [Apache License 2.0](LICENSE).
