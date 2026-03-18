# cody-core-go

[![CI](https://github.com/codycode/cody-core-go/actions/workflows/ci.yml/badge.svg)](https://github.com/codycode/cody-core-go/actions/workflows/ci.yml)
[![Go Reference](https://pkg.go.dev/badge/github.com/codycode/cody-core-go.svg)](https://pkg.go.dev/github.com/codycode/cody-core-go)
[![License](https://img.shields.io/badge/License-Apache_2.0-blue.svg)](LICENSE)

Pydantic AI-style agent framework for Go, built on top of [cloudwego/eino](https://github.com/cloudwego/eino).

## Features

- **Generic `Agent[D, O]`** — Type-safe agents with compile-time checked dependencies and output types
- **Structured Output** — Automatic JSON Schema generation, output tool creation, and deserialization for structs, primitives, slices, and union types
- **Dependency Injection** — `RunContext[D]` provides typed dependencies to tools and system prompts
- **Output Validation + Retry** — `OutputValidator` with `ErrModelRetry` for automatic self-correction
- **TestModel / FunctionModel** — Mock models implementing Eino's `BaseChatModel` interface for unit testing
- **Direct Requests** — `direct.RequestText` / `direct.Request[T]` for simple one-shot model calls
- **Multi-turn Conversation** — `Conversation[D, O]` for automatic message history management
- **Union Output Types** — `OneOf2[A, B]` / `OneOf3[A, B, C]` with `Match` for exhaustive pattern matching

## Quick Start

```go
package main

import (
    "context"
    "fmt"
    "log"

    "github.com/codycode/cody-core-go/agent"
    einoopenai "github.com/cloudwego/eino-ext/components/model/openai"
)

type MovieReview struct {
    Title     string  `json:"title" description:"Movie title"`
    Rating    float64 `json:"rating" description:"Rating 1-10"`
    Sentiment string  `json:"sentiment" description:"Sentiment" enum:"positive,negative,neutral"`
}

func main() {
    ctx := context.Background()
    chatModel, _ := einoopenai.NewChatModel(ctx, &einoopenai.ChatModelConfig{
        Model: "gpt-4o",
    })

    a := agent.New[agent.NoDeps, MovieReview](chatModel,
        agent.WithSystemPrompt[agent.NoDeps, MovieReview]("You are a movie critic."),
    )

    result, err := a.Run(ctx, "Review Interstellar", agent.NoDeps{})
    if err != nil {
        log.Fatal(err)
    }
    fmt.Printf("%s: %.1f (%s)\n", result.Output.Title, result.Output.Rating, result.Output.Sentiment)
}
```

## Package Structure

```
cody-core-go/
├── agent/       # Core Agent[D,O], RunContext, Result, Conversation, Union types
├── output/      # Structured output: schema generation, output tool, validator
├── direct/      # Lightweight direct model requests (no agent loop)
├── deps/        # Dependency injection helpers (re-exports from agent)
├── testutil/    # TestModel, FunctionModel for unit testing
└── docs/        # Design documents
```

## Examples

See the [`examples/`](examples/) directory for runnable examples:

- **[quickstart](examples/quickstart/)** — Structured output agent with `MovieReview`

```bash
go run ./examples/quickstart/
```

## Testing

```bash
go test ./...              # Run all tests
go test -race ./...        # Run tests with race detector
make check                 # Run vet + lint + test
```

All tests use `TestModel` or `FunctionModel` — no real API calls needed.

## Architecture

Built on Eino's foundation (ChatModel, Tool, Message, StreamReader), this project adds:

| Layer | What It Provides |
|-------|-----------------|
| **Eino** | Model abstraction, tool system, streaming, MCP, callbacks |
| **cody-core-go** | Generic Agent[D,O], structured output + validation, dependency injection, TestModel |

See [docs/design.md](docs/design.md) for the full design document.

## Contributing

See [CONTRIBUTING.md](CONTRIBUTING.md) for development setup and guidelines.

## License

Apache License 2.0 — see [LICENSE](LICENSE) for details.
