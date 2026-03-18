package testutil

import (
	"context"
	"io"

	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"
)

// Ensure FunctionModel implements BaseChatModel.
var _ model.BaseChatModel = (*FunctionModel)(nil)

// FunctionModel uses a custom handler function to generate responses.
// This is useful for testing dynamic model behavior.
type FunctionModel struct {
	Handler func(messages []*schema.Message, tools []*schema.ToolInfo) (*schema.Message, error)
}

// NewFunctionModel creates a FunctionModel with the given handler.
func NewFunctionModel(handler func([]*schema.Message, []*schema.ToolInfo) (*schema.Message, error)) *FunctionModel {
	return &FunctionModel{Handler: handler}
}

// Generate calls the handler function with the input messages and tools.
func (m *FunctionModel) Generate(ctx context.Context, input []*schema.Message, opts ...model.Option) (*schema.Message, error) {
	options := model.GetCommonOptions(&model.Options{}, opts...)
	return m.Handler(input, options.Tools)
}

// Stream wraps Generate output as a single-chunk stream.
func (m *FunctionModel) Stream(ctx context.Context, input []*schema.Message, opts ...model.Option) (
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
