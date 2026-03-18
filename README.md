# cody-core-go

[![CI](https://github.com/CodyCodeAgent/cody-core-go/actions/workflows/ci.yml/badge.svg)](https://github.com/CodyCodeAgent/cody-core-go/actions/workflows/ci.yml)
[![Go Reference](https://pkg.go.dev/badge/github.com/CodyCodeAgent/cody-core-go.svg)](https://pkg.go.dev/github.com/CodyCodeAgent/cody-core-go)
[![License](https://img.shields.io/badge/License-Apache_2.0-blue.svg)](LICENSE)

A Pydantic AI-style agent framework for Go, built on [cloudwego/eino](https://github.com/cloudwego/eino). Type-safe agents with structured output, dependency injection, and automatic validation retries.

## Install

```bash
go get github.com/CodyCodeAgent/cody-core-go
```

Requires Go 1.24+.

## Quick Start

```go
package main

import (
    "context"
    "fmt"
    "log"

    "github.com/CodyCodeAgent/cody-core-go/agent"
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

For runnable examples that work without API keys (using `testutil.TestModel`), see the [`examples/`](examples/) directory.

## Core Concepts

### Agent[D, O]

`Agent[D, O]` is the central type. `D` is the dependency type (injected into tools and system prompts), `O` is the output type (auto-validated and deserialized). Use `agent.NoDeps` when no dependencies are needed.

```go
// String output agent (returns plain text)
a := agent.New[agent.NoDeps, string](chatModel)

// Structured output agent (returns typed struct)
a := agent.New[agent.NoDeps, MyStruct](chatModel)

// Agent with dependencies
a := agent.New[MyDeps, MyStruct](chatModel)
```

The agent runs an iterative loop: call model -> execute tool calls -> repeat until the model returns a final result or plain text.

### Structured Output

For non-string output types, the framework auto-generates a `final_result` tool from the struct's JSON schema. The model calls this tool to return structured data.

```go
type WeatherReport struct {
    City        string  `json:"city" description:"City name"`
    Temperature float64 `json:"temperature" description:"Temperature in Celsius"`
    Condition   string  `json:"condition" description:"Weather condition" enum:"sunny,cloudy,rainy,snowy"`
}

a := agent.New[agent.NoDeps, WeatherReport](chatModel)
result, _ := a.Run(ctx, "Weather in Tokyo?", agent.NoDeps{})
fmt.Println(result.Output.City) // "Tokyo"
```

**Supported output types:**

| Type | How it works |
|------|-------------|
| `string` | Model returns plain text, no output tool |
| Struct | Auto-generated `final_result` tool with JSON Schema |
| `int`, `bool`, `float64`, etc. | Wrapped in `{"result": ...}` |
| `[]string`, `[]MyStruct` | Wrapped in `{"result": [...]}` |
| `OneOf2[A, B]`, `OneOf3[A, B, C]` | Separate output tool per variant |

**Struct tags for schema generation:**

```go
type Args struct {
    Query    string   `json:"query" description:"Search query"`
    Category string   `json:"category" enum:"news,tech,science"`
    Limit    int      `json:"limit,omitempty"`                        // omitempty = optional
    Tags     []string `json:"tags" required:"true"`                   // explicit required
    Score    float64  `json:"score" jsonschema:"description=User score,required"`
}
```

### Tools

Register tools that the model can call during the agent loop.

**Inline tools** (with typed args and dependency injection):

```go
type SearchArgs struct {
    Query string `json:"query" description:"Search query"`
}

a := agent.New[MyDeps, string](chatModel,
    agent.WithToolFunc[MyDeps, string, SearchArgs](
        "search", "Search the web",
        func(rc *agent.RunContext[MyDeps], args SearchArgs) (string, error) {
            // Access dependencies
            results, err := rc.Deps.SearchClient.Search(args.Query)
            if err != nil {
                return "", err
            }
            return results, nil
        },
    ),
)
```

**Existing Eino tools** (wrap any `tool.InvokableTool`):

```go
a := agent.New[agent.NoDeps, string](chatModel,
    agent.WithTool[agent.NoDeps, string](myEinoTool),
)
```

**Dynamic tool schema** (modify tool parameters per-run based on dependencies):

```go
agent.WithToolFunc[MyDeps, string, SearchArgs](
    "search", "Search",
    searchFn,
    agent.WithPrepare[MyDeps](func(rc *agent.RunContext[MyDeps], info *schema.ToolInfo) (*schema.ToolInfo, error) {
        if rc.Deps.IsAdmin {
            info.Desc = "Admin search with full access"
        }
        return info, nil
    }),
)
```

### Dependency Injection

Dependencies flow through `RunContext[D]` via `context.Value`. Tools created with `WithToolFunc` receive `*RunContext[D]` directly. For standalone Eino tools, use the `deps` package:

```go
import "github.com/CodyCodeAgent/cody-core-go/deps"

func myEinoTool(ctx context.Context, args string) (string, error) {
    d, ok := deps.GetDeps[MyDeps](ctx)
    if !ok {
        return "", errors.New("deps not found")
    }
    // use d.DB, d.APIKey, etc.
}
```

Run-level metadata is also available:

```go
result, _ := a.Run(ctx, "query", myDeps,
    agent.WithRunMetadata(map[string]any{"trace_id": "abc123"}),
)

// In a tool:
meta := deps.GetMetadata[MyDeps](ctx) // map[string]any{"trace_id": "abc123"}
```

### Output Validation

Validators inspect and optionally transform the model's output. Return `ErrModelRetry` to ask the model to try again with feedback.

```go
a := agent.New[agent.NoDeps, WeatherReport](chatModel,
    agent.WithOutputValidator[agent.NoDeps, WeatherReport](
        func(_ context.Context, w WeatherReport) (WeatherReport, error) {
            if w.Temperature < -100 || w.Temperature > 60 {
                return w, agent.NewModelRetry(
                    fmt.Sprintf("temperature %f is unrealistic", w.Temperature),
                )
            }
            return w, nil
        },
    ),
    agent.WithMaxResultRetries[agent.NoDeps, WeatherReport](3),
)
```

Tools can also trigger retries:

```go
func myTool(rc *agent.RunContext[NoDeps], args Args) (string, error) {
    if args.Query == "" {
        return "", agent.NewModelRetry("query must not be empty")
    }
    // ...
}
```

### Union Output Types

When the model should choose between different output structures, use `OneOf2` or `OneOf3`:

```go
type VulnReport struct {
    Type     string `json:"type"`
    Severity string `json:"severity" enum:"low,medium,high,critical"`
}

type SafeReport struct {
    Summary string `json:"summary"`
}

a := agent.NewOneOf2[agent.NoDeps, VulnReport, SafeReport](chatModel)
result, _ := a.Run(ctx, "Scan this code", agent.NoDeps{})

// Exhaustive pattern matching
result.Output.Match(
    func(v VulnReport) { fmt.Printf("VULN: %s (%s)\n", v.Type, v.Severity) },
    func(s SafeReport) { fmt.Printf("SAFE: %s\n", s.Summary) },
)
```

Each variant gets its own output tool (`final_result_VulnReport`, `final_result_SafeReport`).

### Multi-turn Conversation

`Conversation` automatically carries message history between turns:

```go
conv := agent.NewConversation(a)

r1, _ := conv.Send(ctx, "My name is Alice.", deps)
r2, _ := conv.Send(ctx, "What's my name?", deps)  // history carried forward

conv.Messages()      // full history
conv.Len()           // number of messages
conv.SetMessages(saved) // restore a saved conversation state
conv.Reset()         // clear history
```

### Streaming (Experimental)

`RunStream` provides a streaming interface. The current implementation wraps `Run()` — true token-by-token streaming is planned.

```go
sr, _ := a.RunStream(ctx, "Tell me a story", agent.NoDeps{})

for chunk := range sr.TextStream() {
    fmt.Print(chunk)
}

result, _ := sr.Final()  // get the full Result[O]
```

With conversations, `Final()` auto-updates history:

```go
sr, _ := conv.SendStream(ctx, "Hello", deps)
result, _ := sr.Final() // conversation history is automatically updated
```

### Direct Requests

For simple one-shot calls without the agent loop, tools, or retries:

```go
import "github.com/CodyCodeAgent/cody-core-go/direct"

// Plain text
text, _ := direct.RequestText(ctx, chatModel, "Translate: hello",
    direct.WithSystemPrompt("You are a translator."),
)

// Structured output
result, _ := direct.Request[MyStruct](ctx, chatModel, "Analyze this text",
    direct.WithModelSettings(agent.ModelSettings{
        Temperature: agent.Ptr(float32(0.3)),
    }),
)
```

### Configuration

**Agent-level settings** (apply to all runs):

```go
a := agent.New[agent.NoDeps, string](chatModel,
    agent.WithSystemPrompt[agent.NoDeps, string]("System prompt."),
    agent.WithDynamicSystemPrompt[agent.NoDeps, string](func(rc *agent.RunContext[agent.NoDeps]) (string, error) {
        return "Dynamic prompt based on deps", nil
    }),
    agent.WithModelSettings[agent.NoDeps, string](agent.ModelSettings{
        Temperature: agent.Ptr(float32(0.7)),
        MaxTokens:   agent.Ptr(1000),
        TopP:        agent.Ptr(float32(0.9)),
        Stop:        []string{"\n\n"},
    }),
    agent.WithMaxRetries[agent.NoDeps, string](3),        // per-tool retry limit
    agent.WithMaxResultRetries[agent.NoDeps, string](2),   // output validation retry limit
    agent.WithMaxIterations[agent.NoDeps, string](10),     // agent loop iteration limit (default: 20)
)
```

**Per-run overrides:**

```go
result, _ := a.Run(ctx, "prompt", deps,
    agent.WithHistory(previousMessages),
    agent.WithRunModelSettings(agent.ModelSettings{Temperature: agent.Ptr(float32(0.1))}),
    agent.WithUsageLimits(agent.UsageLimits{RequestLimit: 5, TotalTokensLimit: 10000}),
    agent.WithRunMetadata(map[string]any{"user_id": "u123"}),
)
```

**Result inspection:**

```go
result.Output           // typed output (O)
result.Usage.Requests   // number of model calls
result.Usage.TotalTokens
result.AllMessages()    // full message sequence (excludes system messages)
result.NewMessages()    // only messages from this run
```

## Testing

All tests use mock models — no API keys needed.

```bash
make check                 # vet + lint + test (race detector)
make test                  # go test ./...
go test ./agent/ -run TestName  # single test
```

**TestModel** — pre-configured response sequences:

```go
tm := testutil.NewTestModel(
    testutil.TestResponse{Text: "Hello!"},
    testutil.TestResponse{
        ToolCalls: []schema.ToolCall{{
            ID: "call_1", Type: "function",
            Function: schema.FunctionCall{Name: "final_result", Arguments: `{"city":"Tokyo"}`},
        }},
    },
)

a := agent.New[agent.NoDeps, MyStruct](tm)
result, _ := a.Run(ctx, "test", agent.NoDeps{})
```

**FunctionModel** — custom generate logic:

```go
fm := testutil.NewFunctionModel(func(msgs []*schema.Message, tools []*schema.ToolInfo) (*schema.Message, error) {
    return &schema.Message{Role: schema.Assistant, Content: "dynamic response"}, nil
})
```

**Assertion helpers:**

```go
testutil.AssertToolCalled(t, tm, "my_tool")
testutil.AssertToolRegistered(t, tm, "my_tool")
testutil.AssertSystemPromptContains(t, tm, "helpful")
testutil.AssertNoSystemPrompt(t, tm)
testutil.AssertUserPromptSent(t, tm, "Hello")
```

**Inspect calls:**

```go
tm.CallCount()                   // number of Generate calls
tm.LastCall().Messages           // messages from last call
tm.LastCall().Tools              // tools from last call
tm.AllCalls()                    // all recorded calls
```

## Package Structure

```
cody-core-go/
├── agent/       Core Agent[D,O], RunContext, Result, Conversation, union types, options
├── output/      JSON Schema generation, output tool, validator, parsing
├── direct/      One-shot model requests (no agent loop)
├── deps/        Convenience re-exports of GetDeps, GetRunContext, GetMetadata
├── testutil/    TestModel, FunctionModel, assertion helpers
└── examples/    Runnable examples (quickstart, union-types, validator, conversation)
```

## Contributing

See [CONTRIBUTING.md](CONTRIBUTING.md).

## License

Apache License 2.0 — see [LICENSE](LICENSE).
