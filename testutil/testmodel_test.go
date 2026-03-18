package testutil

import (
	"context"
	"testing"

	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTestModel_Generate_TextResponse(t *testing.T) {
	tm := NewTestModel(TestResponse{Text: "Hello"})

	msg, err := tm.Generate(context.Background(), []*schema.Message{
		{Role: schema.User, Content: "Hi"},
	})

	require.NoError(t, err)
	assert.Equal(t, schema.Assistant, msg.Role)
	assert.Equal(t, "Hello", msg.Content)
}

func TestTestModel_Generate_ToolCallResponse(t *testing.T) {
	tm := NewTestModel(TestResponse{
		ToolCalls: []schema.ToolCall{{
			ID:   "call_1",
			Type: "function",
			Function: schema.FunctionCall{
				Name:      "search",
				Arguments: `{"query":"test"}`,
			},
		}},
	})

	msg, err := tm.Generate(context.Background(), []*schema.Message{
		{Role: schema.User, Content: "Search"},
	})

	require.NoError(t, err)
	assert.Len(t, msg.ToolCalls, 1)
	assert.Equal(t, "search", msg.ToolCalls[0].Function.Name)
}

func TestTestModel_Generate_Sequential(t *testing.T) {
	tm := NewTestModel(
		TestResponse{Text: "first"},
		TestResponse{Text: "second"},
		TestResponse{Text: "third"},
	)

	for _, expected := range []string{"first", "second", "third"} {
		msg, err := tm.Generate(context.Background(), nil)
		require.NoError(t, err)
		assert.Equal(t, expected, msg.Content)
	}
}

func TestTestModel_Generate_ExhaustedResponses(t *testing.T) {
	tm := NewTestModel(TestResponse{Text: "only one"})

	_, err := tm.Generate(context.Background(), nil)
	require.NoError(t, err)

	_, err = tm.Generate(context.Background(), nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no more responses")
}

func TestTestModel_Generate_WithError(t *testing.T) {
	tm := NewTestModel(TestResponse{Err: assert.AnError})

	_, err := tm.Generate(context.Background(), nil)
	require.Error(t, err)
	assert.Equal(t, assert.AnError, err)
}

func TestTestModel_Generate_WithUsage(t *testing.T) {
	tm := NewTestModel(TestResponse{
		Text: "hello",
		Usage: &schema.TokenUsage{
			PromptTokens:     10,
			CompletionTokens: 5,
			TotalTokens:      15,
		},
	})

	msg, err := tm.Generate(context.Background(), nil)
	require.NoError(t, err)
	require.NotNil(t, msg.ResponseMeta)
	require.NotNil(t, msg.ResponseMeta.Usage)
	assert.Equal(t, 10, msg.ResponseMeta.Usage.PromptTokens)
	assert.Equal(t, 15, msg.ResponseMeta.Usage.TotalTokens)
}

func TestTestModel_CallCount(t *testing.T) {
	tm := NewTestModel(
		TestResponse{Text: "a"},
		TestResponse{Text: "b"},
	)

	assert.Equal(t, 0, tm.CallCount())

	_, _ = tm.Generate(context.Background(), nil)
	assert.Equal(t, 1, tm.CallCount())

	_, _ = tm.Generate(context.Background(), nil)
	assert.Equal(t, 2, tm.CallCount())
}

func TestTestModel_AllCalls(t *testing.T) {
	tm := NewTestModel(
		TestResponse{Text: "response"},
	)

	msgs := []*schema.Message{{Role: schema.User, Content: "hello"}}
	tools := []*schema.ToolInfo{{Name: "my_tool", Desc: "desc"}}

	_, _ = tm.Generate(context.Background(), msgs, model.WithTools(tools))

	calls := tm.AllCalls()
	require.Len(t, calls, 1)
	assert.Equal(t, msgs, calls[0].Messages)
	assert.Equal(t, tools, calls[0].Tools)
}

func TestTestModel_LastCall(t *testing.T) {
	tm := NewTestModel(
		TestResponse{Text: "a"},
		TestResponse{Text: "b"},
	)

	_, _ = tm.Generate(context.Background(), []*schema.Message{{Content: "first"}})
	_, _ = tm.Generate(context.Background(), []*schema.Message{{Content: "second"}})

	last := tm.LastCall()
	assert.Equal(t, "second", last.Messages[0].Content)
}

func TestTestModel_Reset(t *testing.T) {
	tm := NewTestModel(TestResponse{Text: "hello"})

	_, _ = tm.Generate(context.Background(), nil)
	assert.Equal(t, 1, tm.CallCount())

	tm.Reset()
	assert.Equal(t, 0, tm.CallCount())

	// Can call again after reset
	msg, err := tm.Generate(context.Background(), nil)
	require.NoError(t, err)
	assert.Equal(t, "hello", msg.Content)
}

func TestTestModel_Stream(t *testing.T) {
	tm := NewTestModel(TestResponse{Text: "streaming"})

	reader, err := tm.Stream(context.Background(), nil)
	require.NoError(t, err)

	msg, err := reader.Recv()
	require.NoError(t, err)
	assert.Equal(t, "streaming", msg.Content)

	reader.Close()
}

func TestTestModel_ImplementsBaseChatModel(t *testing.T) {
	var _ model.BaseChatModel = &TestModel{}
}
