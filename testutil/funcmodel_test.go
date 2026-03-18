package testutil

import (
	"context"
	"fmt"
	"testing"

	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFunctionModel_Generate(t *testing.T) {
	fm := NewFunctionModel(func(msgs []*schema.Message, tools []*schema.ToolInfo) (*schema.Message, error) {
		return &schema.Message{
			Role:    schema.Assistant,
			Content: fmt.Sprintf("Got %d messages and %d tools", len(msgs), len(tools)),
		}, nil
	})

	msg, err := fm.Generate(context.Background(),
		[]*schema.Message{{Role: schema.User, Content: "hi"}},
		model.WithTools([]*schema.ToolInfo{{Name: "t1"}}),
	)
	require.NoError(t, err)
	assert.Equal(t, "Got 1 messages and 1 tools", msg.Content)
}

func TestFunctionModel_GenerateError(t *testing.T) {
	fm := NewFunctionModel(func(msgs []*schema.Message, tools []*schema.ToolInfo) (*schema.Message, error) {
		return nil, fmt.Errorf("api error")
	})

	_, err := fm.Generate(context.Background(), nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "api error")
}

func TestFunctionModel_Stream(t *testing.T) {
	fm := NewFunctionModel(func(msgs []*schema.Message, tools []*schema.ToolInfo) (*schema.Message, error) {
		return &schema.Message{Role: schema.Assistant, Content: "streamed"}, nil
	})

	reader, err := fm.Stream(context.Background(), nil)
	require.NoError(t, err)

	msg, err := reader.Recv()
	require.NoError(t, err)
	assert.Equal(t, "streamed", msg.Content)

	reader.Close()
}

func TestFunctionModel_ImplementsBaseChatModel(t *testing.T) {
	var _ model.BaseChatModel = &FunctionModel{}
}

func TestFunctionModel_DynamicBehavior(t *testing.T) {
	callCount := 0
	fm := NewFunctionModel(func(msgs []*schema.Message, _ []*schema.ToolInfo) (*schema.Message, error) {
		callCount++
		if callCount == 1 {
			return &schema.Message{
				Role: schema.Assistant,
				ToolCalls: []schema.ToolCall{{
					ID: "c1", Type: "function",
					Function: schema.FunctionCall{Name: "search", Arguments: `{"q":"test"}`},
				}},
			}, nil
		}
		return &schema.Message{Role: schema.Assistant, Content: "done"}, nil
	})

	// First call returns tool call
	msg1, err := fm.Generate(context.Background(), nil)
	require.NoError(t, err)
	assert.Len(t, msg1.ToolCalls, 1)

	// Second call returns text
	msg2, err := fm.Generate(context.Background(), nil)
	require.NoError(t, err)
	assert.Equal(t, "done", msg2.Content)
}
