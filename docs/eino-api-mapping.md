# Eino API 适配手册

> 版本：v0.1 Draft
> 日期：2026-03-17
> 目的：记录 cody-core-go 对接 Eino 的每个具体 API 调用点，确保设计假设与 Eino 实际接口一致

---

## 一、总览

本文档逐一列出 cody-core-go 需要使用的 Eino API，包括：
- **实际接口签名**（从 Eino 源码确认）
- **我们怎么用**（调用方式）
- **风险点 / 已知限制**

---

## 二、ChatModel 接口

### 2.1 接口定义（Eino 源码）

```go
// 来源：github.com/cloudwego/eino/components/model/interface.go

package model

import (
    "context"
    "github.com/cloudwego/eino/schema"
)

// BaseChatModel 定义所有 chat model 实现的核心接口
// 两种模式：
//   - Generate: 阻塞直到模型返回完整响应
//   - Stream: 返回 StreamReader，增量产出消息 chunk
type BaseChatModel interface {
    Generate(ctx context.Context, input []*schema.Message, opts ...Option) (*schema.Message, error)
    Stream(ctx context.Context, input []*schema.Message, opts ...Option) (*schema.StreamReader[*schema.Message], error)
}

// ChatModel 在 BaseChatModel 基础上增加 tool 绑定能力
type ChatModel interface {
    BaseChatModel
    BindTools(tools []*schema.ToolInfo) error  // 绑定 tool schema，后续调用自动携带
}
```

### 2.2 我们怎么用

| 场景 | 调用方式 | 备注 |
|------|---------|------|
| Agent.Run() | `chatModel.Generate(ctx, messages, opts...)` | 同步调用 |
| Agent.RunStream() | `chatModel.Stream(ctx, messages, opts...)` | 流式调用 |
| 绑定工具 | `chatModel.BindTools(toolInfos)` | **在 Agent 创建时调用一次** |
| direct.RequestText | `chatModel.Generate(ctx, messages)` | 不绑定 tool |
| direct.Request[T] | `chatModel.Generate(ctx, messages, opts...)` | 绑定 output tool |

### 2.3 Option 机制

```go
// Eino 的模型调用选项
type Option func(*Options)

type Options struct {
    Temperature     *float64
    MaxTokens       *int
    TopP            *float64
    Stop            []string
    Model           *string      // 运行时覆盖模型名
    Tools           []*schema.ToolInfo  // 运行时传入 tools（替代 BindTools）
    // ... 其他 provider 特定参数
}
```

**风险点：**
- BindTools 是"一次绑定"还是"每次 Generate 都要传"？需验证。
  - **确认方式**：如果 ChatModel 实现缓存了 tools，则 BindTools 调一次就行；否则需要每次通过 Option 传入。
  - **安全做法**：每次 Generate 时通过 `model.WithTools(toolInfos)` option 传入，不依赖 BindTools 的状态缓存。这样在 tool prepare 动态修改 schema 时也能正确工作。

### 2.4 Tool prepare 对 BindTools 的影响

当 tool 有 `prepare` 回调时，tool schema 每次调用都可能不同。因此：
- **不能**使用 `BindTools` 一次绑定
- **必须**每次 `Generate` 时通过 Option 传入当次的 tool schema 列表
- 实现策略：Agent Loop 每轮开始前，遍历 tools → 有 prepare 则调用 prepare → 收集最终 tool schema 列表 → 通过 Option 传入

---

## 三、Message 类型体系

### 3.1 核心类型（Eino 源码）

```go
// 来源：github.com/cloudwego/eino/schema/message.go

// Message 是对话中的一条消息
type Message struct {
    Role       RoleType        // system / user / assistant / tool
    Content    string          // 文本内容
    MultiContent []ChatMessagePart  // 多模态内容（图片、音频等）
    Name       string          // 可选，消息名称
    ToolCalls  []ToolCall      // assistant 消息中的 tool 调用
    ToolCallID string          // tool 消息中，关联的 tool call ID

    // Token 用量
    ResponseMeta *ResponseMeta
}

type RoleType string

const (
    System    RoleType = "system"
    User      RoleType = "user"
    Assistant RoleType = "assistant"
    Tool      RoleType = "tool"
)

// ResponseMeta 包含模型响应的元信息
type ResponseMeta struct {
    Usage    *TokenUsage
    FinishReason string
}

type TokenUsage struct {
    PromptTokens     int
    CompletionTokens int
    TotalTokens      int
}
```

### 3.2 我们怎么用

| 场景 | 构建方式 |
|------|---------|
| System prompt | `&schema.Message{Role: schema.System, Content: "..."}` |
| User prompt | `&schema.Message{Role: schema.User, Content: "..."}` |
| 多模态 User | `&schema.Message{Role: schema.User, MultiContent: [...]}` |
| Tool result 反馈 | `&schema.Message{Role: schema.Tool, Content: resultJSON, ToolCallID: "xxx"}` |
| 历史消息传入 | 直接用 `[]*schema.Message`，Eino 原生类型 |

### 3.3 ToolCall 类型

```go
// 来源：github.com/cloudwego/eino/schema/tool.go

type ToolCall struct {
    ID       string        // tool call 的唯一 ID
    Type     string        // "function"
    Function FunctionCall
    Index    *int          // 流式时的 chunk 索引
}

type FunctionCall struct {
    Name      string  // tool 名称
    Arguments string  // JSON 字符串形式的参数
}
```

### 3.4 我们的适配要点

- **Tool result 消息格式**：每个 ToolCall 需要对应一个 `Role=Tool` 的 Message，`ToolCallID` 必须匹配
- **多个 ToolCall**：一个 assistant 消息可能包含多个 ToolCall，需逐个执行并返回对应的 tool message
- **output tool 识别**：检查 `ToolCall.Function.Name == "final_result"` 或 `"final_result_TypeName"` 来识别 output tool

---

## 四、Tool 系统

### 4.1 Tool 接口（Eino 源码）

```go
// 来源：github.com/cloudwego/eino/components/tool/interface.go

// BaseTool 工具的基础接口
type BaseTool interface {
    Info(ctx context.Context) (*schema.ToolInfo, error)  // 返回 tool 的 schema 信息
}

// InvokableTool 可调用的工具
type InvokableTool interface {
    BaseTool
    InvokableRun(ctx context.Context, argumentsInJSON string, opts ...Option) (string, error)
}

// StreamableTool 可流式调用的工具
type StreamableTool interface {
    BaseTool
    StreamableRun(ctx context.Context, argumentsInJSON string, opts ...Option) (*schema.StreamReader[string], error)
}
```

### 4.2 ToolInfo 类型

```go
// 来源：github.com/cloudwego/eino/schema/tool.go

type ToolInfo struct {
    Name        string     // tool 名称
    Desc        string     // tool 描述
    ParamsOneOf *JSONSchema // 参数的 JSON Schema
}

// JSONSchema 表示 JSON Schema
type JSONSchema struct {
    Type        DataType               `json:"type"`
    Description string                 `json:"description,omitempty"`
    Properties  map[string]*JSONSchema `json:"properties,omitempty"`
    Required    []string               `json:"required,omitempty"`
    Items       *JSONSchema            `json:"items,omitempty"`
    Enum        []any                  `json:"enum,omitempty"`
    // ... 其他标准 JSON Schema 字段
}
```

### 4.3 InferTool 工具推断

```go
// 来源：github.com/cloudwego/eino/components/tool/utils/

// InferTool 从函数签名自动推断 tool schema
// fn 签名：func(ctx context.Context, args T) (string, error)
// T 必须是 struct，其字段自动生成 JSON Schema
func InferTool[T any](name, desc string, fn func(ctx context.Context, args T) (string, error)) (InvokableTool, error)
```

### 4.4 我们怎么用

**场景 1：用户直接传 Eino Tool**
```go
// 用户用 InferTool 创建，我们直接存
agent.WithTool[D, O](userEinoTool)
```

**场景 2：WithToolFunc 创建带依赖注入的 Tool**
```go
// 我们内部需要：
// 1. 从 Args struct 生成 JSON Schema（复用 Eino 的 schema 推断 或 自己实现）
// 2. 包装 fn，将 RunContext[D] 从 ctx 中提取出来
// 3. 构造一个实现 InvokableTool 接口的对象
```

**场景 3：Output Tool 生成**
```go
// 我们内部生成 final_result tool：
// 1. 从 O struct 生成 JSON Schema
// 2. 创建一个 InvokableTool，其 InvokableRun 不实际执行，只用于解析参数
// 3. Info() 返回 ToolInfo{Name: "final_result", Desc: "...", ParamsOneOf: schema}
```

### 4.5 关键风险点

| 风险 | 说明 | 应对 |
|------|------|------|
| **InferTool 的 schema 推断能力** | InferTool 能识别哪些 struct tag？`json`、`description` 是否支持？`enum`、`minimum` 等扩展 tag？ | 需要读 InferTool 源码确认。如果不支持 `description` 等扩展 tag，我们需要自己实现 SchemaFor[T] |
| **InvokableRun 参数格式** | 参数以 `argumentsInJSON string` 传入，是原始 JSON 字符串 | 我们需要在 wrapper 里 `json.Unmarshal` 到 Args struct |
| **Tool result 格式** | InvokableRun 返回 `string`，不是结构体 | Tool 函数需要返回 string（通常是 JSON 序列化后的字符串） |

---

## 五、StreamReader

### 5.1 接口定义（Eino 源码）

```go
// 来源：github.com/cloudwego/eino/schema/stream.go

// StreamReader 泛型流式读取器
type StreamReader[T any] struct {
    // 内部实现
}

// Recv 接收下一个 chunk，返回 io.EOF 表示结束
func (r *StreamReader[T]) Recv() (T, error)

// Close 关闭流
func (r *StreamReader[T]) Close()

// Copy 复制流为多个（用于多消费者场景）
func (r *StreamReader[T]) Copy(n int) []*StreamReader[T]
```

### 5.2 我们怎么用

```go
// Agent.RunStream 内部
stream, err := chatModel.Stream(ctx, messages, opts...)
// 包装为 StreamResult[O]

// StreamResult.TextStream() 实现
func (s *StreamResult[O]) TextStream() <-chan string {
    ch := make(chan string)
    go func() {
        defer close(ch)
        for {
            chunk, err := s.stream.Recv()
            if err == io.EOF {
                return
            }
            if chunk.Content != "" {
                ch <- chunk.Content
            }
        }
    }()
    return ch
}
```

### 5.3 流式 Tool Call 处理

**关键问题**：流式模式下，模型返回 tool call 时：
- ToolCall 的 Arguments 可能分多个 chunk 到达
- 需要拼接完整的 Arguments JSON 后才能执行 tool
- Eino 的 StreamReader 是否自动拼接？还是需要我们手动拼接？

**应对策略**：
1. 收集所有 chunk，拼接完整的 assistant message
2. 检查是否包含 ToolCall
3. 如果是 tool call → 执行 tool → 继续 agent loop
4. 如果是纯文本 → 逐 chunk 推送给用户

---

## 六、Callbacks 系统

### 6.1 接口定义（Eino 源码）

```go
// 来源：github.com/cloudwego/eino/callbacks/

// Handler 回调处理器接口
type Handler interface {
    OnStart(ctx context.Context, info *RunInfo, input CallbackInput)
    OnEnd(ctx context.Context, info *RunInfo, output CallbackOutput)
    OnError(ctx context.Context, info *RunInfo, err error)
}

// RunInfo 包含当前执行节点的信息
type RunInfo struct {
    Name      string            // 节点名称
    Type      string            // 节点类型 (ChatModel/Tool/...)
    Component string            // 组件类型
    Extra     map[string]any    // 额外信息
}
```

### 6.2 我们怎么用

在 Phase 4 的可观测性阶段：
- 在 Agent Loop 的关键节点注入 callback 调用
- 复用 Eino 的 `callbacks.Handler` 接口，用户可以传入自己的 Handler
- 不重写 callback 机制，只在我们的层面触发事件

### 6.3 Eino 内置的 callback 传递

```go
// Eino 通过 context 传递 callbacks
ctx = callbacks.WithCallbacks(ctx, myHandler)
chatModel.Generate(ctx, messages, opts...)  // 内部自动触发 OnStart/OnEnd
```

**关键发现**：Eino 的 ChatModel 实现（如 openai Provider）内部已经会触发 callbacks。我们不需要在 Agent 层再手动触发模型调用的 callback，只需要触发 Agent 层特有的事件（如 retry、output validation 等）。

---

## 七、ADK (Agent Development Kit)

### 7.1 ChatModelAgent

```go
// 来源：github.com/cloudwego/eino-ext/devops/adk/

// ChatModelAgent 是 Eino ADK 提供的 ReAct loop Agent
// 它已经实现了：模型调用 → tool 执行 → 模型再调用 的循环
type ChatModelAgent struct {
    // ...
}

// Run 执行 agent loop
func (a *ChatModelAgent) Run(ctx context.Context, input *schema.Message) (*schema.Message, error)
```

### 7.2 我们为什么不直接用 ChatModelAgent

| 原因 | 说明 |
|------|------|
| 无泛型 | 输入输出都是 `*schema.Message`，不是类型化的 |
| 无结构化输出 | 不会自动生成 output tool，不解析结构化结果 |
| 无验证重试 | 不支持 OutputValidator + ErrModelRetry |
| 无依赖注入 | 不支持 RunContext[D] |
| 无 output tool 终止逻辑 | agent loop 的终止条件不含"调用了 output tool" |

**结论**：我们需要**自己实现 Agent Loop**，但复用 Eino 的底层组件（ChatModel、Tool、Message）。

### 7.3 Interrupt / Resume（Deferred Tool 映射）

```go
// ADK 的 interrupt 机制
// Agent 可以在 tool 执行时暂停，等待外部输入后恢复

// Interrupt 发出中断请求
func Interrupt(ctx context.Context, payload any) error

// Resume 在中断点恢复执行
func Resume(ctx context.Context, resumeData any) error
```

**适配方案（Phase 3）**：
- Deferred Tool 内部调用 `Interrupt(ctx, prompt)` 暂停 agent loop
- 外部（如 HTTP handler）获取到暂停状态后，收集用户输入
- 调用 `Resume(ctx, userInput)` 恢复执行
- 具体 API 需进一步验证 ADK 源码

---

## 八、Eino JSON Schema 能力验证

### 8.1 需要验证的问题

我们的 `output/schema.go` 需要从 Go struct 生成 JSON Schema。Eino 的 InferTool 内部已有此能力。

**需要确认：**

| 问题 | 状态 | 说明 |
|------|------|------|
| InferTool 是否导出 schema 生成函数？ | ⚠️ 待验证 | 如果导出了可以直接复用；否则需自己实现 |
| 支持的 struct tag | ⚠️ 待验证 | `json` 肯定支持；`description`、`required`、`enum`、`minimum`、`maximum` 待确认 |
| 嵌套 struct 支持 | ⚠️ 待验证 | 是否支持嵌套 struct 递归生成 schema |
| 原始类型支持 | ⚠️ 待验证 | InferTool 只支持 struct 参数？还是也支持 int、bool 等？ |
| 数组/切片支持 | ⚠️ 待验证 | `[]string`、`[]MyStruct` 是否正确生成 array schema |

### 8.2 备选方案

如果 Eino 的 schema 生成不满足需求（不支持 `description` tag 等），我们自己实现 `SchemaFor[T]()` 也不复杂：

```go
func SchemaFor[T any]() (*schema.JSONSchema, error) {
    t := reflect.TypeOf((*T)(nil)).Elem()
    return structToSchema(t)
}

func structToSchema(t reflect.Type) (*schema.JSONSchema, error) {
    // 处理原始类型
    switch t.Kind() {
    case reflect.Int, reflect.Int64:
        return &schema.JSONSchema{Type: "integer"}, nil
    case reflect.Float64:
        return &schema.JSONSchema{Type: "number"}, nil
    case reflect.Bool:
        return &schema.JSONSchema{Type: "boolean"}, nil
    case reflect.String:
        return &schema.JSONSchema{Type: "string"}, nil
    case reflect.Slice:
        items, _ := structToSchema(t.Elem())
        return &schema.JSONSchema{Type: "array", Items: items}, nil
    case reflect.Struct:
        // 遍历字段，读取 json/description/required/enum 等 tag
        // ...
    }
}
```

工作量：约 150-200 行代码，可控。

---

## 九、待验证清单

> 实现前必须逐一验证

| # | 问题 | 验证方式 | 状态 |
|---|------|---------|------|
| 1 | BindTools vs Option 传 tools 的行为差异 | 读 OpenAI Provider 源码 | ⚠️ |
| 2 | Generate 返回的 Message 中 ToolCall.ID 是否自动生成 | 读 Provider 源码 | ⚠️ |
| 3 | Stream 模式下 ToolCall 的 chunk 拼接方式 | 写 demo 测试 | ⚠️ |
| 4 | InferTool 的 schema 推断能力边界 | 读 InferTool 源码 | ⚠️ |
| 5 | Eino 的 JSONSchema 类型是否有 `Minimum`/`Maximum`/`Default` 等字段 | 读 schema 源码 | ⚠️ |
| 6 | ADK interrupt/resume 的具体 API 和生命周期 | 读 ADK 源码 | ⚠️ |
| 7 | Eino callbacks 是否通过 context 自动传递到 ChatModel 内部 | 读 callbacks 源码 | ⚠️ |
| 8 | model.Option 是否支持运行时传入 tools | 读 model option 源码 | ⚠️ |
