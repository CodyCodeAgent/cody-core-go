// Package output provides structured output support for the agent framework.
// It handles JSON Schema generation, output tool creation, and output validation.
package output

// Mode defines how structured output is returned from the model.
type Mode int

const (
	// ModeTool uses function calling to return structured data (default).
	// The framework registers a "final_result" tool that the model calls
	// to return structured output.
	ModeTool Mode = iota

	// ModeNative uses the model's native structured output API (e.g., OpenAI JSON mode).
	ModeNative

	// ModePrompted constrains output format through prompt engineering.
	ModePrompted
)
