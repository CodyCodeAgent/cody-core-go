# Examples 用例集

> 版本：v0.1 Draft
> 日期：2026-03-17
> 目的：定义端到端示例，作为实现的"活的规格"——先写出期望的代码和行为，再实现让它跑通

---

## Example 1: Hello World — 最简 Agent

**目标：** 验证最基本的文本 Agent 创建和运行。

```go
package main

import (
    "context"
    "fmt"
    "log"
    "os"

    "github.com/codycode/cody-core-go/agent"
    einoopenai "github.com/cloudwego/eino-ext/components/model/openai"
)

func main() {
    ctx := context.Background()

    chatModel, err := einoopenai.NewChatModel(ctx, &einoopenai.ChatModelConfig{
        Model:  "gpt-4o",
        APIKey: os.Getenv("OPENAI_API_KEY"),
    })
    if err != nil {
        log.Fatal(err)
    }

    // 最简 Agent：无依赖，输出纯文本
    myAgent := agent.New[agent.NoDeps, string](
        chatModel,
        agent.WithSystemPrompt[agent.NoDeps, string]("You are a helpful assistant. Be concise."),
    )

    result, err := myAgent.Run(ctx, "What is Go?", agent.NoDeps{})
    if err != nil {
        log.Fatal(err)
    }

    fmt.Println(result.Output)
    fmt.Printf("Tokens used: %d\n", result.Usage.TotalTokens)
}
```

**期望行为：**
- `result.Output` 是 string 类型，包含模型的回答
- 不生成 output tool（O=string）
- `result.Usage` 记录了 token 用量
- `result.NewMessages()` 包含 [UserPrompt, AssistantResponse]

---

## Example 2: 结构化输出 — 数据提取

**目标：** 验证 struct 类型的结构化输出能力。

```go
package main

import (
    "context"
    "fmt"
    "log"

    "github.com/codycode/cody-core-go/agent"
)

// 输出类型
type MovieReview struct {
    Title     string  `json:"title" description:"电影名称"`
    Rating    float64 `json:"rating" description:"评分 1-10" minimum:"1" maximum:"10"`
    Sentiment string  `json:"sentiment" description:"情感倾向" enum:"positive,negative,neutral"`
    Summary   string  `json:"summary" description:"一句话摘要"`
}

func main() {
    ctx := context.Background()
    chatModel := createModel() // 省略模型创建

    myAgent := agent.New[agent.NoDeps, MovieReview](
        chatModel,
        agent.WithSystemPrompt[agent.NoDeps, MovieReview](
            "You are a movie critic. Analyze the given review and extract structured information.",
        ),
    )

    result, err := myAgent.Run(ctx,
        "刚看了《星际穿越》，诺兰的叙事太震撼了，五维空间的概念让人拍案叫绝。唯一不足是中间节奏稍慢。总体9分！",
        agent.NoDeps{},
    )
    if err != nil {
        log.Fatal(err)
    }

    fmt.Printf("电影: %s\n", result.Output.Title)       // 星际穿越
    fmt.Printf("评分: %.1f\n", result.Output.Rating)     // 9.0
    fmt.Printf("情感: %s\n", result.Output.Sentiment)    // positive
    fmt.Printf("摘要: %s\n", result.Output.Summary)
}
```

**期望行为：**
- 框架自动生成 `final_result` output tool，schema 包含 title/rating/sentiment/summary
- 模型调用 `final_result` → 框架反序列化为 `MovieReview`
- `Rating` 字段的 schema 包含 `minimum:1, maximum:10`
- `Sentiment` 字段的 schema 包含 `enum:["positive","negative","neutral"]`

---

## Example 3: Tool + 依赖注入

**目标：** 验证 Tool 注册、依赖注入、多轮 tool call。

```go
package main

import (
    "context"
    "fmt"
    "log"

    "github.com/codycode/cody-core-go/agent"
)

// 依赖
type Deps struct {
    APIKey string
    UserID string
}

// 输出
type WeatherReport struct {
    City        string  `json:"city" description:"城市名"`
    Temperature float64 `json:"temperature" description:"温度（摄氏度）"`
    Condition   string  `json:"condition" description:"天气状况"`
    Advice      string  `json:"advice" description:"穿衣建议"`
}

// Tool 参数
type GetWeatherArgs struct {
    City string `json:"city" description:"城市名称" required:"true"`
}

type GetUserPrefsArgs struct {
    UserID string `json:"user_id" description:"用户ID" required:"true"`
}

func main() {
    ctx := context.Background()
    chatModel := createModel()

    myAgent := agent.New[Deps, WeatherReport](
        chatModel,
        agent.WithSystemPrompt[Deps, WeatherReport](
            "You are a weather assistant. Check the weather and give clothing advice based on user preferences.",
        ),
        agent.WithDynamicSystemPrompt[Deps, WeatherReport](
            func(ctx *agent.RunContext[Deps]) (string, error) {
                return fmt.Sprintf("Current user ID: %s", ctx.Deps.UserID), nil
            },
        ),
        // Tool 1: 获取天气
        agent.WithToolFunc[Deps, WeatherReport, GetWeatherArgs](
            "get_weather", "Get current weather for a city",
            func(ctx *agent.RunContext[Deps], args GetWeatherArgs) (string, error) {
                // ctx.Deps.APIKey 可用
                return fmt.Sprintf(`{"city":"%s","temp":22.5,"condition":"sunny"}`, args.City), nil
            },
        ),
        // Tool 2: 获取用户偏好
        agent.WithToolFunc[Deps, WeatherReport, GetUserPrefsArgs](
            "get_user_prefs", "Get user clothing preferences",
            func(ctx *agent.RunContext[Deps], args GetUserPrefsArgs) (string, error) {
                return `{"style":"casual","cold_sensitive":true}`, nil
            },
        ),
        agent.WithMaxRetries[Deps, WeatherReport](2),
    )

    result, err := myAgent.Run(ctx, "今天北京天气怎么样？我该穿什么？", Deps{
        APIKey: "weather-api-key",
        UserID: "user_123",
    })
    if err != nil {
        log.Fatal(err)
    }

    fmt.Printf("城市: %s, 温度: %.1f°C, 状况: %s\n",
        result.Output.City, result.Output.Temperature, result.Output.Condition)
    fmt.Printf("建议: %s\n", result.Output.Advice)
}
```

**期望行为：**
- 模型先调用 `get_weather("北京")` 和可能的 `get_user_prefs("user_123")`
- Tool 函数中 `ctx.Deps.APIKey` == "weather-api-key"
- 动态 system prompt 中 `ctx.Deps.UserID` == "user_123"
- 最终模型调用 `final_result` 返回 `WeatherReport`

---

## Example 4: Output Validation + Retry

**目标：** 验证输出验证和自动重试机制。

```go
package main

import (
    "context"
    "fmt"
    "log"

    "github.com/codycode/cody-core-go/agent"
)

type QuizAnswer struct {
    Answer     string `json:"answer" description:"答案"`
    Confidence int    `json:"confidence" description:"置信度 0-100"`
    Reasoning  string `json:"reasoning" description:"推理过程"`
}

func main() {
    ctx := context.Background()
    chatModel := createModel()

    myAgent := agent.New[agent.NoDeps, QuizAnswer](
        chatModel,
        agent.WithSystemPrompt[agent.NoDeps, QuizAnswer]("Answer quiz questions with reasoning."),
        agent.WithOutputValidator[agent.NoDeps, QuizAnswer](
            func(ctx context.Context, o QuizAnswer) (QuizAnswer, error) {
                if o.Confidence < 0 || o.Confidence > 100 {
                    return o, agent.NewModelRetry(
                        fmt.Sprintf("confidence must be 0-100, got %d", o.Confidence),
                    )
                }
                if len(o.Reasoning) < 10 {
                    return o, agent.NewModelRetry("reasoning must be at least 10 characters")
                }
                return o, nil
            },
        ),
        agent.WithMaxRetries[agent.NoDeps, QuizAnswer](3),
    )

    result, err := myAgent.Run(ctx, "地球到月球的平均距离是多少？", agent.NoDeps{})
    if err != nil {
        log.Fatal(err)
    }

    fmt.Printf("答案: %s (置信度: %d%%)\n", result.Output.Answer, result.Output.Confidence)
    fmt.Printf("推理: %s\n", result.Output.Reasoning)
}
```

**期望行为：**
- 如果模型首次返回 confidence=150，验证失败
- 框架自动将验证错误反馈给模型（"confidence must be 0-100, got 150"）
- 模型修正后重新生成
- 如果连续失败超过 3 次 → 返回 `MaxRetriesError`

---

## Example 5: Union 输出类型 — 代码审查

**目标：** 验证 OneOf2 Union 输出。

```go
package main

import (
    "context"
    "fmt"
    "log"

    "github.com/codycode/cody-core-go/agent"
)

type VulnFound struct {
    Type     string `json:"type" description:"漏洞类型"`
    Severity string `json:"severity" description:"严重程度" enum:"low,medium,high,critical"`
    Line     int    `json:"line" description:"代码行号"`
    Fix      string `json:"fix" description:"修复建议"`
}

type CodeSafe struct {
    Summary string   `json:"summary" description:"安全分析摘要"`
    Checked []string `json:"checked" description:"已检查的安全项目"`
}

func main() {
    ctx := context.Background()
    chatModel := createModel()

    reviewer := agent.NewOneOf2[agent.NoDeps, VulnFound, CodeSafe](
        chatModel,
        agent.WithSystemPrompt[agent.NoDeps, agent.OneOf2[VulnFound, CodeSafe]](
            "You are a security code reviewer. Analyze code for vulnerabilities.",
        ),
    )

    code := `
    query := "SELECT * FROM users WHERE name = '" + userInput + "'"
    db.Exec(query)
    `

    result, err := reviewer.Run(ctx, "Review this code:\n"+code, agent.NoDeps{})
    if err != nil {
        log.Fatal(err)
    }

    // 方式一：type switch
    switch v := result.Output.Value().(type) {
    case VulnFound:
        fmt.Printf("⚠️ 发现漏洞: %s (严重: %s, 行 %d)\n", v.Type, v.Severity, v.Line)
        fmt.Printf("修复: %s\n", v.Fix)
    case CodeSafe:
        fmt.Printf("✅ 代码安全: %s\n", v.Summary)
    }

    // 方式二：Match
    result.Output.Match(
        func(v VulnFound) {
            fmt.Printf("漏洞: %s\n", v.Type)
        },
        func(s CodeSafe) {
            fmt.Printf("安全: %s\n", s.Summary)
        },
    )
}
```

**期望行为：**
- 注册两个 output tool：`final_result_VulnFound` 和 `final_result_CodeSafe`
- 对于 SQL 注入代码，模型应选择 `final_result_VulnFound`
- `result.Output.Value()` 返回 `VulnFound` 类型
- `Match` 调用正确的分支

---

## Example 6: 多轮对话 — Conversation

**目标：** 验证 Conversation 自动管理消息历史。

```go
package main

import (
    "context"
    "fmt"
    "log"

    "github.com/codycode/cody-core-go/agent"
)

func main() {
    ctx := context.Background()
    chatModel := createModel()

    myAgent := agent.New[agent.NoDeps, string](
        chatModel,
        agent.WithSystemPrompt[agent.NoDeps, string]("You are a helpful assistant. Remember what the user tells you."),
    )

    conv := agent.NewConversation(myAgent)

    // 第一轮
    r1, err := conv.Send(ctx, "我叫张三，我是一个 Go 开发者", agent.NoDeps{})
    if err != nil {
        log.Fatal(err)
    }
    fmt.Printf("R1: %s\n", r1.Output)

    // 第二轮 — 自动携带历史
    r2, err := conv.Send(ctx, "我叫什么名字？", agent.NoDeps{})
    if err != nil {
        log.Fatal(err)
    }
    fmt.Printf("R2: %s\n", r2.Output) // 应该回答"张三"

    // 第三轮 — 继续累积
    r3, err := conv.Send(ctx, "我的职业是什么？", agent.NoDeps{})
    if err != nil {
        log.Fatal(err)
    }
    fmt.Printf("R3: %s\n", r3.Output) // 应该回答"Go 开发者"

    // 查看消息历史
    fmt.Printf("History length: %d\n", len(conv.Messages()))

    // 重置
    conv.Reset()
    r4, err := conv.Send(ctx, "我叫什么？", agent.NoDeps{})
    if err != nil {
        log.Fatal(err)
    }
    fmt.Printf("R4: %s\n", r4.Output) // 应该不知道名字了
}
```

**期望行为：**
- `r2` 能回答"张三"，因为 Conversation 自动传入了 r1 的消息历史
- `r3` 能回答"Go 开发者"
- Reset 后 `r4` 不知道名字
- `conv.Messages()` 返回累积的所有消息（不含 system）

---

## Example 7: Direct Request — 一行搞定

**目标：** 验证直接模型请求（绕过 Agent）。

```go
package main

import (
    "context"
    "fmt"
    "log"

    "github.com/codycode/cody-core-go/direct"
)

type Sentiment struct {
    Label string  `json:"label" description:"positive/negative/neutral"`
    Score float64 `json:"score" description:"confidence 0-1"`
}

func main() {
    ctx := context.Background()
    chatModel := createModel()

    // 纯文本请求
    text, err := direct.RequestText(ctx, chatModel, "翻译为英文：你好世界")
    if err != nil {
        log.Fatal(err)
    }
    fmt.Println(text) // "Hello World"

    // 结构化输出请求
    result, err := direct.Request[Sentiment](ctx, chatModel,
        "分析情感：今天天气真好，心情愉快",
        direct.WithSystemPrompt("You are a sentiment analyzer."),
    )
    if err != nil {
        log.Fatal(err)
    }
    fmt.Printf("Label: %s, Score: %.2f\n", result.Label, result.Score)

    // 原始类型请求
    score, err := direct.Request[int](ctx, chatModel, "Rate this text quality 1-10: Go is awesome")
    if err != nil {
        log.Fatal(err)
    }
    fmt.Printf("Score: %d\n", score)
}
```

**期望行为：**
- `RequestText` 不生成 output tool，直接返回文本
- `Request[Sentiment]` 生成 output tool → 模型调用 → 反序列化
- `Request[int]` 生成 output tool，参数为 `{"result": int}`
- 无 retry 机制（直接请求不含 Agent Loop）

---

## Example 8: 测试 — TestModel 用法

**目标：** 验证使用 TestModel 进行单元测试。

```go
package myapp

import (
    "context"
    "testing"

    "github.com/codycode/cody-core-go/agent"
    "github.com/codycode/cody-core-go/testutil"
    "github.com/cloudwego/eino/schema"
    "github.com/stretchr/testify/assert"
)

func TestWeatherAgent(t *testing.T) {
    // 预设模型响应序列
    tm := testutil.NewTestModel(
        // 第 1 次调用：模型调用 get_weather tool
        testutil.TestResponse{
            ToolCalls: []schema.ToolCall{{
                ID:   "call_1",
                Type: "function",
                Function: schema.FunctionCall{
                    Name:      "get_weather",
                    Arguments: `{"city":"北京"}`,
                },
            }},
        },
        // 第 2 次调用：模型返回 final_result
        testutil.TestResponse{
            ToolCalls: []schema.ToolCall{{
                ID:   "call_2",
                Type: "function",
                Function: schema.FunctionCall{
                    Name:      "final_result",
                    Arguments: `{"city":"北京","temperature":22.5,"condition":"sunny","advice":"穿短袖"}`,
                },
            }},
        },
    )

    // 用 TestModel 替换真实模型
    testAgent := myWeatherAgent.WithModel(tm)

    result, err := testAgent.Run(context.Background(), "北京天气", Deps{
        APIKey: "test-key",
        UserID: "test-user",
    })

    assert.NoError(t, err)
    assert.Equal(t, "北京", result.Output.City)
    assert.Equal(t, 22.5, result.Output.Temperature)
    assert.Equal(t, "sunny", result.Output.Condition)

    // 验证模型被调用了 2 次
    assert.Equal(t, 2, tm.CallCount())

    // 验证第 1 次调用包含 system prompt
    firstCall := tm.AllCalls()[0]
    assert.Equal(t, schema.System, firstCall.Messages[0].Role)

    // 验证 tool info 被传入
    assert.True(t, len(firstCall.Tools) >= 2) // get_weather + final_result
}

func TestWeatherAgent_RetryOnValidationError(t *testing.T) {
    tm := testutil.NewTestModel(
        // 第 1 次：返回无效的 confidence
        testutil.TestResponse{
            ToolCalls: []schema.ToolCall{{
                ID:   "call_1",
                Type: "function",
                Function: schema.FunctionCall{
                    Name:      "final_result",
                    Arguments: `{"city":"北京","temperature":-999,"condition":"sunny","advice":"test"}`,
                },
            }},
        },
        // 第 2 次：修正后的结果
        testutil.TestResponse{
            ToolCalls: []schema.ToolCall{{
                ID:   "call_2",
                Type: "function",
                Function: schema.FunctionCall{
                    Name:      "final_result",
                    Arguments: `{"city":"北京","temperature":22.5,"condition":"sunny","advice":"穿短袖"}`,
                },
            }},
        },
    )

    testAgent := myWeatherAgent.WithModel(tm)

    result, err := testAgent.Run(context.Background(), "北京天气", Deps{
        APIKey: "test-key",
        UserID: "test-user",
    })

    assert.NoError(t, err)
    assert.Equal(t, 22.5, result.Output.Temperature)
    assert.Equal(t, 2, tm.CallCount()) // 调用了 2 次（第 1 次验证失败，第 2 次成功）
}
```

**期望行为：**
- `TestModel` 按顺序返回预设响应
- `WithModel(tm)` 替换模型后，Agent 行为与真实模型一致
- `CallCount()`、`AllCalls()` 正确记录调用
- retry 场景：第 1 次验证失败 → 反馈给模型 → 第 2 次成功

---

## 示例用例检查清单

| Example | 验证的核心能力 | Phase |
|---------|--------------|-------|
| 1. Hello World | Agent 创建、Run、纯文本输出 | P1 |
| 2. 结构化输出 | struct schema 生成、output tool、反序列化 | P1 |
| 3. Tool + DI | Tool 注册、依赖注入、动态 prompt | P1 |
| 4. Validation | OutputValidator、ModelRetry、自动重试 | P1 |
| 5. Union 输出 | OneOf2、多 output tool、Match | P2 |
| 6. Conversation | 多轮对话、自动历史管理 | P2 |
| 7. Direct Request | direct.RequestText、direct.Request[T] | P1 |
| 8. Testing | TestModel、WithModel、调用断言 | P1 |

**实现策略：先跑通 Example 1-4 + 7-8（P1），再跑通 5-6（P2）。**
