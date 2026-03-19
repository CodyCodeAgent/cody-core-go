# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Build & Test Commands

```bash
make check                 # Run all checks: vet + lint + test (race detector)
make test                  # Run all tests
make test-race             # Run all tests with race detector
make test-cover            # Run tests with coverage report
make lint                  # Run golangci-lint
make vet                   # Run go vet
make fmt                   # Format code (gofmt + goimports)
go test ./agent/...        # Run tests for a single package
go test ./agent/ -run TestName  # Run a specific test
```

All tests use `testutil.TestModel` or `testutil.FunctionModel` — no real API calls or credentials needed. CI runs on every push/PR via GitHub Actions (`.github/workflows/ci.yml`). Requires Go 1.25+.

## Architecture

This is a **Pydantic AI-style agent framework for Go** built on [cloudwego/eino](https://github.com/cloudwego/eino). It adds generic typed agents, structured output, dependency injection, and test utilities on top of Eino's model/tool/message abstractions.

### Package Overview

- **`agent/`** — Core package. `Agent[D, O]`, `RunContext[D]`, `Conversation[D, O]`, union types (`OneOf2`, `OneOf3`), options, result types, and retry/error types.
- **`output/`** — Structured output: JSON schema generation from Go struct tags, output tool creation (`final_result`), parsing (struct and primitive wrapper), validation.
- **`direct/`** — Lightweight one-shot model calls (`RequestText`, `Request[T]`) without the agent loop, tools, or retries.
- **`deps/`** — Convenience re-exports of `agent.GetDeps`, `agent.GetRunContext`, and `agent.GetMetadata` for use in Eino tool implementations.
- **`mcp/`** — MCP (Model Context Protocol) integration. Bridges MCP server tools to Eino `InvokableTool` via JSON Schema round-tripping. Uses `github.com/modelcontextprotocol/go-sdk`.
- **`testutil/`** — `TestModel` (pre-configured response sequences), `FunctionModel` (custom generate logic), and assertion helpers (`AssertToolCalled`, `AssertToolRegistered`, `AssertSystemPromptContains`, etc.).

### Key Source Files

- `agent/agent.go` — Agent loop, message flow, tool execution, output-tool early-exit strategy
- `agent/context.go` — `RunContext`, `UsageTracker`, context.Value injection
- `agent/options.go` — Configuration builders, tool registration, model settings merge
- `agent/union.go` — Union type factories (`NewOneOf2`/`NewOneOf3`), output tool routing
- `agent/retry.go` — `ErrModelRetry`, `ToolRetriesExceededError`, `ResultRetriesExceededError`
- `output/schema.go` — Struct-to-JSON-schema conversion, tag parsing, primitive wrapping
- `output/tool_output.go` — `OutputTool` (stub `InvokableTool` — `InvokableRun()` is never called; the agent intercepts output tool calls before execution)

### Agent Loop Internals

The agent loop in `agent.go` runs up to `maxIterations` (default 20):

1. Check usage limits before each model call
2. Call model with current messages + all tool schemas (rebuilt every iteration via `buildToolInfos()` with optional `PrepareFunc` callbacks)
3. Accumulate usage stats (thread-safe `UsageTracker` with `sync.Mutex`)
4. If model returns text (no tool calls) → return as result
5. **Output tool early-exit**: Check if ANY tool call is an output tool FIRST. If so, execute only that tool (parse + validate), skip remaining tool calls (fill with "not executed" messages). This prevents wasting computation when the model is ready to return output.
6. Otherwise, execute all regular tool calls in parallel, collect results, loop back to step 1

Tool execution has panic recovery via deferred recover in `executeSingleTool()`.

### Three Retry Mechanisms

- **Tool retries** (`maxRetries`, default 1): Per-tool counter tracked in `toolRetries map[string]int`. Tool returns `ErrModelRetry` → feedback message sent to model, counter incremented.
- **Result validation retries** (`maxResultRetries`, default 1): Global counter for output parsing/validation failures. Triggered by JSON parse errors or validator functions.
- **Regular tool errors**: No retry consumption; error is sent as feedback to model.

`ErrModelRetry` supports `errors.As()` for wrapped errors. Feedback is sent as `schema.Tool` role messages.

### Dependency Injection via Context

`RunContext[D]` is injected via `context.WithValue` using a **generic struct type as key** (`runContextKey[D]{}`). The empty generic-typed struct prevents collisions between different `D` types. Tools created with `WithToolFunc` receive `*RunContext[D]` directly; standalone Eino tools use `deps.GetDeps[D](ctx)`.

### Model Settings Resolution

`ModelSettings` uses pointer fields to distinguish "not set" from zero values. Per-run settings override agent-level settings via `mergeModelSettings()` — only non-nil fields from the override are applied. Merged settings are converted to Eino `model.Option` slice at call time.

### Key Design Patterns

**Structured Output via Output Tool**: For non-string `O`, the framework auto-generates a `final_result` tool from `O`'s JSON schema at construction time. The `OutputTool` implements `InvokableTool` but its run method is never invoked — the agent intercepts output tool calls to parse and validate directly. Primitive types (int, bool, etc.) are wrapped in `{"result": ...}`.

**Union Output Types**: `OneOf2[A, B]` / `OneOf3[A, B, C]` override the default output tools after construction, replacing them with variant-specific tools (e.g., `final_result_VulnReport`). Type names are sanitized via regex to remove non-alphanumeric chars. Each union type injects a custom `outputParser` closure that routes by tool name.

**Tool Registration**: Either wrap an existing `eino.InvokableTool` with `WithTool`, or create inline tools with `WithToolFunc[D, O, Args]` which auto-generates schema from the `Args` struct and injects `RunContext`. Tools can have `PrepareFunc` callbacks that modify tool schema per-iteration based on runtime state.

**Conversation**: `Conversation[D, O]` carries message history between turns. NOT thread-safe. History is updated by copying from `Result.AllMessages()` after each turn. `SendStream().Final()` auto-updates history via callback.

**Testing Pattern**: Create a `testutil.NewTestModel(responses...)` with pre-configured response sequences, pass it to `agent.New(...)`, call `agent.Run()`, then assert with `tm.CallCount()`, `tm.LastCall()`, and the `testutil.Assert*` helpers. `TestModel` records all calls (messages + tools) in a thread-safe list; response sequence is consumed in order and errors on overflow.

### Struct Tags for Schema

Output and tool arg structs use these tags for JSON Schema generation (processed in `output/schema.go`):

- `json:"name"` — field name; `omitempty` makes the field optional
- `description:"text"` — field description
- `enum:"a,b,c"` — enum constraint
- `required:"true"` / `required:"false"` — explicit override (takes precedence over `omitempty`)
- `jsonschema:"description=text,required"` — composite tag for multiple schema directives

Embedded structs are recursively flattened into the parent schema.
