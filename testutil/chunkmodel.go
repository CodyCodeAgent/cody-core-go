package testutil

import (
	"context"
	"io"

	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"
)

// Ensure ChunkingModel implements BaseChatModel.
var _ model.BaseChatModel = (*ChunkingModel)(nil)

// ChunkingModel is a mock model that streams responses in multiple chunks.
// It can simulate both text streaming and tool call argument streaming.
type ChunkingModel struct {
	chunks []*schema.Message
}

// NewChunkingModel creates a ChunkingModel that streams text in multiple chunks.
func NewChunkingModel(textChunks []string) *ChunkingModel {
	chunks := make([]*schema.Message, len(textChunks))
	for i, t := range textChunks {
		chunks[i] = &schema.Message{Role: schema.Assistant, Content: t}
	}
	return &ChunkingModel{chunks: chunks}
}

// NewChunkingModelFromMessages creates a ChunkingModel from raw message chunks.
// This allows simulating streaming tool call arguments across multiple chunks.
func NewChunkingModelFromMessages(chunks []*schema.Message) *ChunkingModel {
	return &ChunkingModel{chunks: chunks}
}

// Generate returns the accumulated result as a single response.
func (m *ChunkingModel) Generate(ctx context.Context, input []*schema.Message, opts ...model.Option) (*schema.Message, error) {
	// Accumulate all chunks into one message
	result := &schema.Message{Role: schema.Assistant}
	for _, c := range m.chunks {
		result.Content += c.Content
		// Merge tool calls by ID
		for _, tc := range c.ToolCalls {
			found := false
			for j := range result.ToolCalls {
				if result.ToolCalls[j].ID == tc.ID && tc.ID != "" {
					result.ToolCalls[j].Function.Arguments += tc.Function.Arguments
					found = true
					break
				}
			}
			if !found {
				result.ToolCalls = append(result.ToolCalls, tc)
			}
		}
	}
	return result, nil
}

// Stream sends each chunk as a separate streamed message.
func (m *ChunkingModel) Stream(ctx context.Context, input []*schema.Message, opts ...model.Option) (
	*schema.StreamReader[*schema.Message], error,
) {
	reader, writer := schema.Pipe[*schema.Message](len(m.chunks))
	go func() {
		for _, chunk := range m.chunks {
			writer.Send(chunk, nil)
		}
		writer.Send(nil, io.EOF)
		writer.Close()
	}()
	return reader, nil
}
