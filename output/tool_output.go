package output

import (
	"context"
	"fmt"

	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/schema"
)

const (
	// DefaultOutputToolName is the default name for the output tool.
	DefaultOutputToolName = "final_result"
	// OutputToolDescription is the description for the output tool.
	OutputToolDescription = "The final response which ends this conversation"
)

// outputTool implements tool.InvokableTool for the structured output tool.
// It exists only to provide schema information to the model; it is never actually invoked.
type outputTool struct {
	info *schema.ToolInfo
}

// Info returns the tool's schema information.
func (t *outputTool) Info(_ context.Context) (*schema.ToolInfo, error) {
	return t.info, nil
}

// InvokableRun is not called for output tools; the agent loop intercepts the call.
func (t *outputTool) InvokableRun(_ context.Context, _ string, _ ...tool.Option) (string, error) {
	return "", fmt.Errorf("output tool should not be invoked directly")
}

// GenerateOutputTool creates an Eino InvokableTool for structured output.
// The tool's schema is derived from type T.
func GenerateOutputTool[T any](paramsOneOf *schema.ParamsOneOf) tool.InvokableTool {
	return GenerateOutputToolWithName[T](DefaultOutputToolName, paramsOneOf)
}

// GenerateOutputToolWithName creates an output tool with a custom name.
// Used for Union types where each variant gets its own output tool name.
func GenerateOutputToolWithName[T any](name string, paramsOneOf *schema.ParamsOneOf) tool.InvokableTool {
	return &outputTool{
		info: &schema.ToolInfo{
			Name:       name,
			Desc:       OutputToolDescription,
			ParamsOneOf: paramsOneOf,
		},
	}
}

// IsOutputToolName checks if a tool call name matches an output tool.
func IsOutputToolName(name string) bool {
	return name == DefaultOutputToolName || len(name) > len(DefaultOutputToolName)+1 && name[:len(DefaultOutputToolName)+1] == DefaultOutputToolName+"_"
}
