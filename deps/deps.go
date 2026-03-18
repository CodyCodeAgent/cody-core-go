// Package deps provides helper functions for dependency injection in the agent framework.
// It re-exports key functions from the agent package for convenience.
package deps

import (
	"context"

	"github.com/codycode/cody-core-go/agent"
)

// GetDeps extracts typed dependencies from a context.Context.
// This is the primary way for Eino Tool implementations to access agent dependencies.
//
// Usage in a tool function:
//
//	func myTool(ctx context.Context, args MyArgs) (string, error) {
//	    deps, ok := deps.GetDeps[MyDeps](ctx)
//	    if !ok {
//	        return "", errors.New("dependencies not found")
//	    }
//	    // use deps.DB, deps.APIKey, etc.
//	}
func GetDeps[D any](ctx context.Context) (D, bool) {
	return agent.GetDeps[D](ctx)
}

// GetRunContext extracts the full RunContext from a context.Context.
// Use this when you need access to usage tracking or metadata
// in addition to dependencies.
func GetRunContext[D any](ctx context.Context) (*agent.RunContext[D], bool) {
	return agent.GetRunContext[D](ctx)
}

// GetMetadata extracts the run-level metadata from a context.Context.
// Returns nil if no metadata was set or the RunContext is not found.
func GetMetadata[D any](ctx context.Context) map[string]any {
	return agent.GetMetadata[D](ctx)
}
