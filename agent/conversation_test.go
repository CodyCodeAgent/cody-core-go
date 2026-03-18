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

func TestConversation_Len(t *testing.T) {
	fm := testutil.NewFunctionModel(func(msgs []*schema.Message, _ []*schema.ToolInfo) (*schema.Message, error) {
		return &schema.Message{Role: schema.Assistant, Content: "hi"}, nil
	})

	a := New[NoDeps, string](fm)
	conv := NewConversation(a)

	// Empty conversation
	assert.Equal(t, 0, conv.Len())

	// After one turn: user + assistant
	_, err := conv.Send(context.Background(), "Hello", NoDeps{})
	require.NoError(t, err)
	assert.Equal(t, 2, conv.Len())

	// After reset
	conv.Reset()
	assert.Equal(t, 0, conv.Len())
}

func TestConversation_SetMessages(t *testing.T) {
	fm := testutil.NewFunctionModel(func(msgs []*schema.Message, _ []*schema.ToolInfo) (*schema.Message, error) {
		// Check if restored history is carried
		for _, m := range msgs {
			if m.Content == "previously saved message" {
				return &schema.Message{Role: schema.Assistant, Content: "history restored"}, nil
			}
		}
		return &schema.Message{Role: schema.Assistant, Content: "no history"}, nil
	})

	a := New[NoDeps, string](fm)
	conv := NewConversation(a)

	// Restore a saved conversation state
	saved := []*schema.Message{
		{Role: schema.User, Content: "previously saved message"},
		{Role: schema.Assistant, Content: "previously saved response"},
	}
	conv.SetMessages(saved)

	assert.Equal(t, 2, conv.Len())
	assert.Equal(t, saved, conv.Messages())

	// Send should carry restored history
	r, err := conv.Send(context.Background(), "Do you remember?", NoDeps{})
	require.NoError(t, err)
	assert.Equal(t, "history restored", r.Output)
}
