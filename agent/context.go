package agent

import (
	"context"
)

// NoDeps is a placeholder type for agents that don't need dependency injection.
type NoDeps struct{}

// RunContext carries runtime dependencies and metadata for a single agent run.
// It is passed to system prompt functions and tool functions via context.Value.
type RunContext[D any] struct {
	// Ctx is the underlying context.Context.
	Ctx context.Context
	// Deps holds the typed dependencies for this run.
	Deps D
	// Usage tracks token usage across the run.
	Usage *UsageTracker
	// Retry is the current result retry count.
	Retry int
}

// UsageTracker accumulates token usage across multiple model calls in a single run.
type UsageTracker struct {
	Requests        int
	RequestTokens   int
	ResponseTokens  int
	TotalTokens     int
}

// AddTokens records token usage from a single model call.
func (u *UsageTracker) AddTokens(request, response, total int) {
	u.Requests++
	u.RequestTokens += request
	u.ResponseTokens += response
	u.TotalTokens += total
}

// Usage represents the final token usage summary for a run.
type Usage struct {
	Requests       int `json:"requests"`
	RequestTokens  int `json:"request_tokens"`
	ResponseTokens int `json:"response_tokens"`
	TotalTokens    int `json:"total_tokens"`
}

// runContextKey is the context key for RunContext injection.
// Using a generic struct type prevents key collisions between different D types.
type runContextKey[D any] struct{}

// withRunContext injects a RunContext into a context.Context.
func withRunContext[D any](ctx context.Context, rc *RunContext[D]) context.Context {
	return context.WithValue(ctx, runContextKey[D]{}, rc)
}

// GetRunContext extracts the full RunContext from a context.Context.
func GetRunContext[D any](ctx context.Context) (*RunContext[D], bool) {
	rc, ok := ctx.Value(runContextKey[D]{}).(*RunContext[D])
	return rc, ok
}

// GetDeps extracts the typed dependencies from a context.Context.
// This is the primary way for Eino Tool implementations to access dependencies.
func GetDeps[D any](ctx context.Context) (D, bool) {
	rc, ok := GetRunContext[D](ctx)
	if !ok {
		var zero D
		return zero, false
	}
	return rc.Deps, true
}
