package agent

import (
	"context"
	"testing"

	"github.com/cloudwego/eino/schema"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/codycode/cody-core-go/testutil"
)

func TestConversation_MultiTurn(t *testing.T) {
	callIdx := 0
	fm := testutil.NewFunctionModel(func(msgs []*schema.Message, _ []*schema.ToolInfo) (*schema.Message, error) {
		callIdx++
		switch callIdx {
		case 1:
			return &schema.Message{Role: schema.Assistant, Content: "Nice to meet you, Zhang San!"}, nil
		case 2:
			// Verify history is carried
			hasHistory := false
			for _, m := range msgs {
				if m.Content == "My name is Zhang San" {
					hasHistory = true
				}
			}
			if hasHistory {
				return &schema.Message{Role: schema.Assistant, Content: "Your name is Zhang San."}, nil
			}
			return &schema.Message{Role: schema.Assistant, Content: "I don't know your name."}, nil
		default:
			return &schema.Message{Role: schema.Assistant, Content: "OK"}, nil
		}
	})

	a := New[NoDeps, string](fm,
		WithSystemPrompt[NoDeps, string]("Remember everything."),
	)

	conv := NewConversation(a)

	// First turn
	r1, err := conv.Send(context.Background(), "My name is Zhang San", NoDeps{})
	require.NoError(t, err)
	assert.Contains(t, r1.Output, "Zhang San")

	// Second turn — should carry history
	r2, err := conv.Send(context.Background(), "What's my name?", NoDeps{})
	require.NoError(t, err)
	assert.Contains(t, r2.Output, "Zhang San")

	// Check messages accumulated
	assert.True(t, len(conv.Messages()) >= 4) // user1 + assistant1 + user2 + assistant2
}

func TestConversation_Reset(t *testing.T) {
	callIdx := 0
	fm := testutil.NewFunctionModel(func(msgs []*schema.Message, _ []*schema.ToolInfo) (*schema.Message, error) {
		callIdx++
		return &schema.Message{Role: schema.Assistant, Content: "response"}, nil
	})

	a := New[NoDeps, string](fm)
	conv := NewConversation(a)

	_, err := conv.Send(context.Background(), "Hello", NoDeps{})
	require.NoError(t, err)
	assert.True(t, len(conv.Messages()) > 0)

	conv.Reset()
	assert.Empty(t, conv.Messages())
}
