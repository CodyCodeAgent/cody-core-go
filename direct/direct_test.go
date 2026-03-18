package direct

import (
	"context"
	"testing"

	"github.com/cloudwego/eino/schema"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/codycode/cody-core-go/agent"
	"github.com/codycode/cody-core-go/testutil"
)

type Sentiment struct {
	Label string  `json:"label"`
	Score float64 `json:"score"`
}

func TestRequestText(t *testing.T) {
	tm := testutil.NewTestModel(
		testutil.TestResponse{Text: "Hello World"},
	)

	text, err := RequestText(context.Background(), tm, "Translate: 你好世界")
	require.NoError(t, err)
	assert.Equal(t, "Hello World", text)

	// Verify no tools were sent
	call := tm.AllCalls()[0]
	assert.Empty(t, call.Tools)
}

func TestRequestText_WithSystemPrompt(t *testing.T) {
	tm := testutil.NewTestModel(
		testutil.TestResponse{Text: "translated"},
	)

	text, err := RequestText(context.Background(), tm, "Translate",
		WithSystemPrompt("You are a translator."),
	)
	require.NoError(t, err)
	assert.Equal(t, "translated", text)

	call := tm.AllCalls()[0]
	assert.Equal(t, schema.System, call.Messages[0].Role)
	assert.Equal(t, "You are a translator.", call.Messages[0].Content)
}

func TestRequestText_WithMessages(t *testing.T) {
	tm := testutil.NewTestModel(
		testutil.TestResponse{Text: "response"},
	)

	customMsgs := []*schema.Message{
		{Role: schema.System, Content: "Custom system"},
		{Role: schema.User, Content: "Custom user message"},
	}

	text, err := RequestText(context.Background(), tm, "ignored prompt",
		WithMessages(customMsgs),
	)
	require.NoError(t, err)
	assert.Equal(t, "response", text)

	// WithMessages overrides the prompt — verify custom messages were sent
	call := tm.AllCalls()[0]
	assert.Equal(t, 2, len(call.Messages))
	assert.Equal(t, "Custom system", call.Messages[0].Content)
	assert.Equal(t, "Custom user message", call.Messages[1].Content)
}

func TestRequestText_WithModelSettings(t *testing.T) {
	tm := testutil.NewTestModel(
		testutil.TestResponse{Text: "ok"},
	)

	temp := float32(0.7)
	maxTok := 200
	topP := float32(0.9)

	_, err := RequestText(context.Background(), tm, "Test",
		WithModelSettings(agent.ModelSettings{
			Temperature: &temp,
			MaxTokens:   &maxTok,
			TopP:        &topP,
			Stop:        []string{"\n"},
		}),
	)
	require.NoError(t, err)
	// Settings are applied to the model call — no error means they were accepted
}

func TestRequest_Struct(t *testing.T) {
	tm := testutil.NewTestModel(
		testutil.TestResponse{
			ToolCalls: []schema.ToolCall{{
				ID:   "call_1",
				Type: "function",
				Function: schema.FunctionCall{
					Name:      "final_result",
					Arguments: `{"label":"positive","score":0.95}`,
				},
			}},
		},
	)

	result, err := Request[Sentiment](context.Background(), tm, "Analyze sentiment",
		WithSystemPrompt("Sentiment analyzer."),
	)
	require.NoError(t, err)
	assert.Equal(t, "positive", result.Label)
	assert.Equal(t, 0.95, result.Score)

	// Verify output tool was sent
	call := tm.AllCalls()[0]
	hasOutputTool := false
	for _, ti := range call.Tools {
		if ti.Name == "final_result" {
			hasOutputTool = true
		}
	}
	assert.True(t, hasOutputTool)
}

func TestRequest_Int(t *testing.T) {
	tm := testutil.NewTestModel(
		testutil.TestResponse{
			ToolCalls: []schema.ToolCall{{
				ID:   "call_1",
				Type: "function",
				Function: schema.FunctionCall{
					Name:      "final_result",
					Arguments: `{"result":8}`,
				},
			}},
		},
	)

	result, err := Request[int](context.Background(), tm, "Rate 1-10")
	require.NoError(t, err)
	assert.Equal(t, 8, result)
}

func TestRequest_ModelReturnsText_Error(t *testing.T) {
	tm := testutil.NewTestModel(
		testutil.TestResponse{Text: "I think it's positive"},
	)

	_, err := Request[Sentiment](context.Background(), tm, "Analyze")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "did not call the output tool")
}

func TestRequest_ModelError(t *testing.T) {
	tm := testutil.NewTestModel(
		testutil.TestResponse{Err: context.DeadlineExceeded},
	)

	_, err := Request[Sentiment](context.Background(), tm, "Analyze")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "deadline")
}

func TestRequestText_ModelError(t *testing.T) {
	tm := testutil.NewTestModel(
		testutil.TestResponse{Err: context.Canceled},
	)

	_, err := RequestText(context.Background(), tm, "Hello")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "canceled")
}
