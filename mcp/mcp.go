package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/schema"
	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/CodyCodeAgent/cody-core-go/agent"
	"github.com/CodyCodeAgent/cody-core-go/output"
)

// Server represents a connected MCP server whose tools can be used by an agent.
type Server struct {
	session *mcpsdk.ClientSession
	tools   []tool.InvokableTool
}

// ServerOption configures a Server during connection.
type ServerOption func(*serverConfig)

type serverConfig struct {
	clientName    string
	clientVersion string
	toolFilter    func(name string) bool
}

// WithClientInfo sets the client name and version reported to the MCP server.
func WithClientInfo(name, version string) ServerOption {
	return func(c *serverConfig) {
		c.clientName = name
		c.clientVersion = version
	}
}

// WithToolFilter sets a filter function that determines which MCP tools to expose.
// Tools for which the filter returns false are excluded.
func WithToolFilter(fn func(name string) bool) ServerOption {
	return func(c *serverConfig) {
		c.toolFilter = fn
	}
}

// Connect establishes a connection to an MCP server and discovers its tools.
func Connect(ctx context.Context, transport mcpsdk.Transport, opts ...ServerOption) (*Server, error) {
	cfg := &serverConfig{
		clientName:    "cody-core-go",
		clientVersion: "1.0.0",
	}
	for _, o := range opts {
		o(cfg)
	}

	client := mcpsdk.NewClient(&mcpsdk.Implementation{
		Name:    cfg.clientName,
		Version: cfg.clientVersion,
	}, nil)

	session, err := client.Connect(ctx, transport, nil)
	if err != nil {
		return nil, fmt.Errorf("mcp connect: %w", err)
	}

	// Discover tools using the iterator (handles pagination automatically).
	var tools []tool.InvokableTool
	for t, err := range session.Tools(ctx, nil) {
		if err != nil {
			return nil, fmt.Errorf("mcp list tools: %w", err)
		}

		// Skip output tool name collisions.
		if output.IsOutputToolName(t.Name) {
			continue
		}

		// Apply tool filter.
		if cfg.toolFilter != nil && !cfg.toolFilter(t.Name) {
			continue
		}

		mt, err := newMCPTool(t, session)
		if err != nil {
			return nil, fmt.Errorf("mcp convert tool %q: %w", t.Name, err)
		}
		tools = append(tools, mt)
	}

	return &Server{
		session: session,
		tools:   tools,
	}, nil
}

// HTTPOption configures an HTTP MCP connection.
type HTTPOption func(*httpConfig)

type httpConfig struct {
	headers    map[string]string
	httpClient *http.Client
}

// WithHeaders sets custom HTTP headers sent with every request to the MCP server.
// Useful for authentication tokens or tool filtering (e.g., Feishu/Lark MCP).
func WithHeaders(headers map[string]string) HTTPOption {
	return func(c *httpConfig) {
		c.headers = headers
	}
}

// WithHTTPClient sets a custom http.Client for the MCP connection.
func WithHTTPClient(client *http.Client) HTTPOption {
	return func(c *httpConfig) {
		c.httpClient = client
	}
}

// headerTransport injects custom headers into every HTTP request.
type headerTransport struct {
	headers map[string]string
	rt      http.RoundTripper
}

func (t *headerTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	for k, v := range t.headers {
		req.Header.Set(k, v)
	}
	return t.rt.RoundTrip(req)
}

// ConnectHTTP connects to an MCP server over HTTP (Streamable HTTP transport).
// This is a convenience wrapper around Connect for HTTP-based MCP servers like Feishu/Lark.
//
//	server, err := mcptools.ConnectHTTP(ctx, "https://mcp.feishu.cn/mcp",
//	    mcptools.WithHeaders(map[string]string{
//	        "X-Lark-MCP-UAT":           uat,
//	        "X-Lark-MCP-Allowed-Tools": "fetch-doc",
//	    }),
//	)
func ConnectHTTP(ctx context.Context, endpoint string, httpOpts []HTTPOption, serverOpts ...ServerOption) (*Server, error) {
	cfg := &httpConfig{}
	for _, o := range httpOpts {
		o(cfg)
	}

	client := cfg.httpClient
	if client == nil {
		client = &http.Client{}
	}

	if len(cfg.headers) > 0 {
		base := client.Transport
		if base == nil {
			base = http.DefaultTransport
		}
		client.Transport = &headerTransport{
			headers: cfg.headers,
			rt:      base,
		}
	}

	transport := &mcpsdk.StreamableClientTransport{
		Endpoint:   endpoint,
		HTTPClient: client,
	}

	return Connect(ctx, transport, serverOpts...)
}

// Close shuts down the MCP session.
func (s *Server) Close() error {
	return s.session.Close()
}

// Tools returns the discovered MCP tools as Eino InvokableTools.
func (s *Server) Tools() []tool.InvokableTool {
	return s.tools
}

// WithMCPServer registers all tools from a connected MCP server with the agent.
func WithMCPServer[D, O any](server *Server) agent.Option[D, O] {
	return func(a *agent.Agent[D, O]) {
		for _, t := range server.Tools() {
			agent.WithTool[D, O](t)(a)
		}
	}
}

// mcpTool wraps a single MCP tool as an Eino InvokableTool.
type mcpTool struct {
	info    *schema.ToolInfo
	session *mcpsdk.ClientSession
	name    string
}

func newMCPTool(t *mcpsdk.Tool, session *mcpsdk.ClientSession) (*mcpTool, error) {
	paramsOneOf, err := convertInputSchema(t.InputSchema)
	if err != nil {
		return nil, err
	}

	return &mcpTool{
		info: &schema.ToolInfo{
			Name:        t.Name,
			Desc:        t.Description,
			ParamsOneOf: paramsOneOf,
		},
		session: session,
		name:    t.Name,
	}, nil
}

// Info returns the tool's schema information.
func (m *mcpTool) Info(_ context.Context) (*schema.ToolInfo, error) {
	return m.info, nil
}

// InvokableRun calls the MCP server tool and returns the result as a string.
func (m *mcpTool) InvokableRun(ctx context.Context, argumentsInJSON string, _ ...tool.Option) (string, error) {
	// Parse arguments from JSON string to map for the MCP SDK.
	var args map[string]any
	if argumentsInJSON != "" && argumentsInJSON != "{}" {
		if err := json.Unmarshal([]byte(argumentsInJSON), &args); err != nil {
			return "", fmt.Errorf("parse arguments for tool %q: %w", m.name, err)
		}
	}

	result, err := m.session.CallTool(ctx, &mcpsdk.CallToolParams{
		Name:      m.name,
		Arguments: args,
	})
	if err != nil {
		return "", fmt.Errorf("call MCP tool %q: %w", m.name, err)
	}

	return formatCallToolResult(result), nil
}

// formatCallToolResult extracts text from a CallToolResult.
// If IsError is true, the error text is returned as a string (not a Go error)
// so the model can see it and self-correct.
func formatCallToolResult(result *mcpsdk.CallToolResult) string {
	var parts []string
	for _, c := range result.Content {
		switch v := c.(type) {
		case *mcpsdk.TextContent:
			parts = append(parts, v.Text)
		case *mcpsdk.ImageContent:
			parts = append(parts, fmt.Sprintf("[Image: %s]", v.MIMEType))
		case *mcpsdk.AudioContent:
			parts = append(parts, fmt.Sprintf("[Audio: %s]", v.MIMEType))
		default:
			parts = append(parts, "[Unsupported content type]")
		}
	}

	text := strings.Join(parts, "\n")
	if result.IsError {
		return "Error: " + text
	}
	return text
}
