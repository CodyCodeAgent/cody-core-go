# Cody Core Go — 基于 Eino 实现 Pydantic AI 设计文档

> 版本：v0.3 Draft
> 日期：2026-03-17
> 模块名：`github.com/codycode/cody-core-go`（暂定）
> 底层框架：[cloudwego/eino](https://github.com/cloudwego/eino) v0.8.x

---

## 一、项目目标

### 1.1 愿景

**基于 Eino 框架**，在 Go 生态中实现一个 Pydantic AI 风格的 Agent 开发体验。不重新造轮子，而是在 Eino 已有的 ChatModel、Tool、Agent、Compose 等基础设施之上，补齐 Pydantic AI 的核心差异化能力：**结构化输出 + 自动验证、泛型类型安全的 Agent 封装、依赖注入、TestModel**。

### 1.2 为什么选择 Eino

| 维度 | Eino 已有能力 | 我们要补的 |
|------|-------------|-----------|
| **ChatModel 抽象** | ✅ 统一接口，已有 OpenAI/Claude/Gemini/Ollama 等 Provider（eino-ext） | 无需重写 |
| **Tool 系统** | ✅ `InferTool` 从函数签名自动生成 schema | 补充输出验证工具、Deferred Tool |
| **Agent 模式** | ✅ ChatModelAgent（ReAct loop）、DeepAgent、Supervisor、PlanExecute | 补充结构化输出 Agent 封装 |
| **Streaming** | ✅ 框架自动处理流拼接、复制、合并 | 补充结构化流式 partial 输出 |
| **Compose/Graph** | ✅ Graph / Chain / Workflow 三种编排 API | 无需重写 |
| **Callback/可观测** | ✅ OnStart/OnEnd/OnError 切面 + tracing | 适配增强 |
| **MCP** | ✅ eino-ext 已有 MCP 支持 | 无需重写 |
| **Human-in-the-Loop** | ✅ ADK 原生支持 interrupt/resume | 映射到 Deferred Tool 语义 |
| **Multi-Agent** | ✅ Host 模式、Transfer、AgentAsTool | 映射到 Pydantic AI 的 Delegation 模式 |
| **结构化输出 + 自动验证** | ❌ 需手动 JSON + validator | **核心要补的** |
| **泛型 Agent[D, O]** | ❌ 基于 interface 抽象 | **核心要补的** |
| **依赖注入 RunContext** | ❌ 用 context.Context 传值 | **核心要补的** |
| **TestModel** | ❌ 较少测试工具 | **核心要补的** |
| **输出验证 + ModelRetry** | ❌ 无自动重试验证循环 | **核心要补的** |
| **Eval 框架** | ❌ 基本无 | 远期补充 |

**结论：Eino 覆盖了约 60-70% 的基础能力（模型、工具、编排、流式、可观测），我们聚焦在上层的"Pydantic AI 体验层"。**

### 1.3 核心设计原则

| 原则 | 说明 |
|------|------|
| **Build on Eino, not fork** | 依赖 Eino 作为底层，不 fork 不重写，通过组合扩展 |
| **Go-idiomatic** | 用 Go 泛型、接口、context 重新设计 Pydantic AI 的 API |
| **类型安全** | 利用 Go 1.18+ 泛型，Agent 的输入、输出、依赖全部类型化，编译期检查 |
| **结构化输出优先** | 输出自动反序列化 + 校验，是框架的一等能力，这是 Eino 缺失的核心 |
| **薄封装** | 不做重量级抽象，让用户需要时可以直接使用 Eino 底层 API |
| **可测试** | 内置 TestModel，依赖注入，方便单元测试 |

### 1.4 不做什么（明确边界）

- **不重写** ChatModel / Tool / Compose / Callback 等 Eino 已有抽象
- **不重写** 各模型 Provider（直接用 eino-ext 的 OpenAI/Anthropic/Gemini 等）
- **不重写** MCP Client/Server（直接用 eino-ext 的 MCP 实现）
- **不重写** Graph 编排引擎（直接用 Eino Compose）
- **不做** UI 框架（AG-UI / Vercel AI SDK 对接留给上层）
- **不做** 持久化执行引擎

---

## 二、架构分层

```
┌─────────────────────────────────────────────────────────────────┐
│                    用户应用代码                                    │
├─────────────────────────────────────────────────────────────────┤
│                                                                  │
│  cody-core-go（本项目）— Pydantic AI 体验层                       │
│  ┌────────────┬──────────────┬────────────┬──────────────┐       │
│  │ Agent[D,O] │ Structured   │ RunContext │ TestModel    │       │
│  │ 泛型封装    │ Output       │ 依赖注入    │ 测试工具     │       │
│  │            │ + Validator  │            │              │       │
│  └─────┬──────┴──────┬───────┴─────┬──────┴──────┬───────┘       │
│        │             │             │             │               │
├────────┼─────────────┼─────────────┼─────────────┼───────────────┤
│        ▼             ▼             ▼             ▼               │
│  Eino 框架层                                                      │
│  ┌────────────┬──────────────┬────────────┬──────────────┐       │
│  │ ADK        │ Components   │ Compose    │ Callbacks    │       │
│  │ ChatModel  │ model.       │ Graph/     │ OnStart/     │       │
│  │ Agent      │ ChatModel    │ Chain/     │ OnEnd/       │       │
│  │ DeepAgent  │ tool.Tool    │ Workflow   │ OnError      │       │
│  └─────┬──────┴──────┬───────┴────────────┴──────────────┘       │
│        │             │                                           │
├────────┼─────────────┼───────────────────────────────────────────┤
│        ▼             ▼                                           │
│  Eino-Ext（Provider 实现层）                                      │
│  ┌──────────┬──────────┬──────────┬──────────┬────────────┐      │
│  │ OpenAI   │ Claude   │ Gemini   │ Ollama   │ MCP / ...  │      │
│  └──────────┴──────────┴──────────┴──────────┴────────────┘      │
│                                                                  │
└─────────────────────────────────────────────────────────────────┘
```

---

## 三、Eino 核心概念映射

### 3.1 Eino 已有 → Pydantic AI 对应

| Eino 概念 | Pydantic AI 对应 | 关系 |
|-----------|-----------------|------|
| `components/model.ChatModel` | Model 接口 | **直接使用**，不再封装 |
| `schema.Message` / `schema.SystemMessage` / `schema.UserMessage` | Message 类型体系 | **直接使用** |
| `components/tool.InferableTool` + `utils.InferTool()` | `@agent.tool` 装饰器注册 | **直接使用**，Eino 已能从函数签名推断 schema |
| `adk.ChatModelAgent` | Agent.run()（ReAct loop） | **封装增强**，加结构化输出 + 验证 |
| `adk.Runner` | Agent 运行器 | **封装增强** |
| `compose.Graph` | Pydantic Graph | **直接使用** |
| `callbacks.Handler` | 可观测性 | **直接使用** + 增强 |
| ADK `interrupt/resume` | Deferred Tool | **映射适配** |
| eino-ext MCP | MCP Client/Server | **直接使用** |

### 3.2 我们需要新建的

| 能力 | 说明 | Eino 缺失原因 |
|------|------|-------------|
| **`Agent[D, O]` 泛型封装** | 类型化的 Agent 入口，编译期检查输入输出 | Eino Agent 基于 interface，无泛型约束 |
| **结构化输出 OutputType** | 自动生成 output tool / native structured output / prompted output | Eino 不处理输出结构化 |
| **Output Validator + ModelRetry** | 验证输出 → 失败时自动重试（带反馈） | Eino 无此机制 |
| **`RunContext[D]` 依赖注入** | 类型安全的依赖传递给 tool 和 system prompt | Eino 用 `context.Context` 传值，无类型检查 |
| **Dynamic System Prompt** | system prompt 根据依赖动态生成 | Eino ChatModelAgent 只支持静态 prompt |
| **TestModel / FunctionModel** | 测试用模拟模型 | Eino 无内置测试模型 |
| **Eval 框架** | Dataset + Evaluator + Report | Eino 完全没有 |

---

## 四、包结构设计

```
cody-core-go/
├── go.mod                              # 依赖 github.com/cloudwego/eino
├── go.sum
├── README.md
├── docs/
│   └── design.md                       # 本文档
│
├── agent/                              # 核心：泛型 Agent 封装（构建在 Eino ADK 之上）
│   ├── agent.go                        # Agent[D, O] 定义 + 运行逻辑
│   ├── agent_test.go
│   ├── context.go                      # RunContext[D] 依赖注入
│   ├── options.go                      # AgentOption / RunOption
│   ├── result.go                       # Result[O] / StreamResult[O]
│   ├── retry.go                        # ErrModelRetry / 重试逻辑
│   ├── conversation.go                 # Conversation[D, O] 多轮对话封装
│   └── union.go                        # OneOf2 / OneOf3 Union 输出类型
│
├── output/                             # 核心：结构化输出系统
│   ├── output.go                       # OutputMode (Tool/Native/Prompted)
│   ├── schema.go                       # struct → JSON Schema 自动生成（支持 struct/原始类型/切片）
│   ├── validator.go                    # OutputValidator + 验证重试
│   └── tool_output.go                  # Tool 模式实现（生成 output tool）
│
├── direct/                             # 直接模型请求（绕过 Agent）
│   └── direct.go                       # RequestText / Request[T] 轻量封装
│
├── deps/                               # 核心：依赖注入辅助
│   └── deps.go                         # RunContext 工具函数、依赖提取
│
├── testutil/                           # 核心：测试工具
│   ├── testmodel.go                    # TestModel（实现 Eino ChatModel 接口）
│   ├── funcmodel.go                    # FunctionModel
│   └── assertions.go                   # Agent 测试断言辅助
│
├── eval/                               # 评估框架（Phase 5，尚未实现）
│   ├── dataset.go                      # Dataset / Case 定义
│   ├── evaluator.go                    # Evaluator 接口 + 内置实现
│   └── report.go                       # Report 输出
│
└── examples/                           # 示例代码（尚未实现）
    ├── basic/                          # 基础 Agent 用法
    ├── structured_output/              # 结构化输出
    ├── multi_agent/                    # 多 Agent 委托
    ├── streaming/                      # 流式输出
    └── testing/                        # 测试示例
```

**关键点：没有 `model/`、`tool/`、`mcp/`、`compose/` 这些包——全部复用 Eino。**

---

## 五、核心 API 设计

### 5.1 Agent[D, O] — 泛型 Agent 封装

```go
package agent

import (
    "context"

    "github.com/cloudwego/eino/components/model"
    einotool "github.com/cloudwego/eino/components/tool"
    "github.com/cloudwego/eino/schema"
)

// Agent 是框架的核心抽象
// D = 依赖类型（通过 RunContext 传递给 tool 和 system prompt）
// O = 输出类型（自动结构化验证）
type Agent[D any, O any] struct {
    chatModel        model.BaseChatModel       // Eino BaseChatModel（由 eino-ext 提供实现）
    systemPrompts    []SystemPromptFunc[D]      // 动态 system prompt
    staticPrompts    []string                   // 静态 system prompt
    tools            []toolEntry[D]             // 用户工具列表（含可选 prepare 回调）
    outputMode       output.Mode                // 结构化输出模式
    outputValidators []output.ValidatorFunc[O]  // 输出验证器链
    maxToolRetries   int                        // 每个工具的最大重试次数（默认 1）
    maxResultRetries int                        // 输出验证的最大重试次数（默认 1）
    maxIterations    int                        // Agent Loop 最大迭代次数（默认 20）
    modelSettings    map[string]any             // 模型参数覆盖
    initErrors       []error                    // 构建时收集的错误（延迟到 Run 时返回）
    outputTools      []outputToolEntry          // Union 类型的多个 output tool
    outputParser     func(toolName string, argsJSON []byte) (O, error) // Union 类型的自定义解析器
}

// SystemPromptFunc 动态 system prompt 生成函数
type SystemPromptFunc[D any] func(ctx *RunContext[D]) (string, error)

// 创建 Agent — 接受 Eino BaseChatModel 实例
func New[D any, O any](chatModel model.BaseChatModel, opts ...Option[D, O]) *Agent[D, O]

// 同步运行（Go 天然同步，不需要 run_sync）
func (a *Agent[D, O]) Run(ctx context.Context, prompt string, deps D, opts ...RunOption) (*Result[O], error)

// 流式运行
func (a *Agent[D, O]) RunStream(ctx context.Context, prompt string, deps D, opts ...RunOption) (*StreamResult[O], error)

// 传入消息历史（多轮对话）——便捷方法，等价于 Run + WithHistory RunOption
func (a *Agent[D, O]) RunWithHistory(
    ctx context.Context,
    prompt string,
    deps D,
    history []*schema.Message,  // 直接用 Eino 的 Message 类型
    opts ...RunOption,
) (*Result[O], error)

// 替换模型（用于测试）——返回浅拷贝
func (a *Agent[D, O]) WithModel(m model.BaseChatModel) *Agent[D, O]
```

### 5.2 RunContext[D] — 依赖注入

```go
package agent

// RunContext 携带运行时依赖，传递给 system prompt 和 tool
// 内部通过 context.WithValue 注入到 Eino 的 context.Context 中
type RunContext[D any] struct {
    Ctx      context.Context
    Deps     D                   // 类型安全的依赖
    Usage    *UsageTracker       // token 用量追踪
    Metadata map[string]any      // 额外元数据
    Retry    int                 // 当前重试次数
}

// 从 context.Context 中提取依赖（供 Eino Tool 实现使用）
func GetDeps[D any](ctx context.Context) (D, bool)

// 内部：将 RunContext 注入到 context.Context
func withRunContext[D any](ctx context.Context, rc *RunContext[D]) context.Context
```

**Eino 适配关键**：Eino 的 Tool 接口接收 `context.Context`，我们通过 `context.WithValue` 把 `RunContext[D]` 注入进去，Tool 实现通过 `GetDeps[D](ctx)` 提取。

### 5.3 Result[O] — 类型化结果

```go
package agent

import "github.com/cloudwego/eino/schema"

// Result 包含 agent 运行结果
type Result[O any] struct {
    Output          O              // 类型化的结构化输出
    Usage           Usage          // token 用量统计
    allMessages     []*schema.Message   // 所有消息（不含 system，私有字段）
    newMessageStart int                 // 新消息的起始索引
}

// NewMessages 返回本次 run 产生的新消息（不含 history 和 system）
func (r *Result[O]) NewMessages() []*schema.Message

// AllMessages 返回完整消息序列（含 history，不含 system）
func (r *Result[O]) AllMessages() []*schema.Message

// StreamResult 提供流式访问
type StreamResult[O any] struct {
    // 内部字段...
}

// 文本流
func (s *StreamResult[O]) TextStream() <-chan string

// 获取最终结果（等待流式完成）
func (s *StreamResult[O]) Final() (*Result[O], error)

// 释放资源
func (s *StreamResult[O]) Close()
```

### 5.4 Option 模式

```go
package agent

// Agent 构建选项
func WithSystemPrompt[D, O any](prompt string) Option[D, O]
func WithDynamicSystemPrompt[D, O any](fn SystemPromptFunc[D]) Option[D, O]
func WithTool[D, O any](t tool.InvokableTool) Option[D, O]             // 直接传 Eino Tool
func WithToolFunc[D, O any, Args any](                                   // 便捷：从函数创建
    name, desc string,
    fn func(ctx *RunContext[D], args Args) (string, error),
    opts ...ToolOption[D],                                               // 可选 prepare 回调
) Option[D, O]
func WithOutputMode[D, O any](mode output.Mode) Option[D, O]
func WithOutputValidator[D, O any](fn output.ValidatorFunc[O]) Option[D, O]
func WithMaxRetries[D, O any](n int) Option[D, O]                       // per-tool 重试上限
func WithMaxResultRetries[D, O any](n int) Option[D, O]                 // 输出验证重试上限
func WithModelSettings[D, O any](settings map[string]any) Option[D, O]

// 运行时选项
func WithHistory(history []*schema.Message) RunOption
func WithUsageLimits(limits UsageLimits) RunOption
func WithRunModelSettings(settings map[string]any) RunOption
func WithRunMetadata(meta map[string]any) RunOption
```

### 5.5 结构化输出系统

```go
package output

// Mode 输出模式
type Mode int

const (
    ModeTool    Mode = iota  // 通过 function calling 返回结构化数据（默认）
    ModeNative               // 使用模型原生 structured output API（如 OpenAI JSON mode）
    ModePrompted             // 通过 prompt 约束输出格式
)

// ValidatorFunc 输出验证函数
type ValidatorFunc[O any] func(ctx context.Context, output O) (O, error)

// RunValidators 依次执行验证器链
func RunValidators[O any](ctx context.Context, output O, validators []ValidatorFunc[O]) (O, error)

// IsPrimitive 判断 T 是否为非 struct 类型（int、bool、float64、切片等），需要包装为 {"result": ...}
func IsPrimitive[T any]() bool

// IsString 判断 T 是否为 string 类型
func IsString[T any]() bool

// BuildParamsOneOf 从 Go 类型自动生成 Eino ParamsOneOf
// 支持 struct tag: json, description, jsonschema, required, enum
// struct 类型直接生成 schema；原始类型/切片类型包装为 {"result": <schema>}
func BuildParamsOneOf[T any]() (*schema.ParamsOneOf, error)

// GenerateOutputTool 生成用于结构化输出的 Eino InvokableTool
// 当 Mode=ModeTool 时，框架自动注册一个名为 "final_result" 的 tool
// LLM 通过调用此 tool 返回结构化数据
func GenerateOutputTool[T any](paramsOneOf *schema.ParamsOneOf) tool.InvokableTool

// GenerateOutputToolWithName 生成自定义名称的 output tool（用于 Union 类型）
func GenerateOutputToolWithName[T any](name string, paramsOneOf *schema.ParamsOneOf) tool.InvokableTool

// IsOutputToolName 检查 tool name 是否为 output tool
func IsOutputToolName(name string) bool

// ParseStructuredOutput 从 JSON 反序列化输出
// 自动处理 Markdown fence 剥离、原始类型的 {"result": ...} 解包
func ParseStructuredOutput[T any](data []byte) (T, error)
```

### 5.6 工具系统 — 适配 Eino

```go
package agent

import (
    einotool "github.com/cloudwego/eino/components/tool"
    "github.com/cloudwego/eino/components/tool/utils"
)

// WithToolFunc 便捷创建带依赖注入的工具
// 内部将 RunContext[D] 通过 context.Value 传递，适配 Eino Tool 接口
func WithToolFunc[D, O any, Args any](
    name, desc string,
    fn func(ctx *RunContext[D], args Args) (string, error),
    opts ...ToolOption[D],
) Option[D, O] {
    // 内部实现：
    // 1. 调用 output.BuildParamsOneOf[Args]() 从 Args struct 自动生成参数 schema
    // 2. 构建 wrappedTool（实现 tool.InvokableTool），包装 fn：
    //    从 context.Context 中提取 RunContext[D]，json.Unmarshal 参数
    // 3. 附加可选的 prepare 回调
}

// 用户也可以直接传入 Eino 原生 Tool（无需依赖注入的场景）
func WithTool[D, O any](t tool.InvokableTool) Option[D, O]

// 直接使用 Eino 的 InferTool（无依赖注入，最简单的方式）
// searchTool, _ := utils.InferTool("search", "Search the web", mySearchFunc)
// agent := New[NoDeps, MyOutput](chatModel, WithTool[NoDeps, MyOutput](searchTool))
```

### 5.7 ErrModelRetry — 自我反思与纠错

```go
package agent

// ErrModelRetry 特殊 error 类型，触发模型重试
// Tool 或 OutputValidator 返回此 error 时：
// 1. 将 error message 作为反馈追加到对话中
// 2. 重新调用模型，让模型修正输出
type ErrModelRetry struct {
    Message string   // 反馈给模型的修正提示
}

func (e *ErrModelRetry) Error() string { return "model retry: " + e.Message }

// NewModelRetry 便捷构造
func NewModelRetry(msg string) *ErrModelRetry

// 在 OutputValidator 中使用示例：
// func(ctx context.Context, o MyOutput) (MyOutput, error) {
//     if o.Score < 0 {
//         return o, agent.NewModelRetry("score must be non-negative")
//     }
//     return o, nil
// }
```

### 5.8 TestModel — 测试支持

```go
package testutil

import (
    "github.com/cloudwego/eino/components/model"
    "github.com/cloudwego/eino/schema"
)

// TestModel 实现 Eino 的 model.ChatModel 接口
// 用于单元测试，不调用真实 API
type TestModel struct {
    responses []TestResponse    // 预设的响应序列
    calls     []TestCall        // 记录所有调用（用于断言）
}

type TestResponse struct {
    Text      string                   // 文本响应
    ToolCalls []schema.ToolCall        // tool call 响应（注意：值类型，非指针）
    Usage     *schema.TokenUsage       // 可选 token 用量
    Err       error                    // 如设置，Generate 返回此错误
}

type TestCall struct {
    Messages []*schema.Message
    Tools    []*schema.ToolInfo
}

func NewTestModel(responses ...TestResponse) *TestModel

// 实现 Eino BaseChatModel 接口
func (m *TestModel) Generate(ctx context.Context, input []*schema.Message, opts ...model.Option) (*schema.Message, error)
func (m *TestModel) Stream(ctx context.Context, input []*schema.Message, opts ...model.Option) (*schema.StreamReader[*schema.Message], error)

// 内省辅助
func (m *TestModel) CallCount() int
func (m *TestModel) LastCall() TestCall
func (m *TestModel) AllCalls() []TestCall
func (m *TestModel) Reset()  // 清空调用记录并重置响应索引

// FunctionModel 用自定义函数模拟模型行为
type FunctionModel struct {
    Handler func(messages []*schema.Message, tools []*schema.ToolInfo) (*schema.Message, error)
}

func NewFunctionModel(handler func([]*schema.Message, []*schema.ToolInfo) (*schema.Message, error)) *FunctionModel
```

### 5.9 Direct Model Requests — 直接模型请求

有时只需要做一次简单的 LLM 调用，不需要 Agent 的完整机制（tool、retry、结构化输出都不需要）。提供一组轻量级的 `direct` 包函数，绕过 Agent 直接调用模型。

```go
package direct

import (
    "context"

    "github.com/cloudwego/eino/components/model"
)

// RequestText 最简调用，返回纯文本
func RequestText(ctx context.Context, chatModel model.ChatModel, prompt string, opts ...RequestOption) (string, error)

// Request 带结构化输出的直接调用
// 内部：构建 messages + output tool → chatModel.Generate → 反序列化为 T
func Request[T any](ctx context.Context, chatModel model.ChatModel, prompt string, opts ...RequestOption) (T, error)

// RequestOption 可选参数
func WithSystemPrompt(prompt string) RequestOption
func WithMessages(msgs []*schema.Message) RequestOption   // 自定义消息列表
func WithModelSettings(settings map[string]any) RequestOption
```

**使用示例：**

```go
// 一行搞定纯文本请求
text, err := direct.RequestText(ctx, chatModel, "翻译这段话为英文：你好世界")

// 一行搞定结构化输出
type Sentiment struct {
    Label string  `json:"label" description:"positive/negative/neutral"`
    Score float64 `json:"score" description:"confidence 0-1"`
}

result, err := direct.Request[Sentiment](ctx, chatModel, "分析这段文本的情感倾向：我很开心")
fmt.Println(result.Label, result.Score) // positive 0.95
```

**内部实现很轻**：构建 system + user message → 如果 T != string 则追加一个 output tool → 调用 `chatModel.Generate` → 解析返回。本质就是 Agent.Run 的单次调用简化版，不含 tool 执行、retry 等循环逻辑。

### 5.10 Tool `prepare` 回调 — 动态参数裁剪

有些场景下，同一个 tool 需要根据运行时上下文动态修改其参数 schema。例如根据用户权限决定是否暴露某些高级参数。

```go
package agent

// PrepareFunc 在每次模型调用前执行，可动态修改 tool 的 ToolInfo（含参数 schema）
type PrepareFunc[D any] func(ctx *RunContext[D], toolInfo *schema.ToolInfo) (*schema.ToolInfo, error)

// WithToolFunc 增加 prepare 选项
func WithToolFunc[D, O any, Args any](
    name, desc string,
    fn func(ctx *RunContext[D], args Args) (string, error),
    opts ...ToolOption[D],                      // 新增
) Option[D, O]

// ToolOption 工具级选项
func WithPrepare[D any](fn PrepareFunc[D]) ToolOption[D]
```

**使用示例：**

```go
type SearchArgs struct {
    Query       string `json:"query" description:"搜索关键词"`
    AdminFilter string `json:"admin_filter,omitempty" description:"管理员过滤条件"`
}

myAgent := agent.New[MyDeps, string](chatModel,
    agent.WithToolFunc[MyDeps, string, SearchArgs](
        "search", "搜索数据库",
        func(ctx *agent.RunContext[MyDeps], args SearchArgs) (string, error) {
            // ... 执行搜索
            return results, nil
        },
        agent.WithPrepare[MyDeps](func(ctx *agent.RunContext[MyDeps], toolInfo *schema.ToolInfo) (*schema.ToolInfo, error) {
            if !ctx.Deps.IsAdmin {
                // 非管理员看不到这个参数（修改 ToolInfo 的 ParamsOneOf）
                // 具体实现视 ParamsOneOf 的底层结构而定
            }
            return toolInfo, nil
        }),
    ),
)
```

**实现要点：** Agent Loop 每次迭代的 `buildToolInfos` 方法中，遍历所有带 prepare 回调的 tool，调用 prepare 获取修改后的 `*schema.ToolInfo`，再传入 `model.WithTools()` Option。

### 5.11 Union 输出类型 — OneOf2/OneOf3

当一个 Agent 可能返回多种不同结构的结果时（例如成功/失败，或多种分类），使用 Union 输出类型。

**核心思路：** 给每种类型生成一个独立的 output tool（`final_result_TypeA`、`final_result_TypeB`），模型根据实际情况选择调用哪个，框架按对应类型反序列化。

```go
package agent

// OneOf2 表示"二选一"的输出类型
type OneOf2[A, B any] struct {
    value any  // 实际存的是 A 或 B
}

// Value 返回内部值，用于 type switch
func (u OneOf2[A, B]) Value() any { return u.value }

// Match 编译期强制处理所有分支
func (u OneOf2[A, B]) Match(onA func(A), onB func(B)) {
    switch v := u.value.(type) {
    case A:
        onA(v)
    case B:
        onB(v)
    }
}

// OneOf3 三选一，以此类推
type OneOf3[A, B, C any] struct { value any }
```

**使用示例：**

```go
type VulnFound struct {
    Vulnerability string `json:"vulnerability" description:"漏洞类型"`
    Severity      string `json:"severity" description:"严重程度" enum:"low,medium,high,critical"`
    Line          int    `json:"line" description:"出现行号"`
}

type Safe struct {
    Summary      string   `json:"summary" description:"安全摘要"`
    CheckedItems []string `json:"checked_items" description:"已检查项目"`
}

// 使用 NewOneOf2 创建 Union 输出的 Agent（内部自动生成多个 output tool + 自定义解析器）
reviewer := agent.NewOneOf2[MyDeps, VulnFound, Safe](chatModel,
    agent.WithSystemPrompt[MyDeps, agent.OneOf2[VulnFound, Safe]](
        "你是安全审查专家，检查代码安全性",
    ),
)

result, err := reviewer.Run(ctx, "检查这段代码", deps)

// 方式一：type switch
switch v := result.Output.Value().(type) {
case VulnFound:
    fmt.Printf("发现漏洞: %s (行 %d, 严重: %s)\n", v.Vulnerability, v.Line, v.Severity)
case Safe:
    fmt.Printf("安全: %s\n", v.Summary)
}

// 方式二：Match（编译期保证所有分支都被处理）
result.Output.Match(
    func(v VulnFound) { alertTeam(v) },
    func(s Safe)      { log.Info(s.Summary) },
)
```

**优势：** 比起一个大 struct 里塞一堆 optional 字段，Union 类型让每种输出都有精确的 schema，模型生成更准确，代码处理更清晰。

### 5.12 原始类型输出 — int/bool/[]string 等

除了 struct，Agent 的输出类型 `O` 也支持 Go 原始类型和切片类型：

```go
// 输出 int — 评分场景
scoreAgent := agent.New[agent.NoDeps, int](chatModel,
    agent.WithSystemPrompt[agent.NoDeps, int]("Rate the text quality from 1-10, return only the number."),
)
result, _ := scoreAgent.Run(ctx, "This is a great article about Go.", agent.NoDeps{})
fmt.Println(result.Output) // 8

// 输出 bool — 判断场景
checkAgent := agent.New[agent.NoDeps, bool](chatModel,
    agent.WithSystemPrompt[agent.NoDeps, bool]("Determine if the text contains harmful content."),
)
result, _ := checkAgent.Run(ctx, "Hello world!", agent.NoDeps{})
fmt.Println(result.Output) // false

// 输出 []string — 提取场景
tagsAgent := agent.New[agent.NoDeps, []string](chatModel,
    agent.WithSystemPrompt[agent.NoDeps, []string]("Extract key topics as a list of tags."),
)
result, _ := tagsAgent.Run(ctx, "Go 1.22 introduced range over integers...", agent.NoDeps{})
fmt.Println(result.Output) // [go generics range]
```

**实现要点：** `output.BuildParamsOneOf[T]()` 通过反射检测 `T` 的 Kind，对非 struct 类型生成对应的 JSON Schema：

| Go 类型 | JSON Schema type | 备注 |
|---------|-----------------|------|
| `int` / `int64` | `{"type": "integer"}` | |
| `float64` | `{"type": "number"}` | |
| `bool` | `{"type": "boolean"}` | |
| `string` | 直接取文本，不走 output tool | 已有逻辑 |
| `[]T` | `{"type": "array", "items": {...}}` | 递归处理 |

当 `O` 是原始类型时，生成的 `final_result` tool 参数为 `{"result": <schema>}`，框架解析时取 `result` 字段。

### 5.13 Conversation — 多轮对话便捷封装

`Conversation` 是一个轻量级的内存对象，自动管理多轮对话的消息历史传递。不做持久化，不存数据库——只是省掉用户每次手动传 `message_history` 的样板代码。

```go
package agent

// Conversation 封装多轮对话状态（纯内存）
type Conversation[D, O any] struct {
    agent    *Agent[D, O]
    messages []*schema.Message   // 累积的消息历史
}

// NewConversation 创建对话实例
func NewConversation[D, O any](a *Agent[D, O]) *Conversation[D, O]

// Send 发送消息并自动携带历史
func (c *Conversation[D, O]) Send(ctx context.Context, prompt string, deps D, opts ...RunOption) (*Result[O], error) {
    result, err := c.agent.Run(ctx, prompt, deps,
        append(opts, WithHistory(c.messages))...,
    )
    if err != nil {
        return nil, err
    }
    c.messages = result.AllMessages()   // 更新历史
    return result, nil
}

// SendStream 流式版本
func (c *Conversation[D, O]) SendStream(ctx context.Context, prompt string, deps D, opts ...RunOption) (*StreamResult[O], error)

// Messages 获取当前完整的消息历史
func (c *Conversation[D, O]) Messages() []*schema.Message

// Reset 清空历史，开始新对话
func (c *Conversation[D, O]) Reset()
```

**使用示例：**

```go
conv := agent.NewConversation(myAgent)

// 多轮对话，无需手动管理 history
r1, _ := conv.Send(ctx, "我叫张三", deps)
r2, _ := conv.Send(ctx, "我刚才说我叫什么？", deps)   // 自动带上 r1 的历史
r3, _ := conv.Send(ctx, "帮我总结一下我们的对话", deps) // 自动带上 r1+r2 的历史

// 清空，开启新对话
conv.Reset()
r4, _ := conv.Send(ctx, "你好", deps)  // 全新对话，无历史

// 需要持久化？自己拿出去存
history := conv.Messages()
data, _ := json.Marshal(history)
// 存到 Redis/DB/文件...随你
```

**注意：** `Conversation` 不是并发安全的（多轮对话本身就是顺序的）。如果需要并发的多用户对话，每个用户创建独立的 `Conversation` 实例。

---

## 六、Agent 运行循环（核心流程）

内部基于 Eino 的 ChatModel + Tool 驱动，上层增加结构化输出和验证循环：

```
┌─────────────────────────────────────────────────────────────┐
│                     Agent[D, O].Run()                        │
├─────────────────────────────────────────────────────────────┤
│                                                              │
│  1. 创建 RunContext[D]，注入到 context.Context                │
│                                                              │
│  2. 构建 system prompt（静态 + 动态 SystemPromptFunc）        │
│                                                              │
│  3. 准备 tools 列表                                          │
│     ├─ 用户注册的 Eino Tool                                  │
│     ├─ 如果 O != string → 追加 "final_result" output tool    │
│     └─ MCP Server 的 tools（如有）                           │
│                                                              │
│  4. 构建初始 messages（Eino schema.Message 类型）             │
│     ├─ system messages                                       │
│     ├─ message history（如有）                                │
│     └─ user prompt message                                   │
│                                                              │
│  ┌─── Agent Loop ──────────────────────────────────────┐     │
│  │                                                      │     │
│  │  5. chatModel.Generate(ctx, messages, tools)         │     │
│  │     └─ 调用 Eino ChatModel（底层 = OpenAI/Claude等） │     │
│  │                                                      │     │
│  │  6. 解析 Eino 响应 (*schema.Message)                  │     │
│  │     ├─ 纯文本且 O=string → 跳到步骤 8                │     │
│  │     ├─ ToolCall name="final_result" → 跳到步骤 8     │     │
│  │     └─ 其他 ToolCall → 步骤 7                        │     │
│  │                                                      │     │
│  │  7. 执行 Tool Calls（Eino Tool.Run）                  │     │
│  │     ├─ 从 context 提取 RunContext[D] → 传递依赖       │     │
│  │     ├─ 收集 tool results                              │     │
│  │     ├─ ErrModelRetry → 作为 tool error 反馈给模型     │     │
│  │     ├─ 追加 tool call + tool result messages          │     │
│  │     └─ 回到步骤 5                                     │     │
│  │                                                      │     │
│  └──────────────────────────────────────────────────────┘     │
│                                                              │
│  8. 解析最终输出                                              │
│     ├─ O=string → 直接取文本                                 │
│     ├─ O=struct → JSON 反序列化为 O                          │
│     ├─ 运行 OutputValidator                                  │
│     ├─ ErrModelRetry → 反馈给模型，回到步骤 5                │
│     │   （检查 retry 次数，超限返回 ErrMaxRetriesExceeded）   │
│     └─ 验证通过 → 返回 Result[O]                             │
│                                                              │
└─────────────────────────────────────────────────────────────┘
```

---

## 七、完整使用示例

### 基础用法

```go
package main

import (
    "context"
    "fmt"
    "log"

    "github.com/codycode/cody-core-go/agent"
    "github.com/codycode/cody-core-go/output"

    // Eino 模型 Provider
    einoopenai "github.com/cloudwego/eino-ext/components/model/openai"
)

// 1. 定义依赖
type MyDeps struct {
    DB     *sql.DB
    UserID string
}

// 2. 定义输出类型（自动生成 JSON Schema）
type OrderSummary struct {
    OrderID     string  `json:"order_id" description:"订单号"`
    TotalAmount float64 `json:"total_amount" description:"订单总金额"`
    Status      string  `json:"status" description:"订单状态" enum:"pending,completed,cancelled"`
    Summary     string  `json:"summary" description:"订单摘要"`
}

// 3. 定义工具参数
type GetOrderArgs struct {
    OrderID string `json:"order_id" description:"订单ID" required:"true"`
}

func main() {
    ctx := context.Background()

    // 4. 创建 Eino ChatModel（使用 eino-ext OpenAI Provider）
    chatModel, err := einoopenai.NewChatModel(ctx, &einoopenai.ChatModelConfig{
        Model:  "gpt-4o",
        APIKey: os.Getenv("OPENAI_API_KEY"),
    })
    if err != nil {
        log.Fatal(err)
    }

    // 5. 创建 Agent
    myAgent := agent.New[MyDeps, OrderSummary](
        chatModel,
        agent.WithSystemPrompt[MyDeps, OrderSummary]("You are a helpful order assistant."),
        agent.WithDynamicSystemPrompt[MyDeps, OrderSummary](
            func(ctx *agent.RunContext[MyDeps]) (string, error) {
                return fmt.Sprintf("Current user ID: %s", ctx.Deps.UserID), nil
            },
        ),
        // 便捷方式：从函数 + 参数 struct 自动创建 Eino Tool
        agent.WithToolFunc[MyDeps, OrderSummary, GetOrderArgs](
            "get_order", "Get order details by order ID",
            func(ctx *agent.RunContext[MyDeps], args GetOrderArgs) (string, error) {
                // ctx.Deps 是类型安全的 MyDeps
                row := ctx.Deps.DB.QueryRowContext(ctx.Ctx,
                    "SELECT id, amount, status FROM orders WHERE id = ? AND user_id = ?",
                    args.OrderID, ctx.Deps.UserID,
                )
                // ... 查询并返回 JSON
                return orderJSON, nil
            },
        ),
        agent.WithOutputValidator[MyDeps, OrderSummary](
            func(ctx context.Context, o OrderSummary) (OrderSummary, error) {
                if o.TotalAmount < 0 {
                    return o, agent.NewModelRetry("total_amount cannot be negative")
                }
                return o, nil
            },
        ),
        agent.WithMaxRetries[MyDeps, OrderSummary](3),
    )

    // 6. 运行
    result, err := myAgent.Run(ctx, "查询订单 ORD-456 的详情", MyDeps{
        DB:     db,
        UserID: "user_123",
    })
    if err != nil {
        log.Fatal(err)
    }

    // result.Output 是类型安全的 OrderSummary
    fmt.Printf("订单: %s, 金额: %.2f, 状态: %s\n",
        result.Output.OrderID,
        result.Output.TotalAmount,
        result.Output.Status,
    )
}
```

### 纯文本 Agent（最简用法）

```go
// O = string 时，不生成 output tool，直接返回文本
chatAgent := agent.New[agent.NoDeps, string](
    chatModel,
    agent.WithSystemPrompt[agent.NoDeps, string]("You are a helpful assistant."),
)

result, err := chatAgent.Run(ctx, "Hello!", agent.NoDeps{})
fmt.Println(result.Output) // string 类型
```

### 直接使用 Eino 原生 Tool

```go
import "github.com/cloudwego/eino/components/tool/utils"

// 用 Eino 的 InferTool 创建（不需要依赖注入的场景）
calcTool, _ := utils.InferTool("calculate", "Evaluate math expression", func(ctx context.Context, args struct {
    Expression string `json:"expression" description:"math expression"`
}) (string, error) {
    // ...
    return result, nil
})

myAgent := agent.New[agent.NoDeps, MyOutput](
    chatModel,
    agent.WithTool[agent.NoDeps, MyOutput](calcTool),  // 直接传 Eino Tool
)
```

### 流式输出

```go
stream, err := myAgent.RunStream(ctx, "讲一个故事", deps)
if err != nil {
    log.Fatal(err)
}

// 流式文本（底层用 Eino 的 StreamReader）
for text := range stream.TextStream() {
    fmt.Print(text)
}

// 或获取最终结构化结果
final, err := stream.Final()
```

### 多 Agent 委托

```go
// 子 Agent（用 Eino 的 Anthropic Provider）
claudeModel, _ := einoanthropic.NewChatModel(ctx, &einoanthropic.ChatModelConfig{
    Model: "claude-sonnet-4-20250514",
})

subAgent := agent.New[MyDeps, SubResult](claudeModel,
    agent.WithSystemPrompt[MyDeps, SubResult]("You are a specialist."),
)

// 主 Agent 的 tool 中委托给子 Agent
mainAgent := agent.New[MyDeps, MainResult](chatModel,
    agent.WithToolFunc[MyDeps, MainResult, DelegateArgs](
        "delegate", "Delegate to specialist",
        func(ctx *agent.RunContext[MyDeps], args DelegateArgs) (string, error) {
            result, err := subAgent.Run(ctx.Ctx, args.Question, ctx.Deps)
            if err != nil {
                return "", err
            }
            return result.Output.Summary, nil
        },
    ),
)
```

### 测试

```go
func TestOrderAgent(t *testing.T) {
    // TestModel 实现了 Eino ChatModel 接口
    tm := testutil.NewTestModel(
        testutil.TestResponse{
            ToolCalls: []*schema.ToolCall{{
                Function: schema.FunctionCall{
                    Name:      "get_order",
                    Arguments: `{"order_id":"ORD-1"}`,
                },
            }},
        },
        testutil.TestResponse{
            ToolCalls: []*schema.ToolCall{{
                Function: schema.FunctionCall{
                    Name:      "final_result",
                    Arguments: `{"order_id":"ORD-1","total_amount":99.9,"status":"completed","summary":"test"}`,
                },
            }},
        },
    )

    testAgent := myAgent.WithModel(tm)

    result, err := testAgent.Run(ctx, "查询订单", MyDeps{DB: mockDB, UserID: "test"})
    assert.NoError(t, err)
    assert.Equal(t, "ORD-1", result.Output.OrderID)
    assert.Equal(t, 2, tm.CallCount())
}
```

### 使用 MCP（直接用 Eino MCP）

```go
import einomcp "github.com/cloudwego/eino-ext/components/tool/mcp"

// 直接用 eino-ext 的 MCP 客户端获取 tools
mcpTools, _ := einomcp.NewToolsFromStdio(ctx, "python", "-m", "my_mcp_server")

myAgent := agent.New[agent.NoDeps, string](
    chatModel,
    agent.WithTool[agent.NoDeps, string](mcpTools...),
)
```

---

## 八、实现路径（分阶段）

### Phase 1：核心骨架（MVP）

**目标：** 跑通一个完整的结构化输出 Agent loop，基于 Eino ChatModel。

| 模块 | 交付物 | 依赖 Eino |
|------|--------|----------|
| `output/schema.go` | struct / 原始类型 / 切片 → JSON Schema 自动生成 | 无 |
| `output/tool_output.go` | "final_result" output tool 生成 | `components/tool` |
| `output/validator.go` | OutputValidator + ParseOutput | 无 |
| `agent/context.go` | `RunContext[D]` + context.Value 注入 | 无 |
| `agent/retry.go` | `ErrModelRetry` | 无 |
| `agent/agent.go` | `Agent[D,O]` 核心 + `Run()` + Agent Loop | `components/model.ChatModel`、`schema.Message` |
| `agent/options.go` | Option 模式 + `WithToolFunc` | `components/tool/utils.InferTool` |
| `agent/result.go` | `Result[O]` | `schema.Message` |
| `direct/direct.go` | `RequestText` / `Request[T]` 直接模型请求 | `components/model.ChatModel` |
| `testutil/` | TestModel + FunctionModel | `components/model.ChatModel` |

**验收标准：**
- 能用 Eino OpenAI ChatModel 创建 Agent，注册 tool，运行完整 loop，得到类型化输出
- "final_result" tool 模式正常工作（struct / int / bool / []string 等类型）
- OutputValidator + ErrModelRetry 重试正常
- `direct.RequestText` / `direct.Request[T]` 可用
- TestModel 可用于单元测试，不调用真实 API

### Phase 2：流式 + Native Output + 多轮对话 + Union 输出

| 模块 | 交付物 | 依赖 Eino |
|------|--------|----------|
| `agent/` | `RunStream()` + `StreamResult[O]` | `ChatModel.Stream()` |
| `output/native_output.go` | Native 模式（模型原生 structured output） | 模型 option |
| `agent/` | `RunWithHistory()` 多轮对话 | `schema.Message` |
| `agent/conversation.go` | `Conversation[D,O]` 多轮对话便捷封装 | `schema.Message` |
| `agent/union.go` | `OneOf2[A,B]` / `OneOf3[A,B,C]` Union 输出类型 | `components/tool` |
| `agent/` | UsageLimits 实现 | `schema.TokenUsage` |

**验收标准：**
- 流式输出逐 token 返回
- Native structured output 模式可用
- 多轮对话正常工作
- Conversation 可自动管理消息历史
- Union 输出类型（OneOf2/OneOf3）正确生成多个 output tool，Match 编译期安全

### Phase 3：高级 Tool 特性

| 模块 | 交付物 | 依赖 Eino |
|------|--------|----------|
| `agent/` | Deferred Tool（映射 Eino interrupt/resume） | `adk` interrupt |
| `agent/` | Dynamic Toolset（根据上下文暴露不同工具） | 无 |
| `agent/` | Tool `prepare` 回调（`PrepareFunc[D]` 动态裁剪参数 schema） | 无 |
| `agent/` | 多模态输入支持 | `schema.Message` multimodal |

**验收标准：**
- Deferred Tool 可暂停等待外部确认
- 动态 toolset 可根据依赖决定暴露哪些工具
- Tool prepare 可按运行时上下文动态修改参数 schema

### Phase 4：可观测性 + Eval 基础

| 模块 | 交付物 | 依赖 Eino |
|------|--------|----------|
| `agent/` | Callback 集成（适配 Eino Callback Handler） | `callbacks` |
| `eval/` | Dataset + Case + 基础 Evaluator（ExactMatch/Contains） | 无 |
| `eval/` | LLMJudge Evaluator | `components/model.ChatModel` |

### Phase 5（远期）：高级 Eval + Graph 增强

| 模块 | 交付物 | 依赖 Eino |
|------|--------|----------|
| `eval/` | Span-Based Evaluator、Report 输出 | `callbacks` |
| `agent/` | iter() 细粒度节点控制 | `compose.Graph` |

---

## 九、关键技术决策

### 9.1 依赖注入适配 Eino

**问题：** Eino Tool 的 `Run` 方法签名是 `func(ctx context.Context, args string) (string, error)`，没有泛型依赖参数。

**方案：** 通过 `context.WithValue` 桥接：

```go
// 定义 context key（使用泛型类型防止冲突）
type runContextKey[D any] struct{}

// 注入
func withRunContext[D any](ctx context.Context, rc *RunContext[D]) context.Context {
    return context.WithValue(ctx, runContextKey[D]{}, rc)
}

// 提取
func GetDeps[D any](ctx context.Context) (D, bool) {
    rc, ok := ctx.Value(runContextKey[D]{}).(*RunContext[D])
    if !ok {
        var zero D
        return zero, false
    }
    return rc.Deps, true
}
```

### 9.2 结构化输出 — "final_result" Tool 模式

参考 Pydantic AI 的 Tool Output 模式：
1. 从 `O` struct 自动生成 JSON Schema
2. 注册一个名为 `"final_result"` 的隐藏 tool
3. LLM 调用此 tool 时，agent loop 终止，解析其参数为 `O`
4. 如果验证失败，触发 retry

这是最通用的方案，适用于所有支持 function calling 的模型。

### 9.3 JSON Schema 生成

使用自定义的 `output.BuildParamsOneOf[T]()` 实现 schema 生成，支持以下 struct tag：

```go
type MyOutput struct {
    Name  string   `json:"name" description:"用户名称" required:"true"`
    Score int      `json:"score" description:"评分"`
    Tags  []string `json:"tags,omitempty" description:"标签列表"`
    Status string  `json:"status" description:"状态" enum:"active,inactive"`
}
// 也支持 jsonschema tag：`jsonschema:"required,description=xxx,enum=a,enum=b"`
// 注意：目前不支持 minimum/maximum tag
```

### 9.4 流式输出

底层直接使用 Eino 的 `schema.StreamReader[*schema.Message]`，上层封装为 `StreamResult[O]`：

```go
// Eino 层（已有）
stream, err := chatModel.Stream(ctx, messages, opts...)

// 我们封装的层
for chunk := range stream.Recv() {
    // 解析 chunk → TextStream / OutputStream
}
```

### 9.5 并发安全

- Agent 实例不可变（创建后配置不变），可安全并发使用
- RunContext 每次 Run 新建，不需要加锁
- Eino ChatModel 本身就是并发安全的

---

## 十、与 Pydantic AI 功能对照清单

| 功能 | Pydantic AI | 实现方式 | Phase |
|------|-------------|---------|-------|
| 泛型 Agent[D, O] | ✅ | ✅ 新建 `Agent[D, O any]` | P1 |
| Run / RunSync | ✅ | ✅ `Run()`，基于 Eino `ChatModel.Generate` | P1 |
| RunStream | ✅ | ✅ `RunStream()`，基于 Eino `ChatModel.Stream` | P2 |
| 结构化输出（struct） | ✅ | ✅ "final_result" tool 模式 | P1 |
| 结构化输出（原始类型） | ✅ | ✅ int/bool/float64/[]T 等 schema 自动生成 | P1 |
| 结构化输出（Union） | ✅ | ✅ `OneOf2[A,B]` / `OneOf3[A,B,C]` 多 output tool | P2 |
| 结构化输出（Native） | ✅ | ✅ 模型原生 JSON mode | P2 |
| 结构化输出（Prompted） | ✅ | ⚠️ 按需 | P3 |
| Output Validator | ✅ | ✅ `ValidatorFunc[O]` | P1 |
| 流式 partial 输出 | ✅ | ✅ 基于 Eino StreamReader | P2 |
| 依赖注入 RunContext | ✅ | ✅ `RunContext[D]` + context.Value | P1 |
| Tool 自动 Schema | ✅ | ✅ **复用 Eino `InferTool`** | P1 |
| Tool prepare 回调 | ✅ | ✅ `PrepareFunc[D]` 动态裁剪参数 schema | P3 |
| Toolset | ✅ | ✅ 动态 tool 列表 | P3 |
| Deferred Tool | ✅ | ✅ 映射 Eino ADK interrupt/resume | P3 |
| ModelRetry | ✅ | ✅ `ErrModelRetry` | P1 |
| 消息历史 / 多轮对话 | ✅ | ✅ `[]*schema.Message` 传入 | P2 |
| Conversation 对象 | ✅ | ✅ `Conversation[D,O]` 自动管理消息历史 | P2 |
| Direct Model Requests | ✅ | ✅ `direct.Request[T]` / `direct.RequestText` | P1 |
| 多 Agent 委托 | ✅ | ✅ Tool 内调用子 Agent | P1 |
| MCP Client | ✅ | ✅ **直接用 eino-ext MCP** | P1 |
| MCP Server | ✅ | ✅ **直接用 eino-ext MCP** | P1 |
| OpenTelemetry | ✅ | ✅ **复用 Eino Callbacks** + 增强 | P4 |
| TestModel | ✅ | ✅ 新建（实现 Eino ChatModel 接口） | P1 |
| FunctionModel | ✅ | ✅ 新建 | P1 |
| 多模态输入 | ✅ | ✅ **复用 Eino schema.Message** multimodal | P3 |
| Thinking 模式 | ✅ | ✅ **复用 Eino/eino-ext** Anthropic thinking | P2 |
| Embedding | ✅ | ✅ **直接用 Eino `components/embedding`** | - |
| Graph 引擎 | ✅ | ✅ **直接用 Eino `compose.Graph`** | - |
| 模型字符串解析 | ✅ | ✅ 封装 eino-ext Provider 创建 | P2 |
| Fallback Model | ✅ | ⚠️ 简单封装 | P2 |
| Eval 框架 | ✅ | ✅ 新建 `eval/` 包 | P4-P5 |
| Built-in Tools | ✅ | ❌ 不做，用户通过 Tool/MCP 自行接入 | - |
| 持久化执行 | ✅ | ❌ 不在范围 | - |
| UI 集成 | ✅ | ❌ 不在范围 | - |
| A2A 协议 | ✅ | ❌ 不在范围 | - |

**统计：约 35% 直接复用 Eino，约 45% 新建（核心差异化），约 20% 不在范围。**

---

## 十一、总结

### 技术策略

**"站在 Eino 肩膀上，做 Pydantic AI 体验层"**

- **Eino 提供**：模型抽象、Provider 实现、Tool 基础设施、流式处理、编排引擎、MCP、可观测回调
- **我们聚焦**：泛型 Agent[D,O]、结构化输出（struct/原始类型/Union）+ 自动验证、依赖注入 RunContext、Conversation、Direct Request、TestModel、Eval 框架

### 核心卖点

1. **泛型类型安全** — `Agent[D, O]` 编译期保证输入输出类型正确
2. **结构化输出 + 自动验证** — struct → JSON Schema → LLM → 反序列化 → 验证 → 自动重试，全自动
3. **依赖注入** — `RunContext[D]` 让 tool 和 system prompt 可以安全访问运行时依赖
4. **可测试** — TestModel 实现 Eino ChatModel 接口，零 API 调用单测
5. **不造轮子** — 模型、工具、MCP、编排全部复用 Eino 成熟实现
6. **可逃逸** — 用户随时可以降级到 Eino 原生 API，不被锁定

### 工作量估算

| Phase | 核心工作量 | 预估 |
|-------|-----------|------|
| P1 MVP | Agent[D,O] + 结构化输出（含原始类型）+ Direct Request + TestModel | 2-3 周 |
| P2 | 流式 + Native Output + 多轮 + Conversation + Union 输出 | 2-3 周 |
| P3 | Deferred Tool + Tool prepare + 多模态 | 1-2 周 |
| P4 | Eval 基础 + Callback | 2-3 周 |
| P5 | Eval 高级 | 2-3 周 |
