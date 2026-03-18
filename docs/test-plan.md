# 测试计划

> 版本：v0.1 Draft
> 日期：2026-03-17
> 目的：定义 cody-core-go 各模块的测试用例，确保行为与 Pydantic AI 一致

---

## 一、测试策略

### 1.1 测试层次

| 层次 | 工具 | 覆盖范围 |
|------|------|---------|
| **单元测试** | `go test` + TestModel | 每个模块独立测试，不调用真实 API |
| **集成测试** | `go test` + FunctionModel | 模块间交互，验证完整 Agent Loop |
| **端到端测试** | `go test` + 真实 API（CI 选跑） | 真实模型调用，验证与实际 API 的兼容性 |

### 1.2 核心原则

- **所有单元/集成测试使用 TestModel**，零 API 调用，可在 CI 中快速运行
- **测试用例从 Pydantic AI 行为规格推导**，确保行为一致
- **每个 edge case 对应一个测试**，不依赖"正常工作就行"

---

## 二、output/schema 包测试

### 2.1 BuildParamsOneOf[T] — struct 类型

| 测试用例 | 输入 | 期望输出 |
|---------|------|---------|
| 基本 struct | `struct { Name string \`json:"name"\` }` | `{"type":"object","properties":{"name":{"type":"string"}},"required":["name"]}` |
| 带 description tag | `struct { Name string \`json:"name" description:"用户名"\` }` | properties.name.description == "用户名" |
| 带 enum tag | `struct { Status string \`json:"status" enum:"a,b,c"\` }` | properties.status.enum == ["a","b","c"] |
| ~~带 minimum/maximum~~ | ~~`struct { Score int \`json:"score" minimum:"0" maximum:"100"\` }`~~ | ⚠️ **未实现** — 当前不支持 minimum/maximum tag |
| optional 字段（omitempty） | `struct { Name string \`json:"name,omitempty"\` }` | Name 不在 required 列表中 |
| 嵌套 struct | `struct { Addr Address \`json:"addr"\` }` | addr 字段递归生成嵌套 schema |
| 嵌套 slice | `struct { Tags []string \`json:"tags"\` }` | tags 为 array 类型，items 为 string |
| 嵌套 struct slice | `struct { Items []Item \`json:"items"\` }` | items 为 array，items.items 为 object |
| 指针字段 | `struct { Name *string \`json:"name"\` }` | 与非指针相同的 schema |
| 空 struct | `struct{}` | `{"type":"object","properties":{}}` |
| 匿名嵌入 | `struct { BaseModel; Extra string }` | BaseModel 的字段展平到顶层 |

### 2.2 BuildParamsOneOf[T] — 原始类型

| 测试用例 | 输入类型 | 期望 JSON Schema |
|---------|---------|-----------------|
| int | `int` | `{"type":"object","properties":{"result":{"type":"integer"}},"required":["result"]}` |
| int64 | `int64` | 同上 |
| float64 | `float64` | `{"type":"object","properties":{"result":{"type":"number"}},"required":["result"]}` |
| bool | `bool` | `{"type":"object","properties":{"result":{"type":"boolean"}},"required":["result"]}` |
| string | `string` | 不生成 schema（string 类型不走 output tool） |
| []string | `[]string` | `{"type":"object","properties":{"result":{"type":"array","items":{"type":"string"}}},"required":["result"]}` |
| []int | `[]int` | result 为 array，items 为 integer |

---

## 三、output/tool_output 包测试

### 3.1 GenerateOutputTool

| 测试用例 | 输入 | 期望 |
|---------|------|------|
| struct 类型 | `GenerateOutputTool[MyStruct](paramsOneOf)` | tool name == "final_result"，参数 schema 正确 |
| 原始类型 | `GenerateOutputTool[int](paramsOneOf)` | tool name == "final_result"，参数包含 result 字段 |
| tool description | 任意类型 | description 包含 "final response" |

### 3.2 ParseStructuredOutput

| 测试用例 | 输入 JSON | 期望 |
|---------|----------|------|
| 正确 JSON | `{"name":"test","score":5}` | 正确反序列化为 O |
| 无效 JSON | `{invalid}` | 返回解析错误 |
| 字段类型错误 | `{"name":123}` | 返回类型错误 |
| 缺少 required 字段 | `{"name":"test"}` (缺少 score) | 反序列化成功（Go 零值），但 validator 可以捕获 |
| 多余字段 | `{"name":"test","score":5,"extra":"x"}` | 忽略多余字段，正确解析 |
| 原始类型 | `{"result":42}` | 解析为 int(42) |
| 原始类型（空） | `{}` | 返回零值或错误 |

---

## 四、agent 包测试

### 4.1 Agent.Run — 正常路径

| 测试用例 | 场景 | TestModel 配置 | 期望 |
|---------|------|---------------|------|
| 纯文本 Agent | O=string | 返回文本 "hello" | result.Output == "hello" |
| 结构化输出 Agent | O=MyStruct | 返回 final_result tool call | result.Output 正确反序列化 |
| 单 Tool Call | 模型先调 tool 再返回结果 | 2 个响应（tool call + final_result） | tool 被调用，最终 output 正确 |
| 多 Tool Call（同一轮） | 模型一次返回 2 个 tool call | 1 个包含 2 个 tool call 的响应 | 两个 tool 都被调用 |
| 多轮 Tool Call | 模型先调 tool A，再调 tool B，最后返回结果 | 3 个响应 | 按顺序执行 |
| int 输出 | O=int | 返回 final_result(result:42) | result.Output == 42 |
| bool 输出 | O=bool | 返回 final_result(result:true) | result.Output == true |
| []string 输出 | O=[]string | 返回 final_result(result:["a","b"]) | result.Output == ["a","b"] |

### 4.2 Agent.Run — 重试路径

| 测试用例 | 场景 | 期望 |
|---------|------|------|
| OutputValidator 触发重试 | validator 返回 ModelRetry | 模型被再次调用，retry 消息在对话中 |
| Tool 返回 ModelRetry | tool 函数返回 ModelRetry | tool error 反馈给模型，retry 计数 +1 |
| 超过 max retries | 连续失败 > max_retries | 返回 ToolRetriesExceededError / ResultRetriesExceededError |
| 重试后成功 | 第一次验证失败，第二次成功 | 最终正确返回 |
| 模型返回纯文本（O≠string） | 模型不调 output tool | 反馈 "please call the function"，触发 retry |

### 4.3 Agent.Run — Output Tool + 普通 Tool 混合

| 测试用例 | 场景 | 期望 |
|---------|------|------|
| output tool 与普通 tool 同时返回 | 模型同时返回 final_result + search | **忽略普通 tool**，取 output tool 结果 |
| 只有普通 tool | 模型返回 search tool call | 执行 tool，继续 loop |
| 只有 output tool | 模型返回 final_result | 解析结果并返回 |

### 4.4 Agent.Run — 依赖注入

| 测试用例 | 场景 | 期望 |
|---------|------|------|
| Tool 中访问 deps | tool 函数使用 ctx.Deps.DB | deps 正确传递 |
| Dynamic system prompt 访问 deps | prompt 函数使用 ctx.Deps.UserID | deps 正确传递 |
| NoDeps 场景 | D=NoDeps | 无依赖时正常工作 |
| GetDeps 类型不匹配 | 提取错误类型的 deps | 返回 false |

### 4.5 Agent.Run — Usage Limits

| 测试用例 | 场景 | 期望 |
|---------|------|------|
| request_limit 超限 | 设置 limit=2，模型需要 3 次调用 | 第 3 次前返回 UsageLimitExceeded |
| token_limit 超限 | 设置 total_tokens_limit=100 | 超限时返回 UsageLimitExceeded |
| 无 limit | 不设置 usage limits | 不限制 |

### 4.6 Agent.Run — 消息历史

| 测试用例 | 场景 | 期望 |
|---------|------|------|
| 无历史 | 首次调用 | messages = [system, user] |
| 有历史 | 传入 message_history | messages = [system, history..., user] |
| result.NewMessages | 第一次调用 | 只包含新产生的消息 |
| result.AllMessages | 有历史的调用 | 包含 history + new messages |
| system messages 不在历史中 | 任何调用 | AllMessages 和 NewMessages 都不含 system 消息 |

### 4.7 Agent — Union 输出类型

| 测试用例 | 场景 | 期望 |
|---------|------|------|
| OneOf2 返回类型 A | 模型调用 final_result_vuln_found | result.Output.Value() 是 VulnFound 类型 |
| OneOf2 返回类型 B | 模型调用 final_result_safe | result.Output.Value() 是 Safe 类型 |
| OneOf2 Match | 结果是类型 A | Match 的 onA 被调用，onB 不被调用 |
| 多个 output tools 都注册 | Agent 创建时 | tools 列表包含 final_result_a 和 final_result_b |
| 无效的 output tool 名 | 模型调用 final_result_xxx | 作为 tool not found 处理，触发 retry |

### 4.8 Conversation

| 测试用例 | 场景 | 期望 |
|---------|------|------|
| 多轮对话 | Send 3 次 | 第 2 次和第 3 次自动携带历史 |
| Reset | Send → Reset → Send | Reset 后无历史 |
| Messages() | Send 2 次后调用 | 返回完整历史 |
| 并发安全（不保证） | 文档标注即可 | N/A |

---

## 五、direct 包测试

| 测试用例 | 场景 | 期望 |
|---------|------|------|
| RequestText | 纯文本请求 | 返回模型文本 |
| Request[T] | 结构化请求 | 返回正确反序列化的 T |
| Request[int] | 原始类型请求 | 返回正确的 int |
| WithSystemPrompt | 带 system prompt | messages 中包含 system 消息 |
| 无 tool 注入 | RequestText | 不注册任何 tool |
| Request[T] 模型返回纯文本 | O≠string 但模型返回文本 | 返回错误 |

---

## 六、testutil 包测试

### 6.1 TestModel

| 测试用例 | 场景 | 期望 |
|---------|------|------|
| 预设响应顺序 | 设置 3 个响应 | 按顺序返回 |
| 超出预设响应 | 设置 2 个响应，调用 3 次 | 第 3 次返回错误 |
| 记录调用 | 调用 2 次 | CallCount() == 2, AllCalls() 长度 == 2 |
| LastCall | 调用后 | LastCall() 返回最近一次的 messages 和 tools |
| 实现 ChatModel 接口 | 类型断言 | 实现 model.ChatModel |
| Stream 模式 | 调用 Stream | 返回 StreamReader |

### 6.2 FunctionModel

| 测试用例 | 场景 | 期望 |
|---------|------|------|
| 自定义处理函数 | handler 根据 messages 返回不同响应 | 按 handler 逻辑返回 |
| handler 可访问 tools | handler 参数包含 tools 列表 | tools 正确传入 |
| handler 返回错误 | handler 返回 error | Generate 返回 error |

---

## 七、Tool prepare 测试

| 测试用例 | 场景 | 期望 |
|---------|------|------|
| 无 prepare | 普通 tool | schema 不变 |
| prepare 删除字段 | 非管理员删除 admin_filter | 传给模型的 tool schema 中无 admin_filter |
| prepare 修改 description | 根据上下文修改字段描述 | 传给模型的 schema description 已更新 |
| prepare 每轮独立 | 同一 run 中多轮 | 每轮独立调用 prepare |
| prepare 可访问 RunContext | prepare 函数使用 ctx.Deps | deps 正确传入 |

---

## 八、System Prompt 测试

| 测试用例 | 场景 | 期望 |
|---------|------|------|
| 静态 prompt | 设置一个 | messages[0] 为 system 消息 |
| 多个静态 prompt | 设置 2 个 | 有 2 个 system 消息 |
| 动态 prompt | 设置一个函数 | 函数被调用，结果在 system 消息中 |
| 静态 + 动态混合 | 1 个静态 + 1 个动态 | 2 个 system 消息，静态在前 |
| 动态 prompt 错误 | 函数返回 error | Agent.Run 返回 error |
| 动态 prompt 访问 deps | 函数使用 ctx.Deps | deps 正确传递 |

---

## 九、错误处理测试

| 测试用例 | 场景 | 期望错误类型 |
|---------|------|-------------|
| max retries 超限 | 连续 retry > max_retries | ToolRetriesExceededError / ResultRetriesExceededError |
| usage limit 超限 | token/request 超限 | UsageLimitExceeded |
| 模型 API 错误 | ChatModel.Generate 返回 error | 原始错误透传 |
| Tool 函数 panic | tool 内 panic | 不崩溃，作为 error 处理 |
| output JSON 解析失败 | 模型返回无效 JSON | 触发 retry |
| output 验证失败 | validator 返回 ModelRetry | 触发 retry |
| context 取消 | ctx 被 cancel | 返回 context.Canceled |

---

## 十、并发安全测试

| 测试用例 | 场景 | 期望 |
|---------|------|------|
| 同一 Agent 并发 Run | 10 个 goroutine 同时 Run | 全部正确返回，无 data race |
| Agent 不可变 | 创建后并发访问配置 | 无竞争条件 |
| RunContext 隔离 | 并发 Run 使用不同 deps | 各自的 deps 不串扰 |

---

## 十一、性能基准测试

| 测试用例 | 度量 |
|---------|------|
| Agent 创建 | ns/op, allocs/op |
| SchemaFor struct | ns/op（应缓存） |
| JSON 解析 + 验证 | ns/op |
| RunContext 注入/提取 | ns/op |

---

## 十二、测试覆盖率目标

| 包 | 目标覆盖率 |
|-----|-----------|
| output/ | ≥ 90% |
| agent/ | ≥ 85% |
| direct/ | ≥ 90% |
| testutil/ | ≥ 80% |
| deps/ | ≥ 90% |
