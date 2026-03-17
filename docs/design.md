# Cody Core Go — Pydantic AI Go 实现设计文档

> 版本：v0.1 Draft
> 日期：2026-03-17
> 模块名：`github.com/codycode/cody-core-go`（暂定）

---

## 一、项目目标

### 1.1 愿景

在 Go 生态中提供一个**类 Pydantic AI 的 Agent 框架**，将 Pydantic AI 的核心理念——**类型安全、声明式、可测试**——映射到 Go 的类型系统和并发模型中。

### 1.2 核心设计原则

| 原则 | 说明 |
|------|------|
| **Go-idiomatic** | 不是 Python 代码的直译，而是用 Go 的泛型、接口、context 等原语重新设计 |
| **类型安全** | 利用 Go 1.18+ 泛型，Agent 的输入、输出、依赖全部类型化，编译期检查 |
| **结构化输出优先** | 输出自动反序列化 + 校验，是框架的一等能力 |
| **简单原语** | 不做重量级编排，提供少量强力原语让用户自由组合 |
| **可测试** | 内置 TestModel，依赖注入，方便单元测试 |
| **可观测** | 原生 OpenTelemetry 支持 |

### 1.3 不做什么（明确边界）

- **不做** Pydantic 验证库本身的移植（Go 有 `go-playground/validator`、`ozzo-validation` 等）
- **不做** 完整的 DAG/Workflow 引擎（初期只做线性 Agent loop + 简单委托）
- **不做** UI 框架（AG-UI / Vercel AI SDK 对接留给上层）
- **不做** 持久化执行引擎（Temporal 等集成留后期）

---

## 二、Go 语言适配分析

Pydantic AI 大量依赖 Python 特性，以下是到 Go 的映射方案：

| Python 特性 | Go 对应方案 |
|------------|-----------|
| 泛型类 `Agent[Deps, Output]` | Go 泛型 `Agent[D, O any]` |
| Pydantic BaseModel 验证 | struct tag + validator 接口 + JSON Schema 生成 |
| 装饰器 `@agent.tool` | 函数注册（方法调用 / 选项模式） |
| async/await | goroutine + channel / context |
| Union 类型 `A \| B` | 接口 + 类型断言 / `OneOf[A, B]` 封装 |
| dataclass | 普通 struct |
| docstring → schema | struct tag + 注释（或显式 Description 字段） |
| `ModelRetry` 异常 | 特定 error 类型 `ErrModelRetry` |
| 依赖注入 `RunContext[Deps]` | `RunContext[D]` 泛型 struct，内含 `context.Context` |
| 流式迭代 `async for` | `chan` / `iter.Seq` (Go 1.23+) / callback |
| 消息历史 | `[]Message` slice |

---

## 三、包结构设计

```
cody-core-go/
├── go.mod
├── go.sum
├── README.md
├── docs/
│   └── design.md                    # 本文档
│
├── agent/                           # 核心 Agent 框架
│   ├── agent.go                     # Agent[D, O] 泛型定义 + 运行逻辑
│   ├── agent_test.go
│   ├── context.go                   # RunContext[D] 定义
│   ├── options.go                   # AgentOption 选项模式
│   ├── result.go                    # AgentRunResult / StreamResult
│   └── retry.go                     # ModelRetry / 重试逻辑
│
├── model/                           # 模型抽象层
│   ├── model.go                     # Model 接口定义
│   ├── message.go                   # Message 类型体系
│   ├── settings.go                  # ModelSettings
│   ├── usage.go                     # Usage / UsageLimits
│   └── providers/                   # 各 Provider 实现
│       ├── openai/
│       │   ├── openai.go
│       │   └── openai_test.go
│       ├── anthropic/
│       │   ├── anthropic.go
│       │   └── anthropic_test.go
│       └── ollama/
│           └── ollama.go
│
├── tool/                            # 工具系统
│   ├── tool.go                      # Tool 接口 + 注册
│   ├── schema.go                    # JSON Schema 生成
│   ├── toolset.go                   # Toolset 工具集
│   └── deferred.go                  # Deferred Tool（Human-in-the-Loop）
│
├── output/                          # 结构化输出
│   ├── output.go                    # OutputType 接口 + 验证
│   ├── validator.go                 # OutputValidator
│   └── schema.go                    # 输出 JSON Schema 生成
│
├── mcp/                             # MCP 协议支持
│   ├── client.go                    # MCP Client
│   ├── server.go                    # MCP Server
│   └── transport.go                 # stdio / SSE 传输层
│
├── embedding/                       # 向量嵌入
│   ├── embedding.go                 # EmbeddingModel 接口
│   └── providers/
│       └── openai/
│           └── openai.go
│
├── testing/                         # 测试工具
│   ├── testmodel.go                 # TestModel 实现
│   └── funcmodel.go                 # FunctionModel 实现
│
├── otel/                            # 可观测性
│   ├── trace.go                     # OpenTelemetry trace 集成
│   └── middleware.go                # Agent 中间件
│
└── examples/                        # 示例代码
    ├── basic/
    ├── structured_output/
    ├── multi_agent/
    └── streaming/
```

---

## 四、核心 API 设计

### 4.1 Agent 核心

```go
package agent

// Agent 是框架的核心抽象，D = 依赖类型，O = 输出类型
type Agent[D any, O any] struct {
    model          model.Model
    systemPrompts  []SystemPromptFunc[D]
    tools          []tool.Tool[D]
    outputValidator OutputValidatorFunc[D, O]
    options        AgentOptions
}

// 创建 Agent
func New[D any, O any](modelName string, opts ...Option[D, O]) *Agent[D, O]

// 同步运行
func (a *Agent[D, O]) Run(ctx context.Context, prompt string, deps D, opts ...RunOption) (*Result[O], error)

// 流式运行
func (a *Agent[D, O]) RunStream(ctx context.Context, prompt string, deps D, opts ...RunOption) (*StreamResult[O], error)

// 传入消息历史（多轮对话）
func (a *Agent[D, O]) RunWithHistory(ctx context.Context, prompt string, deps D, history []model.Message, opts ...RunOption) (*Result[O], error)
```

#### RunContext

```go
// RunContext 携带运行时依赖和上下文，传递给 system prompt 函数和 tool 函数
type RunContext[D any] struct {
    Ctx      context.Context
    Deps     D
    Model    model.Model
    Usage    *model.Usage
    Metadata map[string]any
}
```

#### Result

```go
// Result 包含 agent 运行结果
type Result[O any] struct {
    Output      O                // 类型化的输出
    Messages    []model.Message  // 完整消息历史
    Usage       model.Usage      // token 用量统计
}

// StreamResult 提供流式访问
type StreamResult[O any] struct {
    // 文本流
    TextStream() <-chan string
    // 结构化输出流（partial）
    OutputStream() <-chan O
    // 获取最终结果（阻塞直到完成）
    Final() (*Result[O], error)
}
```

#### Option 模式

```go
// Agent 构建选项
func WithSystemPrompt[D, O any](prompt string) Option[D, O]
func WithDynamicSystemPrompt[D, O any](fn SystemPromptFunc[D]) Option[D, O]
func WithTool[D, O any](t tool.Tool[D]) Option[D, O]
func WithOutputValidator[D, O any](fn OutputValidatorFunc[D, O]) Option[D, O]
func WithMaxRetries[D, O any](n int) Option[D, O]
func WithModelSettings[D, O any](s model.Settings) Option[D, O]

// 运行选项
func WithUsageLimits(limits model.UsageLimits) RunOption
func WithRunMetadata(meta map[string]any) RunOption
func WithMessageHistory(history []model.Message) RunOption
```

### 4.2 Model 抽象

```go
package model

// Model 是所有模型 Provider 的统一接口
type Model interface {
    // 发送请求，返回完整响应
    Request(ctx context.Context, messages []Message, settings Settings, tools []ToolDefinition) (*Response, error)

    // 流式请求
    RequestStream(ctx context.Context, messages []Message, settings Settings, tools []ToolDefinition) (*StreamResponse, error)

    // 模型名称
    Name() string
}

// 通过字符串解析创建模型实例（如 "openai:gpt-4o"、"anthropic:claude-sonnet-4-20250514"）
func FromString(modelStr string) (Model, error)
```

#### Message 类型体系

```go
// MessageRole 消息角色
type MessageRole string

const (
    RoleSystem    MessageRole = "system"
    RoleUser      MessageRole = "user"
    RoleAssistant MessageRole = "assistant"
    RoleTool      MessageRole = "tool"
)

// Message 统一消息结构
type Message struct {
    Role    MessageRole
    Parts   []Part        // 一条消息可包含多个 Part
}

// Part 是消息内容的基本单元（接口，多种实现）
type Part interface {
    partMarker()
}

// 各种 Part 类型
type TextPart struct { Text string }
type ToolCallPart struct { ID string; Name string; Args json.RawMessage }
type ToolReturnPart struct { ID string; Name string; Content string }
type ThinkingPart struct { Content string }
type ImagePart struct { URL string; Data []byte; MediaType string }
type AudioPart struct { Data []byte; MediaType string }
type DocumentPart struct { Data []byte; MediaType string }
```

### 4.3 Tool 系统

```go
package tool

// Tool 接口
type Tool[D any] interface {
    Name() string
    Description() string
    Schema() JSONSchema          // 参数的 JSON Schema
    Run(ctx *agent.RunContext[D], args json.RawMessage) (any, error)
}

// 快捷创建工具：从函数自动推断 schema
// 利用泛型 + reflect 自动从 struct tag 生成 JSON Schema
func NewTool[D any, Args any](
    name string,
    description string,
    fn func(ctx *agent.RunContext[D], args Args) (string, error),
) Tool[D]

// 不需要依赖的工具
func NewPlainTool[Args any](
    name string,
    description string,
    fn func(args Args) (string, error),
) Tool[any]
```

#### 使用示例

```go
// 定义工具参数结构体
type SearchArgs struct {
    Query   string `json:"query" description:"搜索关键词" required:"true"`
    MaxResults int `json:"max_results" description:"最大结果数" default:"10"`
}

// 创建工具
searchTool := tool.NewTool[MyDeps, SearchArgs](
    "search_web",
    "Search the web for information",
    func(ctx *agent.RunContext[MyDeps], args SearchArgs) (string, error) {
        return ctx.Deps.SearchClient.Search(ctx.Ctx, args.Query, args.MaxResults)
    },
)
```

#### Toolset

```go
// Toolset 将多个工具组合在一起
type Toolset[D any] struct {
    tools []Tool[D]
}

func NewToolset[D any](tools ...Tool[D]) *Toolset[D]

// DynamicToolset 根据上下文动态决定暴露哪些工具
type DynamicToolset[D any] interface {
    Tools(ctx *agent.RunContext[D]) []Tool[D]
}
```

### 4.4 结构化输出

```go
package output

// OutputConfig 描述如何处理 Agent 输出
type OutputConfig[O any] struct {
    Mode       OutputMode       // Tool / Native / Prompted
    Validators []ValidatorFunc[O]
}

type OutputMode int

const (
    OutputModeTool    OutputMode = iota // 通过 function calling 返回结构化数据（默认）
    OutputModeNative                     // 使用模型原生 structured output API
    OutputModePrompted                   // 通过 prompt 约束输出格式
)

// ValidatorFunc 输出验证函数
// 返回 ErrModelRetry 时触发模型重试
type ValidatorFunc[O any] func(ctx context.Context, output O) (O, error)

// 自动从 Go struct 生成 JSON Schema
func SchemaFor[T any]() JSONSchema
```

### 4.5 ModelRetry 机制

```go
package agent

// ErrModelRetry 是一个特殊 error，触发模型重试
// 会把 Message 作为额外上下文反馈给模型
type ErrModelRetry struct {
    Message string
}

func (e *ErrModelRetry) Error() string { return e.Message }

// 在 Tool 或 OutputValidator 中使用
func myValidator(ctx context.Context, output MyOutput) (MyOutput, error) {
    if output.Score < 0 {
        return output, &ErrModelRetry{Message: "score must be non-negative, please correct"}
    }
    return output, nil
}
```

### 4.6 依赖注入 & 测试

```go
package testing

// TestModel 用于测试，不调用真实 API
type TestModel struct {
    ResultText    string
    ResultToolCalls []model.ToolCallPart
    CustomHandler func(messages []model.Message) (*model.Response, error)
}

func NewTestModel(opts ...TestModelOption) *TestModel

// FunctionModel 用自定义函数模拟模型行为
type FunctionModel struct {
    Handler func(messages []model.Message, tools []model.ToolDefinition) (*model.Response, error)
}

// Agent Override（用于测试）
func (a *Agent[D, O]) WithModel(m model.Model) *Agent[D, O]
```

### 4.7 MCP 支持

```go
package mcp

// MCPClient 连接 MCP Server，获取工具列表
type MCPClient interface {
    ListTools(ctx context.Context) ([]tool.Tool[any], error)
    CallTool(ctx context.Context, name string, args json.RawMessage) (string, error)
    Close() error
}

// 通过 stdio 连接
func NewStdioClient(command string, args ...string) (MCPClient, error)

// 通过 SSE 连接
func NewSSEClient(url string) (MCPClient, error)

// MCPServer 将 Agent 的工具暴露为 MCP Server
type MCPServer struct {
    agent interface{} // 任意 Agent
}

func NewMCPServer(agent interface{}) *MCPServer
func (s *MCPServer) ServeStdio() error
func (s *MCPServer) ServeSSE(addr string) error
```

### 4.8 多模态输入

```go
// UserPrompt 支持多模态输入
type UserPrompt interface{}

// 纯文本
// agent.Run(ctx, "Hello", deps)

// 多模态（使用 Parts slice）
type MultimodalPrompt []Part

// 使用示例
prompt := model.MultimodalPrompt{
    model.TextPart{Text: "Describe this image:"},
    model.ImagePart{URL: "https://example.com/image.jpg"},
    model.DocumentPart{Data: pdfBytes, MediaType: "application/pdf"},
}
result, err := myAgent.Run(ctx, prompt, deps)
```

### 4.9 Embedding

```go
package embedding

type EmbeddingModel interface {
    Embed(ctx context.Context, text string) ([]float64, error)
    EmbedMany(ctx context.Context, texts []string) ([][]float64, error)
    Dimensions() int
}
```

### 4.10 可观测性（OpenTelemetry）

```go
package otel

// Middleware 自动为 Agent 添加 OTel trace
func Middleware[D, O any]() agent.Middleware[D, O]

// 自动 trace 的内容：
// - Agent.Run 整体 span
// - 每轮 model request span
// - 每次 tool call span
// - token usage 记录为 span attributes
```

---

## 五、Agent 运行循环（核心流程）

```
┌─────────────────────────────────────────────────────────┐
│                     Agent.Run()                         │
├─────────────────────────────────────────────────────────┤
│                                                         │
│  1. 构建 system prompt（静态 + 动态）                     │
│  2. 构建初始 messages                                    │
│     ├─ system prompt messages                           │
│     ├─ message history（如有）                           │
│     └─ user prompt message                              │
│                                                         │
│  ┌─── Agent Loop ──────────────────────────────────┐    │
│  │                                                  │    │
│  │  3. 调用 model.Request(messages, tools)          │    │
│  │     ├─ 检查 UsageLimits                          │    │
│  │     └─ 发送请求到 LLM                             │    │
│  │                                                  │    │
│  │  4. 解析模型响应                                   │    │
│  │     ├─ 纯文本 → 跳到步骤 6                        │    │
│  │     └─ ToolCall → 步骤 5                         │    │
│  │                                                  │    │
│  │  5. 执行 Tool Calls                              │    │
│  │     ├─ 查找并执行对应 tool                         │    │
│  │     ├─ 收集 tool results                         │    │
│  │     ├─ 如果 tool 返回 ErrModelRetry → 反馈给模型   │    │
│  │     ├─ 追加 tool call + tool result 到 messages   │    │
│  │     └─ 回到步骤 3（继续循环）                      │    │
│  │                                                  │    │
│  └──────────────────────────────────────────────────┘    │
│                                                         │
│  6. 解析最终输出                                         │
│     ├─ 反序列化为 OutputType O                          │
│     ├─ 运行 OutputValidator                             │
│     ├─ 如果验证失败（ErrModelRetry）→ 回到步骤 3        │
│     └─ 如果验证通过 → 返回 Result[O]                    │
│                                                         │
└─────────────────────────────────────────────────────────┘
```

---

## 六、完整使用示例

```go
package main

import (
    "context"
    "fmt"
    "log"

    "github.com/codycode/cody-core-go/agent"
    "github.com/codycode/cody-core-go/model"
    "github.com/codycode/cody-core-go/tool"
)

// 1. 定义依赖
type MyDeps struct {
    DB     *sql.DB
    UserID string
}

// 2. 定义输出类型
type OrderSummary struct {
    OrderID     string  `json:"order_id"`
    TotalAmount float64 `json:"total_amount" validate:"gte=0"`
    Status      string  `json:"status"`
    Summary     string  `json:"summary"`
}

// 3. 定义工具参数
type GetOrderArgs struct {
    OrderID string `json:"order_id" description:"订单ID" required:"true"`
}

func main() {
    // 4. 创建工具
    getOrder := tool.NewTool[MyDeps, GetOrderArgs](
        "get_order",
        "Get order details by order ID",
        func(ctx *agent.RunContext[MyDeps], args GetOrderArgs) (string, error) {
            row := ctx.Deps.DB.QueryRowContext(ctx.Ctx,
                "SELECT id, amount, status FROM orders WHERE id = ? AND user_id = ?",
                args.OrderID, ctx.Deps.UserID,
            )
            // ... 查询并返回 JSON
            return orderJSON, nil
        },
    )

    // 5. 创建 Agent
    myAgent := agent.New[MyDeps, OrderSummary](
        "openai:gpt-4o",
        agent.WithSystemPrompt[MyDeps, OrderSummary]("You are a helpful order assistant."),
        agent.WithDynamicSystemPrompt[MyDeps, OrderSummary](
            func(ctx *agent.RunContext[MyDeps]) (string, error) {
                return fmt.Sprintf("Current user ID: %s", ctx.Deps.UserID), nil
            },
        ),
        agent.WithTool[MyDeps, OrderSummary](getOrder),
        agent.WithOutputValidator[MyDeps, OrderSummary](
            func(ctx context.Context, o OrderSummary) (OrderSummary, error) {
                if o.TotalAmount < 0 {
                    return o, &agent.ErrModelRetry{Message: "total_amount cannot be negative"}
                }
                return o, nil
            },
        ),
        agent.WithMaxRetries[MyDeps, OrderSummary](3),
    )

    // 6. 运行
    deps := MyDeps{DB: db, UserID: "user_123"}
    result, err := myAgent.Run(context.Background(), "查询订单 ORD-456 的详情", deps)
    if err != nil {
        log.Fatal(err)
    }

    fmt.Printf("订单: %s, 金额: %.2f, 状态: %s\n",
        result.Output.OrderID,
        result.Output.TotalAmount,
        result.Output.Status,
    )
    fmt.Printf("Token 用量: %+v\n", result.Usage)
}
```

### 流式示例

```go
stream, err := myAgent.RunStream(ctx, "讲一个故事", deps)
if err != nil {
    log.Fatal(err)
}

// 流式文本
for text := range stream.TextStream() {
    fmt.Print(text)
}

// 或获取最终结构化结果
final, err := stream.Final()
```

### 多 Agent 委托示例

```go
// 子 Agent
subAgent := agent.New[MyDeps, SubResult]("openai:gpt-4o",
    agent.WithSystemPrompt[MyDeps, SubResult]("You are a specialist."),
)

// 主 Agent 的 tool 中委托给子 Agent
delegateTool := tool.NewTool[MyDeps, DelegateArgs](
    "delegate_to_specialist",
    "Delegate a specialized question to the specialist agent",
    func(ctx *agent.RunContext[MyDeps], args DelegateArgs) (string, error) {
        result, err := subAgent.Run(ctx.Ctx, args.Question, ctx.Deps)
        if err != nil {
            return "", err
        }
        return result.Output.Summary, nil
    },
)
```

### 测试示例

```go
func TestMyAgent(t *testing.T) {
    testModel := testing.NewTestModel(
        testing.WithResultText(`{"order_id":"ORD-1","total_amount":99.9,"status":"completed","summary":"test"}`),
    )

    myAgent := myAgent.WithModel(testModel)

    result, err := myAgent.Run(context.Background(), "查询订单", MyDeps{
        DB:     mockDB,
        UserID: "test_user",
    })

    assert.NoError(t, err)
    assert.Equal(t, "ORD-1", result.Output.OrderID)
}
```

---

## 七、实现路径（分阶段）

### Phase 1：核心骨架（MVP）

**目标：** 跑通一个完整的 Agent loop，支持结构化输出和工具调用。

| 模块 | 交付物 |
|------|--------|
| `model/` | Model 接口、Message 类型体系、ModelSettings |
| `model/providers/openai/` | OpenAI Provider（Chat Completions + Function Calling） |
| `output/` | struct → JSON Schema 自动生成、JSON 反序列化验证 |
| `tool/` | Tool 接口、NewTool 泛型构造、参数 struct → JSON Schema |
| `agent/` | Agent[D,O] 核心、RunContext、Run()、Agent Loop、ErrModelRetry |
| `testing/` | TestModel 基本实现 |

**验收标准：**
- 能创建一个 Agent，注册 tool，运行一次完整的 loop，得到类型化输出
- 输出经过 JSON Schema 验证
- Tool call → Tool result → 继续对话 循环正常
- TestModel 可用于单元测试

### Phase 2：流式 & 多 Provider

**目标：** 支持流式输出，接入更多模型。

| 模块 | 交付物 |
|------|--------|
| `agent/` | RunStream()、StreamResult、TextStream/OutputStream |
| `model/providers/anthropic/` | Anthropic Provider（含 Thinking 模式） |
| `model/providers/ollama/` | Ollama Provider（通过 OpenAI 兼容 API） |
| `model/` | FromString() 模型解析、FallbackModel（自动故障切换） |
| `output/` | 流式 partial 结构化输出 |
| `agent/` | 消息历史（多轮对话）支持 |

**验收标准：**
- 流式输出能逐 token 返回
- 结构化输出流式 partial 可用
- Anthropic / Ollama Provider 可正常运行
- 多轮对话通过 message_history 正常工作

### Phase 3：高级工具 & MCP

**目标：** 完善工具系统，支持 MCP 协议。

| 模块 | 交付物 |
|------|--------|
| `tool/` | Toolset（工具集）、DynamicToolset |
| `tool/` | DeferredTool（Human-in-the-Loop） |
| `mcp/` | MCP Client（stdio + SSE） |
| `mcp/` | MCP Server（将 Agent tools 暴露为 MCP） |
| `agent/` | 多模态输入（Image/Audio/Document Part） |

**验收标准：**
- Toolset 可组合管理多个工具
- DeferredTool 可暂停等待外部确认
- MCP Client 能连接标准 MCP Server 并调用工具
- MCP Server 能暴露 Agent 工具供外部调用

### Phase 4：可观测性 & 生态

**目标：** 完善可观测性，增加 Embedding 等辅助能力。

| 模块 | 交付物 |
|------|--------|
| `otel/` | OpenTelemetry Trace 中间件（Agent/Tool/Model 三层 span） |
| `embedding/` | EmbeddingModel 接口 + OpenAI 实现 |
| `model/providers/` | 更多 Provider（Gemini、Bedrock、Groq 等） |
| `agent/` | UsageLimits 完整实现 |

**验收标准：**
- OTel trace 完整覆盖 Agent 运行全流程
- Embedding 接口可用
- 至少 5 个模型 Provider 可用

### Phase 5（远期）：图执行 & Eval

**目标：** 高级编排和评估能力。

| 模块 | 交付物 |
|------|--------|
| `graph/` | 类 Pydantic Graph 的状态机引擎 |
| `eval/` | 评估框架（Dataset、Evaluator、Report） |
| `agent/` | iter() 细粒度节点控制 |

---

## 八、关键技术决策

### 8.1 JSON Schema 生成

采用 **reflect + struct tag** 方案，从 Go struct 自动生成 JSON Schema：

```go
type SearchArgs struct {
    Query      string `json:"query" description:"搜索关键词" required:"true"`
    MaxResults int    `json:"max_results,omitempty" description:"最大结果数" default:"10"`
}

// 自动生成：
// {
//   "type": "object",
//   "properties": {
//     "query": {"type": "string", "description": "搜索关键词"},
//     "max_results": {"type": "integer", "description": "最大结果数", "default": 10}
//   },
//   "required": ["query"]
// }
```

考虑复用已有库如 `invopop/jsonschema` 并扩展自定义 tag。

### 8.2 流式输出方案

采用 **channel + callback 双模式**：

```go
// Channel 模式（推荐，Go-idiomatic）
for text := range stream.TextStream() {
    fmt.Print(text)
}

// Callback 模式（简单场景）
stream.OnText(func(text string) {
    fmt.Print(text)
})
```

### 8.3 错误处理

遵循 Go 惯例，用具体 error 类型而非异常：

```go
var ErrUsageLimitExceeded = errors.New("usage limit exceeded")
var ErrMaxRetriesExceeded = errors.New("max retries exceeded")

type ErrModelRetry struct { Message string }
type ErrToolNotFound struct { Name string }
type ErrOutputValidation struct { Errors []ValidationError }
```

### 8.4 并发安全

- Agent 实例是**不可变的**（创建后配置不变），可安全并发使用
- RunContext 每次 Run 创建新实例，不需要加锁
- StreamResult 内部使用 channel，天然并发安全

---

## 九、与 Pydantic AI 功能对照清单

| 功能 | Pydantic AI | Go 实现 | Phase |
|------|-------------|---------|-------|
| 泛型 Agent[D, O] | ✅ | ✅ `Agent[D, O any]` | P1 |
| Run / RunSync | ✅ | ✅ `Run()` (Go 天然同步) | P1 |
| RunStream | ✅ | ✅ `RunStream()` | P2 |
| 结构化输出（struct） | ✅ | ✅ struct + JSON Schema | P1 |
| 结构化输出（Union） | ✅ | ⚠️ 接口 + 类型注册 | P2 |
| Output Validator | ✅ | ✅ `ValidatorFunc[O]` | P1 |
| 流式 partial 输出 | ✅ | ✅ channel 逐步推送 | P2 |
| 依赖注入 RunContext | ✅ | ✅ `RunContext[D]` | P1 |
| Tool 自动 Schema | ✅ | ✅ reflect + struct tag | P1 |
| Toolset | ✅ | ✅ `Toolset[D]` | P3 |
| Deferred Tool | ✅ | ✅ `DeferredTool` | P3 |
| ModelRetry | ✅ | ✅ `ErrModelRetry` | P1 |
| 消息历史 / 多轮对话 | ✅ | ✅ `[]Message` 传入 | P2 |
| 多 Agent 委托 | ✅ | ✅ Tool 内调用子 Agent | P1 |
| MCP Client | ✅ | ✅ stdio + SSE | P3 |
| MCP Server | ✅ | ✅ | P3 |
| OpenTelemetry | ✅ | ✅ `otel/` 中间件 | P4 |
| TestModel | ✅ | ✅ `testing/` 包 | P1 |
| FunctionModel | ✅ | ✅ | P1 |
| 多模态输入 | ✅ | ✅ Part 类型体系 | P3 |
| Thinking 模式 | ✅ | ✅ ThinkingPart | P2 |
| Embedding | ✅ | ✅ `embedding/` 包 | P4 |
| 模型字符串解析 | ✅ | ✅ `FromString()` | P2 |
| Fallback Model | ✅ | ✅ | P2 |
| Graph 引擎 | ✅ | 🔜 远期 | P5 |
| Eval 框架 | ✅ | 🔜 远期 | P5 |
| 持久化执行 | ✅ | ❌ 不在首期范围 | - |
| UI 集成 | ✅ | ❌ 不在首期范围 | - |
| A2A 协议 | ✅ | ❌ 不在首期范围 | - |

---

## 十、总结

本项目目标是打造一个 **Go-native 的类 Pydantic AI Agent 框架**，核心卖点是：

1. **泛型类型安全** — `Agent[D, O]` 编译期保证输入输出类型正确
2. **结构化输出 + 自动验证** — struct → JSON Schema → LLM → 反序列化 → 验证，全自动
3. **工具自动注册** — 从函数签名和 struct tag 自动生成 tool schema
4. **可测试** — TestModel + FunctionModel，零 API 调用的单元测试
5. **Go 并发优势** — goroutine 驱动的流式输出，天然高性能

通过 5 个阶段逐步实现，Phase 1 聚焦 MVP（约 2-3 周），快速验证核心 Agent loop 的可行性。
