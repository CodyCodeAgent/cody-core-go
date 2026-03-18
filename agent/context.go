package agent

import (
	"context"
	"sync"
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
	// Metadata holds additional run-level metadata.
	Metadata map[string]any
	// Retry is the current result retry count.
	Retry int
}

// UsageTracker accumulates token usage across multiple model calls in a single run.
// All methods are safe for concurrent use.
type UsageTracker struct {
	mu             sync.Mutex
	requests       int
	requestTokens  int
	responseTokens int
	totalTokens    int
}

// AddTokens records token usage from a single model call.
func (u *UsageTracker) AddTokens(request, response, total int) {
	u.mu.Lock()
	defer u.mu.Unlock()
	u.requests++
	u.requestTokens += request
	u.responseTokens += response
	u.totalTokens += total
}

// Snapshot returns a consistent snapshot of current usage values.
func (u *UsageTracker) Snapshot() (requests, requestTokens, responseTokens, totalTokens int) {
	u.mu.Lock()
	defer u.mu.Unlock()
	return u.requests, u.requestTokens, u.responseTokens, u.totalTokens
}

// Requests returns the current number of model calls.
func (u *UsageTracker) Requests() int {
	u.mu.Lock()
	defer u.mu.Unlock()
	return u.requests
}

// RequestTokens returns the total prompt tokens used.
func (u *UsageTracker) RequestTokens() int {
	u.mu.Lock()
	defer u.mu.Unlock()
	return u.requestTokens
}

// ResponseTokens returns the total completion tokens used.
func (u *UsageTracker) ResponseTokens() int {
	u.mu.Lock()
	defer u.mu.Unlock()
	return u.responseTokens
}

// TotalTokens returns the total tokens used.
func (u *UsageTracker) TotalTokens() int {
	u.mu.Lock()
	defer u.mu.Unlock()
	return u.totalTokens
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
