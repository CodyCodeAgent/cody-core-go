// Package testutil provides test utilities for the agent framework,
// including TestModel and FunctionModel for unit testing without real API calls.
package testutil

import (
	"context"
	"fmt"
	"io"
	"sync"

	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"
)

// Ensure TestModel implements BaseChatModel.
var _ model.BaseChatModel = (*TestModel)(nil)

// TestResponse defines a pre-configured response for TestModel.
type TestResponse struct {
	// Text is the text content of the response.
	Text string
	// ToolCalls are the tool calls in the response.
	ToolCalls []schema.ToolCall
	// Usage is optional token usage for this response.
	Usage *schema.TokenUsage
	// Err if set, Generate will return this error.
	Err error
}

// TestCall records a single call to the model.
type TestCall struct {
	Messages []*schema.Message
	Tools    []*schema.ToolInfo
}

// TestModel is a mock model for unit testing that returns pre-configured responses.
// It implements the Eino BaseChatModel interface.
type TestModel struct {
	mu        sync.Mutex
	responses []TestResponse
	calls     []TestCall
	callIndex int
}

// NewTestModel creates a TestModel with a sequence of responses.
// Responses are returned in order on successive Generate calls.
func NewTestModel(responses ...TestResponse) *TestModel {
	return &TestModel{
		responses: responses,
	}
}

// Generate returns the next pre-configured response.
func (m *TestModel) Generate(ctx context.Context, input []*schema.Message, opts ...model.Option) (*schema.Message, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Extract tools from options
	options := model.GetCommonOptions(&model.Options{}, opts...)
	call := TestCall{
		Messages: input,
		Tools:    options.Tools,
	}
	m.calls = append(m.calls, call)

	if m.callIndex >= len(m.responses) {
		return nil, fmt.Errorf("TestModel: no more responses (called %d times, only %d responses configured)",
			m.callIndex+1, len(m.responses))
	}

	resp := m.responses[m.callIndex]
	m.callIndex++

	if resp.Err != nil {
		return nil, resp.Err
	}

	msg := &schema.Message{
		Role:      schema.Assistant,
		Content:   resp.Text,
		ToolCalls: resp.ToolCalls,
	}

	if resp.Usage != nil {
		msg.ResponseMeta = &schema.ResponseMeta{
			Usage: resp.Usage,
		}
	}

	return msg, nil
}

// Stream returns a StreamReader that yields the next response as a single chunk.
func (m *TestModel) Stream(ctx context.Context, input []*schema.Message, opts ...model.Option) (
	*schema.StreamReader[*schema.Message], error,
) {
	msg, err := m.Generate(ctx, input, opts...)
	if err != nil {
		return nil, err
	}

	reader, writer := schema.Pipe[*schema.Message](1)
	go func() {
		writer.Send(msg, nil)
		writer.Send(nil, io.EOF)
		writer.Close()
	}()

	return reader, nil
}

// CallCount returns the number of times Generate was called.
func (m *TestModel) CallCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.calls)
}

// LastCall returns the most recent call record.
func (m *TestModel) LastCall() TestCall {
	m.mu.Lock()
	defer m.mu.Unlock()
	if len(m.calls) == 0 {
		return TestCall{}
	}
	return m.calls[len(m.calls)-1]
}

// AllCalls returns all recorded calls.
func (m *TestModel) AllCalls() []TestCall {
	m.mu.Lock()
	defer m.mu.Unlock()
	result := make([]TestCall, len(m.calls))
	copy(result, m.calls)
	return result
}

// Reset clears call records and resets the response index.
func (m *TestModel) Reset() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.calls = nil
	m.callIndex = 0
}
