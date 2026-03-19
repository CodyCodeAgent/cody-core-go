// Package mcp bridges MCP (Model Context Protocol) servers with cody-core-go agents.
//
// It converts MCP server tools into Eino InvokableTools, allowing any MCP server's
// tools to be used as agent tools without custom wrappers.
//
// Usage:
//
//	transport := mcp.NewCommandTransport(exec.Command("my-mcp-server"))
//	server, err := mcptools.Connect(ctx, transport)
//	defer server.Close()
//
//	a := agent.New[agent.NoDeps, string](chatModel,
//	    mcptools.WithMCPServer[agent.NoDeps, string](server),
//	)
package mcp
