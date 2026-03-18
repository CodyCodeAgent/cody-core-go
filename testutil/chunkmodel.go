package testutil

import (
	"context"
	"io"

	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"
)

// Ensure ChunkingModel implements BaseChatModel.
var _ model.BaseChatModel = (*ChunkingModel)(nil)

// ChunkingModel is a mock model that streams text in multiple chunks.
// Used to test real token-by-token streaming behavior.
type ChunkingModel struct {
	chunks []string
}

// NewChunkingModel creates a ChunkingModel that streams the given text chunks.
func NewChunkingModel(chunks []string) *ChunkingModel {
	return &ChunkingModel{chunks: chunks}
}

// Generate returns the concatenated chunks as a single response.
func (m *ChunkingModel) Generate(ctx context.Context, input []*schema.Message, opts ...model.Option) (*schema.Message, error) {
	var content string
	for _, c := range m.chunks {
		content += c
	}
	return &schema.Message{Role: schema.Assistant, Content: content}, nil
}

// Stream sends each chunk as a separate streamed message.
func (m *ChunkingModel) Stream(ctx context.Context, input []*schema.Message, opts ...model.Option) (
	*schema.StreamReader[*schema.Message], error,
) {
	reader, writer := schema.Pipe[*schema.Message](len(m.chunks))
	go func() {
		for _, chunk := range m.chunks {
			writer.Send(&schema.Message{
				Role:    schema.Assistant,
				Content: chunk,
			}, nil)
		}
		writer.Send(nil, io.EOF)
		writer.Close()
	}()
	return reader, nil
}
