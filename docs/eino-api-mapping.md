# Eino API 适配手册

> 版本：v0.2（源码验证版）
> 日期：2026-03-18
> 目的：记录 cody-core-go 对接 Eino 的每个具体 API 调用点，确保设计假设与 Eino 实际接口一致
> 变更：v0.2 — 根据 Eino 源码验证修正接口签名、补充 ToolCallingChatModel、InferTool tag、ADK AsyncIterator 等

---

## 一、总览

本文档逐一列出 cody-core-go 需要使用的 Eino API，包括：
- **实际接口签名**（从 Eino 源码确认）
- **我们怎么用**（调用方式）
- **风险点 / 已知限制**

---

## 二、ChatModel 接口

### 2.1 接口定义（源码验证 ✅）

```go
// 来源：github.com/cloudwego/eino/components/model/interface.go

// BaseChatModel 定义所有 chat model 实现的核心接口
type BaseChatModel interface {
    Generate(ctx context.Context, input []*schema.Message, opts ...Option) (*schema.Message, error)
    Stream(ctx context.Context, input []*schema.Message, opts ...Option) (
        *schema.StreamReader[*schema.Message], error)
}

// ⚠️ ChatModel 已废弃 — BindTools 原地修改，并发不安全
// Deprecated: use ToolCallingChatModel instead
type ChatModel interface {
    BaseChatModel
    BindTools(tools []*schema.ToolInfo) error
}

// ✅ 推荐接口 — WithTools 返回新实例，并发安全
type ToolCallingChatModel interface {
    BaseChatModel
    WithTools(tools []*schema.ToolInfo) (ToolCallingChatModel, error)
}
```

**关键区别**：
- `ChatModel.BindTools` → 原地修改（多 goroutine 共享同一实例会 race）
- `ToolCallingChatModel.WithTools` → 返回新不可变实例（并发安全）
- **我们的策略**：接受 `BaseChatModel`，用 `model.WithTools()` Option 每次调用传 tools

### 2.2 我们怎么用

| 场景 | 调用方式 | 备注 |
|------|---------|------|
| Agent.Run() | `chatModel.Generate(ctx, messages, opts...)` | 同步调用 |
| Agent.RunStream() | `chatModel.Stream(ctx, messages, opts...)` | 流式调用 |
| 绑定工具 | `model.WithTools(toolInfos)` Option | **每次 Generate 时传入，不用 BindTools** |
| direct.RequestText | `chatModel.Generate(ctx, messages)` | 不绑定 tool |
| direct.Request[T] | `chatModel.Generate(ctx, messages, opts...)` | 绑定 output tool |

### 2.3 Option 机制（源码验证 ✅）

```go
// 来源：github.com/cloudwego/eino/components/model/option.go

type Option struct { /* internal */ }

// 已验证的 Option 构造函数：
func WithTemperature(temperature float32) Option
func WithMaxTokens(maxTokens int) Option
func WithModel(name string) Option
func WithTopP(topP float32) Option
func WithStop(stop []string) Option
func WithTools(tools []*schema.ToolInfo) Option          // ✅ 运行时传入 tools
func WithToolChoice(toolChoice schema.ToolChoice, allowedToolNames ...string) Option  // ✅ 控制 tool 选择

// Provider 特定选项（如 OpenAI 特有参数）
func WrapImplSpecificOptFn[T any](optFn func(*T)) Option
```

**注意**：
- `Option` 是 struct 类型，不是 `func(*Options)` — v0.1 的假设有误
- `WithTemperature` 接受 `float32`，不是 `*float64`
- `WithToolChoice` 支持 `ToolChoiceForbidden`/`ToolChoiceAllowed`/`ToolChoiceForced`

### 2.4 Tool prepare 与 WithTools 策略（已验证 ✅）

当 tool 有 `prepare` 回调时，tool schema 每次调用都可能不同。因此：
- **不能**使用 `BindTools` 一次绑定（已废弃且并发不安全）
- **必须**每次 `Generate` 时通过 `model.WithTools(toolInfos)` Option 传入当次的 tool schema 列表
- 可选配合 `model.WithToolChoice(schema.ToolChoiceForced)` 强制模型调用 output tool
- 实现策略：Agent Loop 每轮开始前，遍历 tools → 有 prepare 则调用 prepare → 收集最终 tool schema 列表 → 通过 Option 传入

---

## 三、Message 类型体系

### 3.1 核心类型（源码验证 ✅）

```go
// 来源：github.com/cloudwego/eino/schema/message.go

type Message struct {
    Role                     RoleType             `json:"role"`
    Content                  string               `json:"content"`
    MultiContent             []ChatMessagePart    `json:"multi_content,omitempty"`        // ⚠️ Deprecated
    UserInputMultiContent    []MessageInputPart   `json:"user_input_multi_content,omitempty"`   // ✅ 用户多模态输入
    AssistantGenMultiContent []MessageOutputPart  `json:"assistant_output_multi_content,omitempty"` // ✅ 助手多模态输出
    Name                     string               `json:"name,omitempty"`
    ToolCalls                []ToolCall           `json:"tool_calls,omitempty"`           // assistant only
    ToolCallID               string               `json:"tool_call_id,omitempty"`         // tool only
    ToolName                 string               `json:"tool_name,omitempty"`            // tool only（v0.1 遗漏）
    ResponseMeta             *ResponseMeta        `json:"response_meta,omitempty"`
    ReasoningContent         string               `json:"reasoning_content,omitempty"`    // 推理链内容（新发现）
    Extra                    map[string]any       `json:"extra,omitempty"`
}

type RoleType string
const (
    System    RoleType = "system"
    User      RoleType = "user"
    Assistant RoleType = "assistant"
    Tool      RoleType = "tool"
)

type ResponseMeta struct {
    FinishReason string      `json:"finish_reason,omitempty"`
    Usage        *TokenUsage `json:"usage,omitempty"`
    LogProbs     *LogProbs   `json:"logprobs,omitempty"`   // 新发现
}

type TokenUsage struct {
    PromptTokens            int                     `json:"prompt_tokens"`
    PromptTokenDetails      PromptTokenDetails      `json:"prompt_token_details"`
    CompletionTokens        int                     `json:"completion_tokens"`
    TotalTokens             int                     `json:"total_tokens"`
    CompletionTokensDetails CompletionTokensDetails `json:"completion_token_details"`
}
```

**v0.1 → v0.2 差异**：
- `MultiContent` 已废弃，用 `UserInputMultiContent` / `AssistantGenMultiContent` 替代
- `ToolName` 字段存在 — tool 消息可以携带 tool 名称
- `ReasoningContent` 字段 — 支持 thinking/reasoning 模型
- `TokenUsage` 有 `PromptTokenDetails` / `CompletionTokensDetails` 细分字段
- `ResponseMeta` 有 `LogProbs` 字段

### 3.2 我们怎么用

| 场景 | 构建方式 |
|------|---------|
| System prompt | `&schema.Message{Role: schema.System, Content: "..."}` |
| User prompt | `&schema.Message{Role: schema.User, Content: "..."}` |
| 多模态 User | `&schema.Message{Role: schema.User, MultiContent: [...]}` |
| Tool result 反馈 | `&schema.Message{Role: schema.Tool, Content: resultJSON, ToolCallID: "xxx"}` |
| 历史消息传入 | 直接用 `[]*schema.Message`，Eino 原生类型 |

### 3.3 ToolCall 类型（源码验证 ✅）

```go
// 来源：github.com/cloudwego/eino/schema/tool.go

type ToolCall struct {
    Index    *int           `json:"index,omitempty"`   // 流式 chunk 索引
    ID       string         `json:"id"`
    Type     string         `json:"type"`              // "function"
    Function FunctionCall   `json:"function"`
    Extra    map[string]any `json:"extra,omitempty"`   // v0.1 遗漏
}

type FunctionCall struct {
    Name      string `json:"name,omitempty"`
    Arguments string `json:"arguments,omitempty"`  // JSON 字符串
}
```

**注意**：`ToolCall.Extra` 可携带 provider 特定信息。

### 3.4 我们的适配要点

- **Tool result 消息格式**：每个 ToolCall 需要对应一个 `Role=Tool` 的 Message，`ToolCallID` 必须匹配
- **多个 ToolCall**：一个 assistant 消息可能包含多个 ToolCall，需逐个执行并返回对应的 tool message
- **output tool 识别**：检查 `ToolCall.Function.Name == "final_result"` 或 `"final_result_TypeName"` 来识别 output tool

---

## 四、Tool 系统

### 4.1 Tool 接口（源码验证 ✅）

```go
// 来源：github.com/cloudwego/eino/components/tool/interface.go

type BaseTool interface {
    Info(ctx context.Context) (*schema.ToolInfo, error)
}

// 标准 Tool — 入参 JSON string，出参 string
type InvokableTool interface {
    BaseTool
    InvokableRun(ctx context.Context, argumentsInJSON string, opts ...Option) (string, error)
}

type StreamableTool interface {
    BaseTool
    StreamableRun(ctx context.Context, argumentsInJSON string, opts ...Option) (
        *schema.StreamReader[string], error)
}

// ✅ Enhanced Tool — 入参 ToolArgument，出参 ToolResult（支持多模态）
type EnhancedInvokableTool interface {
    BaseTool
    InvokableRun(ctx context.Context, toolArgument *schema.ToolArgument, opts ...Option) (
        *schema.ToolResult, error)
}

type EnhancedStreamableTool interface {
    BaseTool
    StreamableRun(ctx context.Context, toolArgument *schema.ToolArgument, opts ...Option) (
        *schema.StreamReader[*schema.ToolResult], error)
}
```

**v0.1 → v0.2 差异**：发现 Enhanced 系列接口，支持多模态 tool 输入（`ToolArgument`）和输出（`ToolResult`）。我们 Phase 1 主要用标准 `InvokableTool`。

### 4.2 ToolInfo 类型（源码验证 ✅）

```go
// 来源：github.com/cloudwego/eino/schema/tool.go

type ToolChoice string
const (
    ToolChoiceForbidden ToolChoice = "forbidden"  // "none" in OpenAI
    ToolChoiceAllowed   ToolChoice = "allowed"    // "auto" in OpenAI
    ToolChoiceForced    ToolChoice = "forced"     // "required" in OpenAI
)

type ToolInfo struct {
    Name  string
    Desc  string
    Extra map[string]any    // v0.1 遗漏
    *ParamsOneOf            // 嵌入，不是简单的 JSONSchema 字段
}

// ParamsOneOf 支持两种方式定义参数 schema
type ParamsOneOf struct {
    // 内部支持两种来源（互斥）：
    //   1. params map[string]*ParameterInfo  — 通过 NewParamsOneOfByParams 创建
    //   2. jsonschema *jsonschema.Schema     — 通过 NewParamsOneOfByJSONSchema 创建
}

func NewParamsOneOfByParams(params map[string]*ParameterInfo) *ParamsOneOf
func NewParamsOneOfByJSONSchema(s *jsonschema.Schema) *ParamsOneOf
func (p *ParamsOneOf) ToJSONSchema() (*jsonschema.Schema, error)

type ParameterInfo struct {
    Type      DataType
    ElemInfo  *ParameterInfo              // 数组元素类型
    SubParams map[string]*ParameterInfo   // 嵌套 struct 字段
    Desc      string
    Enum      []string
    Required  bool
}

// Enhanced tool 的入出类型
type ToolArgument struct {
    Text string `json:"text,omitempty"`
}

type ToolResult struct {
    Parts []ToolOutputPart `json:"parts,omitempty"`
}
```

**v0.1 → v0.2 重大差异**：
- `ToolInfo.ParamsOneOf` 不是简单的 JSONSchema struct，而是一个包装类型
- 支持两种创建方式：手动定义 `ParameterInfo` map 或直接传 `jsonschema.Schema`
- 可用 `ToJSONSchema()` 方法统一转换
- 我们的 Output Tool schema 生成应该用 `NewParamsOneOfByJSONSchema`

### 4.3 InferTool 工具推断（源码验证 ✅）

```go
// 来源：github.com/cloudwego/eino/components/tool/utils/

// ✅ 泛型签名（v0.1 有误，现修正）
// T = 输入参数类型, D = 输出类型
type InvokeFunc[T, D any]  func(ctx context.Context, input T) (output D, err error)
type StreamFunc[T, D any]  func(ctx context.Context, input T) (output *schema.StreamReader[D], err error)

func InferTool[T, D any](toolName, toolDesc string, i InvokeFunc[T, D], opts ...Option) (tool.InvokableTool, error)
func InferStreamTool[T, D any](toolName, toolDesc string, s StreamFunc[T, D], opts ...Option) (tool.StreamableTool, error)

// 不做推断，直接传 ToolInfo
func NewTool[T, D any](desc *schema.ToolInfo, i InvokeFunc[T, D], opts ...Option) tool.InvokableTool
func NewStreamTool[T, D any](desc *schema.ToolInfo, s StreamFunc[T, D], opts ...Option) tool.StreamableTool

// ✅ 导出的 schema 生成函数（可直接复用！）
func GoStruct2ParamsOneOf[T any](opts ...Option) (*schema.ParamsOneOf, error)
func GoStruct2ToolInfo[T any](toolName, toolDesc string, opts ...Option) (*schema.ToolInfo, error)
```

**✅ InferTool 使用 `jsonschema` tag（不是 `description` tag）**：
```go
type SearchParams struct {
    Query string `json:"query" jsonschema:"required,description=search query"`
    Limit int    `json:"limit" jsonschema:"description=max results,enum=10,enum=20"`
}
```

**关键发现**：
- `GoStruct2ParamsOneOf[T]()` 和 `GoStruct2ToolInfo[T]()` 已导出（但我们最终采用了自定义 `output.BuildParamsOneOf[T]()` 以支持更多 tag 格式）
- tag 格式是 `jsonschema:"required,description=xxx,enum=10,enum=20"`
- 函数签名是 `func(ctx, T) (D, error)`，不是 `func(ctx, T) (string, error)` — InferTool 内部会 JSON 序列化 D

### 4.4 我们怎么用

**场景 1：用户直接传 Eino Tool**
```go
agent.WithTool[D, O](userEinoTool)
```

**场景 2：WithToolFunc 创建带依赖注入的 Tool**
```go
// 使用自定义 wrappedTool 实现
// 1. 用 output.BuildParamsOneOf[Args]() 生成参数 schema
// 2. 构建 wrappedTool，包装 fn：从 ctx 提取 RunContext[D]，json.Unmarshal 参数
// 3. 返回 tool.InvokableTool
```

**场景 3：Output Tool 生成**
```go
// 使用自定义 BuildParamsOneOf[O]() 生成 schema
paramsOneOf, err := output.BuildParamsOneOf[O]()
outTool := output.GenerateOutputTool[O](paramsOneOf)
// outTool 实现 tool.InvokableTool，名为 "final_result"
```

### 4.5 关键风险点（已更新）

| 风险 | 说明 | 应对 |
|------|------|------|
| ~~InferTool schema 推断能力~~ | ✅ 已验证：Eino 使用 `jsonschema` tag | 最终采用自定义 `output.BuildParamsOneOf`，同时支持 `description`/`jsonschema` tag |
| **InvokableRun 参数格式** | 参数以 `argumentsInJSON string` 传入，是原始 JSON 字符串 | 我们需要在 wrapper 里 `json.Unmarshal` 到 Args struct |
| **Tool result 格式** | 标准 InvokableRun 返回 `string`；InferTool 包装后自动序列化 D→string | 用 InferTool/NewTool 可返回任意类型 D |
| **jsonschema tag 兼容性** | 用户可能习惯 `description` tag 而非 `jsonschema` tag | 自定义实现同时支持两种 tag |

---

## 五、StreamReader / StreamWriter

### 5.1 接口定义（源码验证 ✅）

```go
// 来源：github.com/cloudwego/eino/schema/stream.go

type StreamReader[T any] struct { /* 内部有 5 种 reader 实现变体 */ }
func (sr *StreamReader[T]) Recv() (T, error)               // 返回 io.EOF 表示结束
func (sr *StreamReader[T]) Close()
func (sr *StreamReader[T]) Copy(n int) []*StreamReader[T]   // 复制为多消费者
func (sr *StreamReader[T]) SetAutomaticClose()              // 新发现：自动关闭

type StreamWriter[T any] struct { /* 内部 */ }
func (sw *StreamWriter[T]) Send(chunk T, err error) (closed bool)
func (sw *StreamWriter[T]) Close()

// Pipe 创建配对的 reader/writer
func Pipe[T any](cap int) (*StreamReader[T], *StreamWriter[T])

// 从数组创建 StreamReader
func StreamReaderFromArray[T any](arr []T) *StreamReader[T]
```

**v0.1 → v0.2 新发现**：
- `Pipe[T](cap)` — 可用于构建自定义流（如 Agent loop 内部的流式转发）
- `StreamReaderFromArray[T]` — 测试时很有用
- `StreamWriter.Send` 返回 `closed bool` — 可检测读端是否已关闭

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

### 6.1 接口定义（源码验证 ✅）

```go
// 来源：github.com/cloudwego/eino/callbacks/interface.go

type RunInfo struct {
    Name      string               // 节点名称（如 compose.WithNodeName 设置）
    Type      string               // 实现类型（如 "OpenAI"）
    Component components.Component  // 组件类型（如 ComponentOfChatModel）
}

type CallbackInput  = any
type CallbackOutput = any

// ✅ Handler 接口签名（v0.1 有误：方法返回 context.Context，不是 void）
type Handler interface {
    OnStart(ctx context.Context, info *RunInfo, input CallbackInput) context.Context
    OnEnd(ctx context.Context, info *RunInfo, output CallbackOutput) context.Context
    OnError(ctx context.Context, info *RunInfo, err error) context.Context
    OnStartWithStreamInput(ctx context.Context, info *RunInfo,
        input *schema.StreamReader[CallbackInput]) context.Context
    OnEndWithStreamOutput(ctx context.Context, info *RunInfo,
        output *schema.StreamReader[CallbackOutput]) context.Context
}

// TimingChecker 可选实现 — 按需启用特定 callback timing
type TimingChecker interface {
    Needed(ctx context.Context, info *RunInfo, timing CallbackTiming) bool
}

type CallbackTiming uint8
const (
    TimingOnStart CallbackTiming = iota
    TimingOnEnd
    TimingOnError
    TimingOnStartWithStreamInput
    TimingOnEndWithStreamOutput
)

// 全局 handler 注册（不线程安全，应在 init 时调用）
func AppendGlobalHandlers(handlers ...Handler)
```

**v0.1 → v0.2 重大差异**：
- Handler 方法返回 `context.Context`（不是 void）— 可在 callback 中注入 context
- 有流式专用 callback：`OnStartWithStreamInput` / `OnEndWithStreamOutput`
- `TimingChecker` 接口可选实现，按需启用 callback
- `RunInfo.Component` 是 `components.Component` 类型（不是 string）

### 6.2 我们怎么用

Phase 4 可观测性阶段：
- 在 Agent Loop 关键节点注入 callback 调用
- 复用 Eino 的 `callbacks.Handler` 接口
- Eino 的 ChatModel Provider 内部已自动触发 callbacks — 不需要重复触发

### 6.3 Eino 内置的 callback 传递

```go
ctx = callbacks.WithCallbacks(ctx, myHandler)
chatModel.Generate(ctx, messages, opts...)  // 内部自动触发 OnStart/OnEnd
```

---

## 七、ADK (Agent Development Kit)

### 7.1 Agent 接口（源码验证 ✅）

```go
// 来源：github.com/cloudwego/eino-ext/devops/adk/interface.go

type Message      = *schema.Message
type MessageStream = *schema.StreamReader[Message]

// ✅ Agent 接口 — Run 返回 AsyncIterator（不是简单的 return value）
type Agent interface {
    Name(ctx context.Context) string
    Description(ctx context.Context) string
    Run(ctx context.Context, input *AgentInput, options ...AgentRunOption) *AsyncIterator[*AgentEvent]
}

type ResumableAgent interface {
    Agent
    Resume(ctx context.Context, info *ResumeInfo, opts ...AgentRunOption) *AsyncIterator[*AgentEvent]
}

type AgentInput struct {
    Messages       []Message
    EnableStreaming bool
}

// ✅ AsyncIterator — 基于 channel 的异步迭代器
type AsyncIterator[T any] struct { /* wraps internal.UnboundedChan[T] */ }
func (ai *AsyncIterator[T]) Next() (T, bool)   // false = 迭代结束

type AsyncGenerator[T any] struct { /* wraps internal.UnboundedChan[T] */ }
func (ag *AsyncGenerator[T]) Send(v T)
func (ag *AsyncGenerator[T]) Close()

func NewAsyncIteratorPair[T any]() (*AsyncIterator[T], *AsyncGenerator[T])
```

### 7.2 AgentEvent 结构（源码验证 ✅）

```go
type AgentEvent struct {
    AgentName string
    RunPath   []RunStep
    Output    *AgentOutput
    Action    *AgentAction
    Err       error
}

type AgentOutput struct {
    MessageOutput    *MessageVariant
    CustomizedOutput any
}

type AgentAction struct {
    Exit             bool
    Interrupted      *InterruptInfo
    TransferToAgent  *TransferToAgentAction
    BreakLoop        *BreakLoopAction
    CustomizedAction any
}

type MessageVariant struct {
    IsStreaming    bool
    Message        Message
    MessageStream  MessageStream
    Role           schema.RoleType
    ToolName       string
}
```

### 7.3 ChatModelAgent 配置（源码验证 ✅）

```go
type ChatModelAgentConfig struct {
    Name             string
    Description      string
    Instruction      string              // system prompt
    Model            model.BaseChatModel
    ToolsConfig      ToolsConfig
    GenModelInput    GenModelInput        // 自定义模型输入构建
    Exit             tool.BaseTool        // 退出条件 tool
    OutputKey        string
    MaxIterations    int                  // 默认 20
    Middlewares      []AgentMiddleware
    Handlers         []ChatModelAgentMiddleware
    ModelRetryConfig *ModelRetryConfig
}

type ToolsConfig struct {
    compose.ToolsNodeConfig
    ReturnDirectly     map[string]bool    // tool 直接返回（不经过模型）
    EmitInternalEvents bool
}
```

### 7.4 我们为什么不直接用 ChatModelAgent

| 原因 | 说明 |
|------|------|
| 无泛型 | 输入输出都是 `*schema.Message` / `AgentEvent` |
| 无结构化输出 | 不会自动生成 output tool，不解析结构化结果 |
| 无验证重试 | 不支持 OutputValidator + ErrModelRetry |
| 无依赖注入 | 不支持 RunContext[D] |
| 无 output tool 终止逻辑 | 终止条件是 Exit tool / MaxIterations |

**结论**：我们自己实现 Agent Loop，但复用 Eino 底层组件（BaseChatModel、Tool、Message、StreamReader）。

### 7.5 Interrupt / Resume（源码验证 ✅）

```go
// 来源：github.com/cloudwego/eino-ext/devops/adk/interrupt.go

// 三种中断函数，返回包含中断动作的 *AgentEvent
func Interrupt(ctx context.Context, info any) *AgentEvent
func StatefulInterrupt(ctx context.Context, info any, state any) *AgentEvent
func CompositeInterrupt(ctx context.Context, info any, state any,
    subInterruptSignals ...*InterruptSignal) *AgentEvent

// 恢复执行通过 Runner
type Runner struct { /* Agent + CheckPointStore */ }
func NewRunner(_ context.Context, conf RunnerConfig) *Runner
func (r *Runner) Run(ctx context.Context, messages []Message, opts ...AgentRunOption) *AsyncIterator[*AgentEvent]
func (r *Runner) Resume(ctx context.Context, checkPointID string, opts ...AgentRunOption) (*AsyncIterator[*AgentEvent], error)
func (r *Runner) ResumeWithParams(ctx context.Context, checkPointID string,
    params *ResumeParams, opts ...AgentRunOption) (*AsyncIterator[*AgentEvent], error)

type ResumeInfo struct {
    EnableStreaming bool
    *InterruptInfo
    WasInterrupted bool
    InterruptState any
    IsResumeTarget bool
    ResumeData     any
}
```

**v0.1 → v0.2 重大差异**：
- `Interrupt` 不是返回 `error`，而是返回 `*AgentEvent`
- Resume 通过 `Runner` 进行，需要 `CheckPointStore`（gob 编码持久化）
- 支持 `StatefulInterrupt` 和 `CompositeInterrupt`（多层中断）
- **适配方案（Phase 3）**：Deferred Tool 内部用 `Interrupt` 发出事件，由我们的 Agent loop 捕获并暂停

### 7.6 Multi-Agent 支持

```go
// SetSubAgents 设置子 Agent，支持 Agent 间转移
func SetSubAgents(ctx context.Context, agent Agent, subAgents []Agent) (ResumableAgent, error)
func AgentWithOptions(ctx context.Context, agent Agent, opts ...AgentOption) Agent
func WithDisallowTransferToParent() AgentOption
func WithHistoryRewriter(h HistoryRewriter) AgentOption
```

**备注**：Multi-Agent 是 Phase 3+ 的功能，暂不需要实现。

### 7.7 ChatModelAgentMiddleware

```go
type ChatModelAgentMiddleware interface {
    BeforeAgent(ctx context.Context, runCtx *ChatModelAgentContext) (context.Context, *ChatModelAgentContext, error)
    BeforeModelRewriteState(ctx context.Context, state *ChatModelAgentState, mc *ModelContext) (context.Context, *ChatModelAgentState, error)
    AfterModelRewriteState(ctx context.Context, state *ChatModelAgentState, mc *ModelContext) (context.Context, *ChatModelAgentState, error)
    WrapInvokableToolCall(ctx context.Context, endpoint InvokableToolCallEndpoint, tCtx *ToolContext) (InvokableToolCallEndpoint, error)
    WrapModel(ctx context.Context, m model.BaseChatModel, mc *ModelContext) (model.BaseChatModel, error)
    // ... 还有 WrapStreamableToolCall, WrapEnhancedInvokableToolCall 等
}

// 提供无操作基类，方便只实现部分方法
type BaseChatModelAgentMiddleware struct{}
```

---

## 八、Eino JSON Schema 能力验证

### 8.1 验证结果（✅ 已完成）

| 问题 | 状态 | 结论 |
|------|------|------|
| InferTool 是否导出 schema 生成函数？ | ✅ | `GoStruct2ParamsOneOf[T]()` 和 `GoStruct2ToolInfo[T]()` 已导出 |
| 支持的 struct tag | ✅ | 使用 `jsonschema` tag：`required`/`description`/`enum` 已确认支持 |
| 嵌套 struct 支持 | ✅ | `ParameterInfo.SubParams` 支持嵌套 |
| 原始类型支持 | ✅ | `ParameterInfo.Type` 支持 DataType（各种基本类型） |
| 数组/切片支持 | ✅ | `ParameterInfo.ElemInfo` 支持数组元素类型递归定义 |

### 8.2 策略决定（实现更新）

**最终采用自定义 `output.BuildParamsOneOf[T]()` 实现**，没有直接复用 Eino 的 `utils.GoStruct2ParamsOneOf[T]()`。原因是需要同时支持 `description` tag 和 `jsonschema` tag，以及 `enum`、`required` 等自定义 tag。

```go
// 生成 output tool schema（使用自定义实现）
paramsOneOf, err := output.BuildParamsOneOf[O]()
if err != nil {
    return fmt.Errorf("build output schema: %w", err)
}

outTool := output.GenerateOutputTool[O](paramsOneOf)
```

**支持的 struct tag**：
- `json:"field_name,omitempty"` — JSON 字段名，omitempty 标记为非 required
- `description:"字段描述"` — 字段描述
- `required:"true"` — 强制标记为 required
- `enum:"a,b,c"` — 枚举值
- `jsonschema:"required,description=xxx,enum=a,enum=b"` — Eino 风格 jsonschema tag（也支持）

---

## 九、待验证清单

> v0.2 更新：大部分已通过源码验证完成

| # | 问题 | 验证方式 | 状态 |
|---|------|---------|------|
| 1 | BindTools vs Option 传 tools 的行为差异 | 源码验证 | ✅ BindTools 已废弃，用 `model.WithTools()` Option |
| 2 | Generate 返回的 Message 中 ToolCall.ID 是否自动生成 | Provider 源码 | ⚠️ 待验证（需读 OpenAI Provider 实现） |
| 3 | Stream 模式下 ToolCall 的 chunk 拼接方式 | 集成测试 | ⚠️ 待验证（需写 demo） |
| 4 | InferTool 的 schema 推断能力边界 | 源码验证 | ✅ 用 `jsonschema` tag，`GoStruct2ParamsOneOf` 已导出 |
| 5 | Eino 的 JSONSchema 类型字段 | 源码验证 | ✅ 用 `jsonschema.Schema` 类型（外部包），支持标准 JSON Schema |
| 6 | ADK interrupt/resume API | 源码验证 | ✅ Interrupt 返回 AgentEvent，Resume 通过 Runner |
| 7 | Eino callbacks context 传递 | 源码验证 | ✅ Handler 方法返回 context，支持注入 |
| 8 | model.Option 运行时传入 tools | 源码验证 | ✅ `model.WithTools(tools)` 和 `model.WithToolChoice(choice)` 已确认 |

---

## 十、ToolsNodeConfig（补充）

```go
// 来源：github.com/cloudwego/eino/compose/tool_node.go

type ToolsNodeConfig struct {
    Tools                []tool.BaseTool
    UnknownToolsHandler  func(ctx context.Context, name, input string) (string, error)
    ExecuteSequentially  bool                                                  // 顺序执行 tools（默认并行）
    ToolArgumentsHandler func(ctx context.Context, name, arguments string) (string, error)
    ToolCallMiddlewares  []ToolMiddleware
}
```

**备注**：`ExecuteSequentially` 控制多个 tool call 是并行还是顺序执行。我们的 Agent loop 默认并行执行 tool calls。
