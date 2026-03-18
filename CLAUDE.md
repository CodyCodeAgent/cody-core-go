# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Build & Test Commands

```bash
make check                 # Run all checks: vet + lint + test
make test                  # Run all tests
make test-race             # Run all tests with race detector
make test-cover            # Run tests with coverage report
make lint                  # Run golangci-lint
make vet                   # Run go vet
make fmt                   # Format code
go test ./agent/...        # Run tests for a single package
go test ./agent/ -run TestName  # Run a specific test
```

All tests use `testutil.TestModel` or `testutil.FunctionModel` — no real API calls or credentials needed. CI runs on every push/PR via GitHub Actions (`.github/workflows/ci.yml`).

## Architecture

This is a **Pydantic AI-style agent framework for Go** built on [cloudwego/eino](https://github.com/cloudwego/eino). It adds generic typed agents, structured output, dependency injection, and test utilities on top of Eino's model/tool/message abstractions.

### Package Overview

- **`agent/`** — Core package. Contains `Agent[D, O]` (the main generic agent), `RunContext[D]`, `Conversation[D, O]`, union types (`OneOf2`, `OneOf3`), options, result types, and retry/error types.
- **`output/`** — Structured output machinery: JSON schema generation from Go struct tags, output tool creation (`final_result`), parsing (struct and primitive wrapper), and output validation.
- **`direct/`** — Lightweight one-shot model calls (`RequestText`, `Request[T]`) without the agent loop, tools, or retries.
- **`deps/`** — Convenience re-exports of `agent.GetDeps`, `agent.GetRunContext`, and `agent.GetMetadata` for use in Eino tool implementations.
- **`testutil/`** — `TestModel` (pre-configured response sequences), `FunctionModel` (custom generate logic), and assertion helpers (`AssertToolCalled`, `AssertToolRegistered`, `AssertSystemPromptContains`, etc.).

### Key Design Patterns

**Generic Agent `Agent[D, O]`**: `D` is the dependency type (injected via `RunContext`), `O` is the output type (auto-validated/deserialized). Use `agent.NoDeps` when no dependencies are needed. The agent runs an iterative loop: call model → execute tool calls → repeat until an output tool is called or text is returned.

**Structured Output via Output Tool**: For non-string `O`, the framework auto-generates a `final_result` tool from `O`'s JSON schema. The model calls this tool to return structured data. Primitive types (int, bool, etc.) are wrapped in `{"result": ...}`.

**Union Output Types**: `OneOf2[A, B]` / `OneOf3[A, B, C]` generate separate output tools per variant (e.g., `final_result_TypeA`, `final_result_TypeB`). Created via `agent.NewOneOf2` / `agent.NewOneOf3` constructors. Use `.Match()` for exhaustive dispatch.

**Dependency Injection**: Dependencies flow through `context.Value` via `RunContext[D]`. Tools access them with `deps.GetDeps[D](ctx)` or `agent.GetDeps[D](ctx)`.

**Struct Tags for Schema**: Output and tool arg structs use `json`, `description`, `enum`, `required`, and `jsonschema` tags to generate JSON Schema for the model.

**Tool Registration**: Either wrap an existing `eino.InvokableTool` with `WithTool`, or create inline tools with `WithToolFunc[D, O, Args]` which auto-generates schema from the `Args` struct and injects `RunContext`.

**Model Settings**: Use the strongly-typed `ModelSettings` struct with pointer fields for optional values: `WithModelSettings(ModelSettings{Temperature: Ptr(float32(0.7)), MaxTokens: Ptr(100)})`.

**Testing Pattern**: Create a `testutil.NewTestModel(responses...)` with pre-configured responses, pass it to `agent.New(...)`, call `agent.Run()`, then assert with `tm.CallCount()`, `tm.LastCall()`, and the `testutil.Assert*` helpers.
