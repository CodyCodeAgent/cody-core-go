# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added
- Core `Agent[D, O]` with generic typed dependencies and structured output
- `RunContext[D]` dependency injection via `context.Value`
- Structured output via `final_result` output tool with automatic JSON Schema generation
- Output validation with `ErrModelRetry` for automatic self-correction
- Union output types: `OneOf2[A, B]` / `OneOf3[A, B, C]` with exhaustive `Match`
- Multi-turn `Conversation[D, O]` with automatic history management
- `direct.RequestText` / `direct.Request[T]` for one-shot model calls
- `testutil.TestModel` and `testutil.FunctionModel` for unit testing
- `testutil.Assert*` helpers for common test assertions
- Strongly-typed `ModelSettings` struct for model parameters
- Thread-safe `UsageTracker` with mutex protection
- GitHub Actions CI with race detector and coverage
- `Makefile` with common development commands
- `golangci-lint` configuration
- `WithMaxIterations` option to configure agent loop iteration limit
- `GetMetadata` helper for accessing run-level metadata in tools
- Streaming tests covering `RunStream`, `TextStream`, `Final`
- Tests for `WithTool`, `RunWithHistory`, `WithMaxIterations`, assertion helpers
- Advanced examples: union-types, validator, conversation
- CI checks: `go mod tidy` verification, `go build ./examples/...`

### Fixed
- `StreamResult.done` channel was created twice (in `RunStream` and `TextStream`), causing a dead channel
- `NewMessages()` now returns empty slice instead of nil when there are no new messages
- `Conversation.SendStream` now auto-updates history when `Final()` is called
- Removed dead `RunContext.Retry` field (was never populated)

### Changed
- `RunStream` / `StreamResult` marked as EXPERIMENTAL in documentation
- `direct` package now uses `agent.ModelSettings` instead of a duplicate local type
- Renamed `output/output.go` to `output/doc.go` (Go package doc convention)
- Removed internal design documents (`docs/`) — README is now the primary SDK guide

### Dependencies
- Built on [cloudwego/eino](https://github.com/cloudwego/eino) v0.8.3
- Go 1.24+
