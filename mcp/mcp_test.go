package mcp

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/cloudwego/eino/schema"
	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/CodyCodeAgent/cody-core-go/agent"
	"github.com/CodyCodeAgent/cody-core-go/testutil"
)

// startTestServer creates an in-memory MCP server with the given tools and returns a connected Server.
func startTestServer(t *testing.T, tools map[string]mcpsdk.ToolHandler) *Server {
	t.Helper()
	ctx := context.Background()

	server := mcpsdk.NewServer(&mcpsdk.Implementation{Name: "test-server", Version: "0.1.0"}, nil)
	for name, handler := range tools {
		server.AddTool(&mcpsdk.Tool{
			Name:        name,
			Description: "Test tool: " + name,
			InputSchema: json.RawMessage(`{"type":"object","properties":{"input":{"type":"string"}}}`),
		}, handler)
	}

	sTransport, cTransport := mcpsdk.NewInMemoryTransports()
	_, err := server.Connect(ctx, sTransport, nil)
	require.NoError(t, err)

	s, err := Connect(ctx, cTransport)
	require.NoError(t, err)
	t.Cleanup(func() { s.Close() })

	return s
}

func TestConnectAndDiscoverTools(t *testing.T) {
	handler := func(_ context.Context, req *mcpsdk.CallToolRequest) (*mcpsdk.CallToolResult, error) {
		return &mcpsdk.CallToolResult{
			Content: []mcpsdk.Content{&mcpsdk.TextContent{Text: "ok"}},
		}, nil
	}

	s := startTestServer(t, map[string]mcpsdk.ToolHandler{
		"tool_a": handler,
		"tool_b": handler,
	})

	tools := s.Tools()
	assert.Len(t, tools, 2)

	names := make(map[string]bool)
	for _, tool := range tools {
		info, err := tool.Info(context.Background())
		require.NoError(t, err)
		names[info.Name] = true
	}
	assert.True(t, names["tool_a"])
	assert.True(t, names["tool_b"])
}

func TestCallTool(t *testing.T) {
	s := startTestServer(t, map[string]mcpsdk.ToolHandler{
		"greet": func(_ context.Context, req *mcpsdk.CallToolRequest) (*mcpsdk.CallToolResult, error) {
			var args struct{ Input string }
			if err := json.Unmarshal(req.Params.Arguments, &args); err != nil {
				return nil, err
			}
			return &mcpsdk.CallToolResult{
				Content: []mcpsdk.Content{&mcpsdk.TextContent{Text: "Hello, " + args.Input}},
			}, nil
		},
	})

	tools := s.Tools()
	require.Len(t, tools, 1)

	result, err := tools[0].InvokableRun(context.Background(), `{"input":"World"}`)
	require.NoError(t, err)
	assert.Equal(t, "Hello, World", result)
}

func TestCallToolError(t *testing.T) {
	s := startTestServer(t, map[string]mcpsdk.ToolHandler{
		"fail": func(_ context.Context, req *mcpsdk.CallToolRequest) (*mcpsdk.CallToolResult, error) {
			result := &mcpsdk.CallToolResult{
				Content: []mcpsdk.Content{&mcpsdk.TextContent{Text: "something went wrong"}},
				IsError: true,
			}
			return result, nil
		},
	})

	tools := s.Tools()
	require.Len(t, tools, 1)

	// MCP tool errors are returned as strings, not Go errors.
	result, err := tools[0].InvokableRun(context.Background(), `{}`)
	require.NoError(t, err)
	assert.Equal(t, "Error: something went wrong", result)
}

func TestToolFilter(t *testing.T) {
	ctx := context.Background()
	handler := func(_ context.Context, req *mcpsdk.CallToolRequest) (*mcpsdk.CallToolResult, error) {
		return &mcpsdk.CallToolResult{
			Content: []mcpsdk.Content{&mcpsdk.TextContent{Text: "ok"}},
		}, nil
	}

	server := mcpsdk.NewServer(&mcpsdk.Implementation{Name: "test-server", Version: "0.1.0"}, nil)
	for _, name := range []string{"allowed", "blocked", "also_allowed"} {
		server.AddTool(&mcpsdk.Tool{
			Name:        name,
			InputSchema: json.RawMessage(`{"type":"object"}`),
		}, handler)
	}

	sTransport, cTransport := mcpsdk.NewInMemoryTransports()
	_, err := server.Connect(ctx, sTransport, nil)
	require.NoError(t, err)

	s, err := Connect(ctx, cTransport, WithToolFilter(func(name string) bool {
		return name != "blocked"
	}))
	require.NoError(t, err)
	defer s.Close()

	assert.Len(t, s.Tools(), 2)
}

func TestOutputToolNameFiltered(t *testing.T) {
	ctx := context.Background()

	server := mcpsdk.NewServer(&mcpsdk.Implementation{Name: "test-server", Version: "0.1.0"}, nil)
	for _, name := range []string{"real_tool", "final_result"} {
		server.AddTool(&mcpsdk.Tool{
			Name:        name,
			InputSchema: json.RawMessage(`{"type":"object"}`),
		}, func(_ context.Context, req *mcpsdk.CallToolRequest) (*mcpsdk.CallToolResult, error) {
			return &mcpsdk.CallToolResult{
				Content: []mcpsdk.Content{&mcpsdk.TextContent{Text: "ok"}},
			}, nil
		})
	}

	sTransport, cTransport := mcpsdk.NewInMemoryTransports()
	_, err := server.Connect(ctx, sTransport, nil)
	require.NoError(t, err)

	s, err := Connect(ctx, cTransport)
	require.NoError(t, err)
	defer s.Close()

	// final_result should be filtered out.
	tools := s.Tools()
	assert.Len(t, tools, 1)
	info, _ := tools[0].Info(ctx)
	assert.Equal(t, "real_tool", info.Name)
}

func TestEmptyArgs(t *testing.T) {
	s := startTestServer(t, map[string]mcpsdk.ToolHandler{
		"simple": func(_ context.Context, req *mcpsdk.CallToolRequest) (*mcpsdk.CallToolResult, error) {
			return &mcpsdk.CallToolResult{
				Content: []mcpsdk.Content{&mcpsdk.TextContent{Text: "done"}},
			}, nil
		},
	})

	tools := s.Tools()
	require.Len(t, tools, 1)

	// Calling with empty args should work.
	result, err := tools[0].InvokableRun(context.Background(), "")
	require.NoError(t, err)
	assert.Equal(t, "done", result)

	// Calling with empty object should also work.
	result, err = tools[0].InvokableRun(context.Background(), "{}")
	require.NoError(t, err)
	assert.Equal(t, "done", result)
}

func TestWithMCPServerIntegration(t *testing.T) {
	s := startTestServer(t, map[string]mcpsdk.ToolHandler{
		"search": func(_ context.Context, req *mcpsdk.CallToolRequest) (*mcpsdk.CallToolResult, error) {
			return &mcpsdk.CallToolResult{
				Content: []mcpsdk.Content{&mcpsdk.TextContent{Text: "found 3 results"}},
			}, nil
		},
	})

	// Create a TestModel that calls the search tool, then returns text.
	tm := testutil.NewTestModel(
		// First response: call the MCP tool.
		testutil.TestResponse{
			ToolCalls: []schema.ToolCall{{
				ID: "call_1",
				Function: schema.FunctionCall{
					Name:      "search",
					Arguments: `{"input":"test query"}`,
				},
			}},
		},
		// Second response: return final text after seeing tool result.
		testutil.TestResponse{
			Text: "Search returned: found 3 results",
		},
	)

	a := agent.New[agent.NoDeps, string](tm,
		WithMCPServer[agent.NoDeps, string](s),
	)

	result, err := a.Run(context.Background(), "search for something", agent.NoDeps{})
	require.NoError(t, err)
	assert.Equal(t, "Search returned: found 3 results", result.Output)
	assert.Equal(t, 2, tm.CallCount())

	// Verify the MCP tool was in the tool list.
	testutil.AssertToolRegistered(t, tm, "search")
}

func TestSchemaConversion(t *testing.T) {
	// Test round-tripping a complex schema.
	inputSchema := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"query": map[string]any{
				"type":        "string",
				"description": "Search query",
			},
			"limit": map[string]any{
				"type":        "integer",
				"description": "Max results",
			},
		},
		"required": []any{"query"},
	}

	paramsOneOf, err := convertInputSchema(inputSchema)
	require.NoError(t, err)
	require.NotNil(t, paramsOneOf)

	// Verify the schema can be used to build ToolInfo.
	info := &schema.ToolInfo{
		Name:        "test",
		Desc:        "test tool",
		ParamsOneOf: paramsOneOf,
	}
	assert.Equal(t, "test", info.Name)
}

func TestConvertInputSchema_Nil(t *testing.T) {
	result, err := convertInputSchema(nil)
	require.NoError(t, err)
	assert.Nil(t, result)
}

func TestHeaderTransport(t *testing.T) {
	ht := &headerTransport{
		headers: map[string]string{
			"X-Custom-Auth": "token123",
			"X-Other":       "value",
		},
		rt: http.DefaultTransport,
	}

	// Create a test request and verify headers are injected.
	req, _ := http.NewRequest("POST", "http://localhost/test", nil)
	// We can't actually send the request, but we can verify the RoundTrip
	// modifies the request headers by wrapping with a checking transport.
	called := false
	ht.rt = roundTripFunc(func(req *http.Request) (*http.Response, error) {
		called = true
		assert.Equal(t, "token123", req.Header.Get("X-Custom-Auth"))
		assert.Equal(t, "value", req.Header.Get("X-Other"))
		return &http.Response{StatusCode: 200}, nil
	})

	_, err := ht.RoundTrip(req)
	require.NoError(t, err)
	assert.True(t, called)
}

// roundTripFunc adapts a function to http.RoundTripper.
type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) { return f(req) }

func TestFormatCallToolResult(t *testing.T) {
	tests := []struct {
		name     string
		result   *mcpsdk.CallToolResult
		expected string
	}{
		{
			name: "single text",
			result: &mcpsdk.CallToolResult{
				Content: []mcpsdk.Content{&mcpsdk.TextContent{Text: "hello"}},
			},
			expected: "hello",
		},
		{
			name: "multiple text",
			result: &mcpsdk.CallToolResult{
				Content: []mcpsdk.Content{
					&mcpsdk.TextContent{Text: "line 1"},
					&mcpsdk.TextContent{Text: "line 2"},
				},
			},
			expected: "line 1\nline 2",
		},
		{
			name: "error result",
			result: &mcpsdk.CallToolResult{
				Content: []mcpsdk.Content{&mcpsdk.TextContent{Text: "not found"}},
				IsError: true,
			},
			expected: "Error: not found",
		},
		{
			name: "image content",
			result: &mcpsdk.CallToolResult{
				Content: []mcpsdk.Content{&mcpsdk.ImageContent{MIMEType: "image/png"}},
			},
			expected: "[Image: image/png]",
		},
		{
			name:     "empty content",
			result:   &mcpsdk.CallToolResult{},
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, formatCallToolResult(tt.result))
		})
	}
}
