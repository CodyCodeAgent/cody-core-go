package agent

import (
	"context"

	"github.com/cloudwego/eino/schema"
)

// Conversation manages multi-turn conversation state.
// It automatically carries message history between Send calls.
// Not safe for concurrent use — create one per user/session.
type Conversation[D, O any] struct {
	agent    *Agent[D, O]
	messages []*schema.Message
}

// NewConversation creates a new conversation with the given agent.
func NewConversation[D, O any](a *Agent[D, O]) *Conversation[D, O] {
	return &Conversation[D, O]{
		agent: a,
	}
}

// Send sends a message and automatically carries forward the conversation history.
func (c *Conversation[D, O]) Send(ctx context.Context, prompt string, deps D, opts ...RunOption) (*Result[O], error) {
	// Prepend history option
	allOpts := make([]RunOption, 0, len(opts)+1)
	if len(c.messages) > 0 {
		allOpts = append(allOpts, WithHistory(c.messages))
	}
	allOpts = append(allOpts, opts...)

	result, err := c.agent.Run(ctx, prompt, deps, allOpts...)
	if err != nil {
		return nil, err
	}

	// Update history with all messages from this run
	c.messages = result.AllMessages()

	return result, nil
}

// SendStream sends a message with streaming and carries forward conversation history.
func (c *Conversation[D, O]) SendStream(ctx context.Context, prompt string, deps D, opts ...RunOption) (*StreamResult[O], error) {
	allOpts := make([]RunOption, 0, len(opts)+1)
	if len(c.messages) > 0 {
		allOpts = append(allOpts, WithHistory(c.messages))
	}
	allOpts = append(allOpts, opts...)

	return c.agent.RunStream(ctx, prompt, deps, allOpts...)
}

// Messages returns the current conversation history.
func (c *Conversation[D, O]) Messages() []*schema.Message {
	return c.messages
}

// Reset clears the conversation history.
func (c *Conversation[D, O]) Reset() {
	c.messages = nil
}
