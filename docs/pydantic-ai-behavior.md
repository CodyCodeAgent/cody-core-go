# Pydantic AI 行为规格文档

> 版本：v0.2 Draft（基于 Pydantic AI 源码验证修正）
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

A:（源码确认）取决于 `end_strategy` 配置：

- **`end_strategy='early'`（默认）**：output tool 先处理，一旦产出最终结果，剩余的普通 tool **全部跳过**。跳过的 tool 收到一个 `ToolReturnPart(content='Tool not executed - a final result was already...')`。
- **`end_strategy='exhaustive'`**：即使 output tool 已产出结果，所有普通 tool 仍然会被执行。

**Go 实现决策：** 默认行为用 `early`（忽略普通 tool），暂不实现 `exhaustive` 模式。

**Q: 模型返回多个普通 tool call 怎么办？**

A:（源码确认）**并行执行**（Python 中用 `asyncio.create_task` + `asyncio.wait`）。Go 中用 goroutine + WaitGroup。所有 tool 结果收集后一起追加到 messages，再进入下一轮模型调用。

支持三种并行模式：`sequential`、`parallel_ordered_events`、`parallel`。Go 中简化为：默认并行（goroutine），用户可配置顺序执行。

**Q: 模型返回空响应（无文本、无 tool call）怎么办？**

A: 视为 unexpected model behavior，触发 retry 或返回错误。

---

## 二、Retry 机制

### 2.1 两套独立的重试计数器（源码确认）

**⚠️ 重要：Pydantic AI 有两套独立的 retry 计数，不是一套。**

**A) Per-tool retries（工具级）**
- 存储在 `RunContext.retries: dict[str, int]` 中，每个 tool name 有独立计数
- 当某个 tool 返回 `ModelRetry` 时，**仅该 tool** 的计数 +1
- 每个 tool 有自己的 `max_retries`（可在 tool 上单独设置，默认继承 agent 的值）
- 检查逻辑：
```python
def _check_max_retries(self, name: str, max_retries: int, error: Exception) -> None:
    if self.ctx.retries.get(name, 0) == max_retries:
        raise UnexpectedModelBehavior(
            f'Tool {name!r} exceeded max retries count of {max_retries}') from error
```

**B) Result retries（输出级）**
- 存储在 `GraphAgentState.retries: int` 中，单一整数
- 当 output validation 失败时计数 +1
- 由 `max_result_retries` 控制（默认 1）
- 检查逻辑：
```python
def increment_retries(self, max_result_retries, error=None):
    self.retries += 1
    if self.retries > max_result_retries:
        raise UnexpectedModelBehavior(f'Exceeded maximum retries ({max_result_retries})')
```

**关键区别表：**

| | Per-tool retries | Result retries |
|---|---|---|
| 存储 | `dict[str, int]`（per tool name） | `int`（全局） |
| 触发 | Tool 函数返回 `ModelRetry` | Output validation 失败 / 模型返回纯文本但需要结构化 |
| 限制 | `tool.max_retries`（每个 tool 独立） | `max_result_retries`（全局） |
| 重置 | 不重置，整个 run 期间累积 | 不重置，整个 run 期间累积 |
| 默认值 | 1 | 1 |

**Go 实现决策：** 我们也应该实现两套独立计数器，而不是简化为一套。

### 2.2 重试时的消息反馈

当 retry 触发时，框架通过 `RetryPromptPart` 反馈给模型：

```python
# RetryPromptPart 结构：
RetryPromptPart(
    tool_name="search",            # 哪个 tool 失败了
    tool_call_id="call_xxx",       # 关联到哪个 tool call
    content=error_message,         # 反馈内容
)
```

**反馈内容因场景不同：**

| 场景 | content |
|------|---------|
| Tool 返回 `ModelRetry("msg")` | `"msg"`（开发者自定义的提示） |
| Output validation 失败（Pydantic） | `error.errors(include_url=False)`（验证错误详情列表） |
| 模型返回纯文本但需要 tool call | `"Please return text or include your response in a tool call."` |

### 2.3 超过重试次数

```python
# 错误类型：UnexpectedModelBehavior（AgentRunError 的子类）
# Tool 超限：
raise UnexpectedModelBehavior(f"Tool 'tool_name' exceeded max retries count of {max_retries}")
# Result 超限：
raise UnexpectedModelBehavior(f"Exceeded maximum retries ({max_result_retries})")
```

在 Go 中映射为：
```go
// Tool retry 超限
type ToolRetriesExceededError struct {
    ToolName   string
    MaxRetries int
    LastError  error
}

// Result retry 超限
type ResultRetriesExceededError struct {
    MaxRetries int
    LastError  error
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

### 3.4 Union 输出类型的 tool 命名（源码确认）

```python
# output_type=Success | Failure
# 生成两个 tool：

# Tool 1:
{
    "name": "final_result_Success",     # ⚠️ 注意：保留原始类名大小写！
    "description": "...",
    "parameters": { /* Success 的 schema */ }
}

# Tool 2:
{
    "name": "final_result_Failure",
    "description": "...",
    "parameters": { /* Failure 的 schema */ }
}
```

**命名规则（源码确认）：**
```python
DEFAULT_OUTPUT_TOOL_NAME = 'final_result'
OUTPUT_TOOL_NAME_SANITIZER = re.compile(r'[^a-zA-Z0-9-_]')

# 多类型时追加类型名（保留 CamelCase，仅去除非法字符）
if multiple:
    safe_name = OUTPUT_TOOL_NAME_SANITIZER.sub('', type_name)
    name = f'final_result_{safe_name}'
```

**⚠️ 之前的设计文档写的 snake_case 是错的，实际是保留原始类名。**
在 Go 中，struct 名是 CamelCase，所以命名为 `"final_result_VulnFound"`、`"final_result_Safe"`。

**Q: 模型调用了一个不存在的 output tool 名怎么办？**
A: 该 tool call 被归类为 `'unknown'`，不作为 output 或 function tool 处理 → 触发 retry，告知模型使用正确的 tool 名。

### 3.5 Plain Text Fallback（源码确认）

当 `output_type≠str` 但模型返回纯文本时：

```python
# Pydantic AI 的处理：
# 1. 检查 OutputSchema 是否有 text_processor
#    - output_type=str 时有 → 直接返回文本
#    - output_type=struct 时无 → 触发 retry
# 2. 无 text_processor → 抛出 ToolRetryError，反馈给模型
```

**反馈消息：**
```
"Please include your response in a tool call."
```
（具体措辞取决于可用的替代方案列表）

### 3.6 Markdown Fence 自动剥离（源码确认）

**⚠️ 新发现：** Pydantic AI 在 JSON 解析前会自动剥离 Markdown 代码块标记：

```python
data = _utils.strip_markdown_fences(data)
# 即 ```json\n{...}\n``` → {...}
```

**Go 实现也应该做这个处理**，因为很多模型喜欢在 JSON 外面包 markdown fence。

### 3.7 JSON 解析与验证不区分（源码确认）

**⚠️ 之前假设 JSON 解析失败和字段验证失败是不同错误 — 实际它们都走 Pydantic 的 `validate_json()`，统一返回 `ValidationError`。**

```python
# 无论是 JSON 语法错误还是字段类型/约束错误，都是：
except ValidationError as e:
    m = RetryPromptPart(content=e.errors(include_url=False))
    raise ToolRetryError(m) from e
```

Go 实现中也应该统一处理：JSON unmarshal 错误和 validator 错误走同一条 retry 路径。

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

### 4.3 Tool 执行顺序（源码确认）

- 同一轮中的多个 tool calls：**并行执行**（默认，用 `asyncio.create_task`）
- 不同轮的 tool calls：**顺序执行**（等上一轮的 tool results 返回后才进入下一轮）
- 支持三种并行模式：`sequential` / `parallel_ordered_events` / `parallel`
- Go 实现简化为：默认并行（goroutine + WaitGroup），可选顺序执行

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

### 8.1 流式 + 结构化输出（源码确认）

当 `output_type≠str` 时的流式行为：

1. 模型开始生成 output tool call 的参数
2. 参数 JSON 逐 token 到达
3. **部分 JSON 解析**（partial parsing）：
   - 使用 Pydantic 的 `'trailing-strings'` partial validation mode
   - 每收到新 chunk 尝试解析一次
   - **⚠️ 关键：partial 阶段的 ValidationError 被静默忽略**
   ```python
   async for response in self.stream_responses(debounce_by=debounce_by):
       try:
           yield await self.validate_response_output(response, allow_partial=True)
       except ValidationError:
           pass  # 静默忽略 partial 验证失败
   ```
   - 成功解析出部分字段 → 产出 partial O
4. 最终完整 JSON 到达 → **严格验证**（`allow_partial=False`），此时 ValidationError 不再忽略

**Go 实现要点：** 需要一个 partial JSON parser。可以简单处理：尝试补全未闭合的 `{`/`[` 后解析。partial 阶段 unmarshal 错误静默忽略。

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

---

## 十二、源码验证修正记录

> v0.2 中基于 Pydantic AI 源码验证后发现的与 v0.1 假设不一致之处

| # | 原假设 | 源码实际行为 | 影响 |
|---|--------|------------|------|
| 1 | Retry 是一个全局计数器 | **两套独立计数器**：per-tool（dict）+ per-result（int） | 需要实现两套计数 |
| 2 | Union tool 命名用 snake_case | **保留原始类名 CamelCase**（`final_result_Success`） | 命名规则修正 |
| 3 | output tool + 普通 tool 同时返回时忽略普通 tool | 取决于 **`end_strategy`**：`early`（默认，忽略）或 `exhaustive`（都执行） | 增加配置选项 |
| 4 | JSON 解析失败和验证失败是不同错误 | **不区分**，统一走 `validate_json` → `ValidationError` | 统一错误处理 |
| 5 | 流式 partial 验证失败触发 retry | **静默忽略**，只有最终完整结果严格验证 | 简化流式实现 |
| 6 | 未考虑 Markdown fence | 自动 **strip markdown fences** 后再解析 JSON | 需要增加预处理 |
| 7 | Tool 错误和 ModelRetry 行为相同 | **Tool 普通异常**不消耗 retry 计数（作为 tool error result）；**ModelRetry** 消耗 per-tool retry 计数 | 区分处理 |
| 8 | 未考虑 output_tool_return_content | `all_messages()` 支持 **自定义 output tool return content**（用于多轮对话） | 考虑是否实现 |
