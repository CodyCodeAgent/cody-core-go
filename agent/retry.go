package agent

import "fmt"

// ErrModelRetry is a special error type that triggers a model retry.
// When a tool function or output validator returns this error:
// 1. The error message is appended to the conversation as feedback
// 2. The model is called again to correct its output
type ErrModelRetry struct {
	Message string
}

func (e *ErrModelRetry) Error() string {
	return "model retry: " + e.Message
}

// NewModelRetry creates a new ErrModelRetry with the given feedback message.
func NewModelRetry(msg string) *ErrModelRetry {
	return &ErrModelRetry{Message: msg}
}

// IsModelRetry checks if an error is an ErrModelRetry.
func IsModelRetry(err error) (*ErrModelRetry, bool) {
	if e, ok := err.(*ErrModelRetry); ok {
		return e, true
	}
	return nil, false
}

// ToolRetriesExceededError is returned when a tool exceeds its max retry count.
type ToolRetriesExceededError struct {
	ToolName   string
	MaxRetries int
	LastError  error
}

func (e *ToolRetriesExceededError) Error() string {
	return fmt.Sprintf("tool %q exceeded max retries count of %d", e.ToolName, e.MaxRetries)
}

func (e *ToolRetriesExceededError) Unwrap() error {
	return e.LastError
}

// ResultRetriesExceededError is returned when output validation exceeds max retries.
type ResultRetriesExceededError struct {
	MaxRetries int
	LastError  error
}

func (e *ResultRetriesExceededError) Error() string {
	return fmt.Sprintf("exceeded maximum result retries (%d)", e.MaxRetries)
}

func (e *ResultRetriesExceededError) Unwrap() error {
	return e.LastError
}

// UsageLimitExceededError is returned when usage limits are exceeded.
type UsageLimitExceededError struct {
	Message string
}

func (e *UsageLimitExceededError) Error() string {
	return "usage limit exceeded: " + e.Message
}
