# Pydantic AI 行为规格文档

> 版本：v0.1 Draft
> 日期：2026-03-17
> 目的：精确记录 Pydantic AI 每个核心能力的行为细节、边缘 case、错误处理，作为 cody-core-go 实现的行为参照

---

## 一、Agent Run 生命周期

### 1.1 基本流程

```
agent.run(prompt, deps)
│
├── 1. 创建 RunContext（deps + usage tracker + retry count）
├── 2. 构建 system prompt（静态 + 动态）
├── 3. 准备 tools 列表（用户 tools + output tool）
├── 4. 构建初始 messages
│     ├── system messages
│     ├── message_history（如有）
│     └── user prompt message
│
├── 5. ┌─── Model Loop ───────────────────────┐
│     │  调用模型                              │
│     │  ├── 纯文本响应                        │
│     │  │   ├── O=str → 作为最终结果返回       │
│     │  │   └── O≠str → 作为 output tool 的   │
│     │  │       参数尝试 JSON 解析             │
│     │  ├── Tool Call = output tool            │
│     │  │   → 解析参数 → 验证 → 返回           │
│     │  └── Tool Call = 普通 tool              │
│     │       → 执行 tool → 追加结果到 messages  │
│     │       → 回到循环顶部                     │
│     └────────────────────────────────────────┘
│
└── 6. 返回 RunResult（output + messages + usage）
```

### 1.2 终止条件

Agent Loop 在以下情况终止：

| 条件 | 行为 |
|------|------|
| 模型返回纯文本且 `O=str` | 直接返回文本作为 output |
| 模型调用了 output tool（`final_result`） | 解析参数为 O，验证通过后返回 |
| 达到 max retries | 返回 `UnexpectedModelBehavior` 错误 |
| 达到 usage limits（token/request） | 返回 `UsageLimitExceeded` 错误 |
| 模型返回纯文本但 `O≠str` | **尝试 JSON 解析文本内容**；如失败，触发 retry |

### 1.3 关键行为确认

**Q: 模型同时返回 output tool call 和普通 tool call 怎么办？**

A: Pydantic AI 的处理逻辑：
1. 先检查所有 tool calls
2. 如果其中有 output tool → **忽略其他 tool calls**，直接取 output tool 的结果
3. output tool 的结果进入验证流程

**Q: 模型返回多个普通 tool call 怎么办？**

A: **并行执行**所有 tool calls（Python 中用 `asyncio.gather`）。所有 tool 结果收集后一起追加到 messages，再进入下一轮模型调用。

**Q: 模型返回空响应（无文本、无 tool call）怎么办？**

A: 视为 unexpected model behavior，触发 retry 或返回错误。

---

## 二、Retry 机制

### 2.1 重试计数

```python
# Pydantic AI 的 retry 计数逻辑
max_retries = 1  # 默认值

# retry 计数器是 per-run 的，不是 per-tool
# 每次 ModelRetry 或 validation 失败，计数器 +1
# 达到 max_retries 后抛出异常
```

**关键细节：**

| 行为 | 说明 |
|------|------|
| 默认 max_retries | **1**（即最多重试 1 次，总共调用模型 2 次） |
| 计数范围 | **per-run**，整个 run 共享一个计数器 |
| Tool ModelRetry | 也消耗 retry 计数 |
| Output Validation 失败 | 也消耗 retry 计数 |
| 普通 tool call 成功 | **不消耗** retry 计数（tool call → tool result 是正常流程） |
| 计数器重置 | 每次 `agent.run()` 重新开始，计数器从 0 开始 |

### 2.2 重试时的消息反馈

当 retry 触发时，以下消息被追加到对话中：

**Tool ModelRetry（tool 函数返回 ModelRetry）：**
```python
# 追加的消息：
# 1. assistant message（包含 tool call）—— 已经在对话中
# 2. tool result message（包含 retry 的 error message）
messages.append(ToolReturn(
    tool_call_id=tool_call.id,
    content=retry_error.message,  # 反馈给模型的修正提示
))
# 3. 回到模型调用，模型看到 tool 返回了错误，会尝试修正
```

**Output Validation 失败：**
```python
# 追加的消息：
# 1. assistant message（包含 output tool call）—— 已经在对话中
# 2. retry prompt message（告诉模型验证失败的原因）
messages.append(RetryPrompt(
    content=f"Validation error: {error_message}",
    tool_call_id=output_tool_call.id,  # 关联到 output tool call
))
# 3. 回到模型调用
```

### 2.3 超过重试次数

```python
# 达到 max_retries 后：
raise UnexpectedModelBehavior(
    f"Exceeded max retries ({max_retries}) ..."
)
```

在 Go 中映射为：
```go
var ErrMaxRetriesExceeded = errors.New("max retries exceeded")

type MaxRetriesError struct {
    MaxRetries int
    LastError  error  // 最后一次失败的原因
}
```

---

## 三、结构化输出（Output Tool 模式）

### 3.1 Output Tool 生成规则

| 场景 | Output Tool 名称 | 数量 |
|------|-----------------|------|
| `output_type=MyStruct` | `"final_result"` | 1 个 |
| `output_type=str` | 无（不生成 output tool） | 0 |
| `output_type=int/bool/float` | `"final_result"` | 1 个 |
| `output_type=Success \| Failure` | `"final_result_success"`, `"final_result_failure"` | N 个 |

### 3.2 Output Tool 的 schema 生成

```python
# 对于 output_type=MyStruct：
{
    "name": "final_result",
    "description": "The final response which ends this conversation",
    "parameters": {
        "type": "object",
        "properties": {
            "field1": {"type": "string", "description": "..."},
            "field2": {"type": "integer", "description": "..."}
        },
        "required": ["field1", "field2"]
    }
}

# 对于 output_type=int：
{
    "name": "final_result",
    "description": "The final response which ends this conversation",
    "parameters": {
        "type": "object",
        "properties": {
            "result": {"type": "integer"}
        },
        "required": ["result"]
    }
}
# 注意：原始类型会被包装在 {"result": value} 中
```

### 3.3 Output Tool 的 description

Pydantic AI 生成的 output tool description 包含类型信息：
```
"The final response which ends this conversation"
```

如果有自定义 description（通过 `output_type` 的 docstring），会追加到 tool description 中。

### 3.4 Union 输出类型的 tool 命名

```python
# output_type=Success | Failure
# 生成两个 tool：

# Tool 1:
{
    "name": "final_result_success",
    "description": "...",
    "parameters": { /* Success 的 schema */ }
}

# Tool 2:
{
    "name": "final_result_failure",
    "description": "...",
    "parameters": { /* Failure 的 schema */ }
}
```

命名规则：`"final_result_" + 类型名的 snake_case 形式`

**Q: 模型调用了一个不存在的 output tool 名怎么办？**
A: 视为普通 tool call not found → 返回 error 反馈给模型 → 触发 retry。

### 3.5 Plain Text Fallback

当 `output_type≠str` 但模型返回纯文本时：

```python
# Pydantic AI 的处理：
# 1. 尝试将文本作为 JSON 解析
# 2. 如果成功 → 反序列化为 O 类型
# 3. 如果失败 → 触发 retry，反馈给模型需要调用 output tool
```

**反馈消息：**
```
"Plain text responses are not permitted, please call one of the functions instead."
```

---

## 四、Tool 执行

### 4.1 Tool 函数签名

```python
@agent.tool
async def my_tool(ctx: RunContext[MyDeps], arg1: str, arg2: int) -> str:
    # ctx.deps 可用
    return "result"
```

### 4.2 Tool 返回值处理

| 返回值 | 处理 |
|--------|------|
| `str` | 直接作为 tool result |
| 其他类型（int, dict, dataclass...） | 自动 JSON 序列化为 string |
| `ModelRetry("message")` | 特殊处理：不作为成功 result，而是触发 retry |
| 异常 | 反馈 error 信息给模型（不是直接崩溃） |

### 4.3 Tool 执行顺序

- 同一轮中的多个 tool calls：**并行执行**
- 不同轮的 tool calls：**顺序执行**（等上一轮的 tool results 返回后才进入下一轮）

### 4.4 Tool Result 消息格式

```python
# 每个 tool call 对应一个 tool result 消息
ToolReturn(
    tool_name="search",
    tool_call_id="call_xxx",  # 与 ToolCall.id 匹配
    content="search result text here",
)
```

### 4.5 Tool Error 处理

```python
# 如果 tool 函数抛出异常（非 ModelRetry）：
ToolReturn(
    tool_name="search",
    tool_call_id="call_xxx",
    content=f"Error running tool: {error_message}",
    # 注意：这个 error 是作为正常的 tool result 返回给模型的
    # 模型会看到这个 error 并决定如何处理
)
# 不消耗 retry 计数
```

**关键区别：**
- **Tool 抛普通异常** → 作为 tool error result 反馈给模型，不消耗 retry，模型可以重试或换方案
- **Tool 抛 ModelRetry** → 作为 retry prompt 反馈给模型，**消耗 retry 计数**

---

## 五、System Prompt

### 5.1 静态 System Prompt

```python
agent = Agent('model', system_prompt="You are a helpful assistant.")
# 或多个：
agent = Agent('model', system_prompt=[
    "You are a helpful assistant.",
    "Always respond in Chinese.",
])
```

多个 system prompt → **合并为一个 system message**（用换行分隔），还是**多个 system message**？
- Pydantic AI：**多个独立的 system message**

### 5.2 动态 System Prompt

```python
@agent.system_prompt
async def dynamic_prompt(ctx: RunContext[MyDeps]) -> str:
    return f"Current user: {ctx.deps.user_name}"
```

**执行时机**：每次 `agent.run()` 开始时调用，在模型调用之前。
**顺序**：按注册顺序依次调用，结果追加到 system messages 列表后面。

### 5.3 System Prompt 顺序

```
最终的 system messages:
1. 静态 system prompt(s)     ← 按声明顺序
2. 动态 system prompt(s)     ← 按注册顺序
3. (output tool 的使用说明)   ← 框架自动追加（如果有 output tool）
```

---

## 六、消息历史 (Message History)

### 6.1 new_messages() vs all_messages()

```python
result = await agent.run("hello", message_history=old_messages)

# result.new_messages()
# → 只返回这次 run 产生的新消息（不含 message_history 和 system messages）
# → 包含：user prompt + 模型响应 + tool calls + tool results + final output

# result.all_messages()
# → 返回完整消息序列（含 message_history，但不含 system messages）
# → = message_history + new_messages()
```

**关键：system messages 永远不包含在 `new_messages()` 或 `all_messages()` 中。**
- 原因：system prompt 可能是动态的，每次 run 不一样，历史中的 system prompt 没意义
- 用户传 message_history 时，不需要（也不应该）包含 system messages

### 6.2 message_history 合并逻辑

```python
# agent.run() 内部构建的完整 messages：
messages = [
    *system_messages,            # 1. system prompts（不来自 history）
    *message_history,            # 2. 历史消息（由用户传入）
    UserPrompt(content=prompt),  # 3. 当前用户 prompt
]
```

### 6.3 多轮对话的标准模式

```python
# 第一轮
r1 = await agent.run("你好")
# r1.all_messages() = [UserPrompt("你好"), ModelResponse("你好！")]

# 第二轮
r2 = await agent.run("我叫什么？", message_history=r1.all_messages())
# 内部 messages: [system, UserPrompt("你好"), ModelResponse("你好！"), UserPrompt("我叫什么？")]
# r2.all_messages() = [UserPrompt("你好"), ModelResponse("你好！"), UserPrompt("我叫什么？"), ModelResponse("...")]
```

---

## 七、Usage Tracking

### 7.1 Usage Limits

```python
from pydantic_ai import UsageLimits

result = await agent.run("hello", usage_limits=UsageLimits(
    request_limit=10,     # 最多调用模型 10 次
    request_tokens_limit=1000,  # 最多消耗 1000 prompt tokens
    response_tokens_limit=500,  # 最多产出 500 completion tokens
    total_tokens_limit=1500,    # 总 token 上限
))
```

### 7.2 超限行为

- 在**每次模型调用前**检查 usage limits
- 如果已超限 → 抛出 `UsageLimitExceeded` 错误
- 不会在调用中途中断（一次完整调用不会被截断）

### 7.3 Usage 记录

```python
result.usage()
# → Usage(
#     requests=3,              # 模型调用次数
#     request_tokens=500,      # 总 prompt tokens
#     response_tokens=200,     # 总 completion tokens
#     total_tokens=700,        # 总 tokens
#     details={...}            # provider 特定信息
# )
```

---

## 八、Streaming 行为

### 8.1 流式 + 结构化输出

当 `output_type≠str` 时的流式行为：

1. 模型开始生成 output tool call 的参数
2. 参数 JSON 逐 token 到达
3. **部分 JSON 解析**（partial parsing）：
   - 利用 partial JSON parser 在不完整的 JSON 上尝试解析
   - 每收到新 chunk 尝试解析一次
   - 成功解析出部分字段 → 产出 partial O
4. 最终完整 JSON 到达 → 完整解析 + 验证

### 8.2 流式 + Tool Call

流式模式下的 tool call 处理：

1. 流式接收 assistant 消息
2. 检测到 tool call → 停止文本推送
3. 等待 tool call 的 arguments 完整到达
4. 执行 tool → 获取结果
5. 继续模型调用（新一轮可能仍然是流式的）

### 8.3 流式中断

如果流式过程中 context 被取消或出错：
- StreamReader 的 `Recv()` 返回 error
- 用户应调用 `Close()` 清理资源
- 已接收到的 partial 数据可能不完整

---

## 九、Model Settings

### 9.1 设置覆盖优先级

```python
agent = Agent('model',
    model_settings=ModelSettings(temperature=0.5),  # Agent 级别
)

result = await agent.run("hello",
    model_settings=ModelSettings(temperature=0.9),  # Run 级别覆盖
)
```

**优先级：Run 级别 > Agent 级别 > Provider 默认值**

### 9.2 支持的设置

```python
class ModelSettings(TypedDict, total=False):
    max_tokens: int
    temperature: float
    top_p: float
    timeout: float          # 请求超时（秒）
    parallel_tool_calls: bool  # 是否允许并行 tool call（部分模型支持）
```

---

## 十、错误类型总结

| 错误 | 触发条件 | 是否可重试 |
|------|---------|-----------|
| `UnexpectedModelBehavior` | 超过 max retries / 模型行为异常 | 否 |
| `UsageLimitExceeded` | 超过 usage limits | 否 |
| `ModelRetry` | Tool 或 Validator 显式返回 | 是（自动重试） |
| `UserError` | 配置错误（如 output_type 不合法） | 否（编程错误） |
| 模型 API 错误 | 网络错误、rate limit 等 | 取决于 Provider |
| Tool 运行异常 | Tool 函数内部错误 | 作为 tool error 反馈给模型 |

---

## 十一、Go 实现映射表

> 将 Pydantic AI 的 Python 行为映射到我们的 Go 实现

| Pydantic AI | Go 实现 | 备注 |
|-------------|---------|------|
| `agent.run()` | `Agent[D,O].Run()` | |
| `agent.run_stream()` | `Agent[D,O].RunStream()` | |
| `RunContext[D]` | `*RunContext[D]` | 通过 context.Value 传递 |
| `result.data` | `result.Output` | 泛型 O |
| `result.new_messages()` | `result.NewMessages()` | `[]*schema.Message` |
| `result.all_messages()` | `result.AllMessages()` | `[]*schema.Message` |
| `result.usage()` | `result.Usage` | `Usage` struct |
| `ModelRetry("msg")` | `return agent.NewModelRetry("msg")` | |
| `@agent.tool` | `agent.WithToolFunc[D,O,Args](...)` | |
| `@agent.system_prompt` | `agent.WithDynamicSystemPrompt[D,O](fn)` | |
| `message_history=` | `agent.WithHistory(msgs)` | RunOption |
| `usage_limits=` | `agent.WithUsageLimits(limits)` | RunOption |
| `model_settings=` | `agent.WithModelSettings(settings)` | RunOption |
| `output_type=int` | `Agent[D, int]` | 泛型参数 |
| `output_type=A \| B` | `Agent[D, OneOf2[A,B]]` | |
| `agent.override(model=)` | `agent.WithModel(m)` | 返回新 Agent |
| `TestModel` | `testutil.TestModel` | 实现 Eino ChatModel |
| `FunctionModel` | `testutil.FunctionModel` | 实现 Eino ChatModel |
