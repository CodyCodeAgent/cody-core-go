package mcp

import (
	"encoding/json"
	"fmt"

	einojsonschema "github.com/eino-contrib/jsonschema"

	"github.com/cloudwego/eino/schema"
)

// convertInputSchema converts an MCP tool's InputSchema to an Eino ParamsOneOf.
//
// The MCP SDK uses github.com/google/jsonschema-go while Eino uses
// github.com/eino-contrib/jsonschema. Both serialize to standard JSON Schema,
// so we bridge them via JSON round-tripping.
func convertInputSchema(inputSchema any) (*schema.ParamsOneOf, error) {
	if inputSchema == nil {
		return nil, nil
	}

	// Marshal MCP schema to JSON bytes.
	jsonBytes, err := json.Marshal(inputSchema)
	if err != nil {
		return nil, fmt.Errorf("marshal MCP input schema: %w", err)
	}

	// Unmarshal into Eino's jsonschema.Schema.
	var einoSchema einojsonschema.Schema
	if err := json.Unmarshal(jsonBytes, &einoSchema); err != nil {
		return nil, fmt.Errorf("unmarshal to eino schema: %w", err)
	}

	return schema.NewParamsOneOfByJSONSchema(&einoSchema), nil
}
