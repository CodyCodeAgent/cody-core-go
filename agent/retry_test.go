package agent

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestErrModelRetry(t *testing.T) {
	err := NewModelRetry("score must be positive")
	assert.Equal(t, "model retry: score must be positive", err.Error())

	retry, ok := IsModelRetry(err)
	assert.True(t, ok)
	assert.Equal(t, "score must be positive", retry.Message)
}

func TestIsModelRetry_NotModelRetry(t *testing.T) {
	err := assert.AnError
	_, ok := IsModelRetry(err)
	assert.False(t, ok)
}

func TestToolRetriesExceededError(t *testing.T) {
	err := &ToolRetriesExceededError{
		ToolName:   "search",
		MaxRetries: 3,
		LastError:  NewModelRetry("bad input"),
	}
	assert.Contains(t, err.Error(), "search")
	assert.Contains(t, err.Error(), "3")
	assert.Equal(t, "model retry: bad input", err.Unwrap().Error())
}

func TestResultRetriesExceededError(t *testing.T) {
	err := &ResultRetriesExceededError{
		MaxRetries: 2,
		LastError:  NewModelRetry("invalid output"),
	}
	assert.Contains(t, err.Error(), "2")
}

func TestUsageLimitExceededError(t *testing.T) {
	err := &UsageLimitExceededError{Message: "request limit 5 reached"}
	assert.Contains(t, err.Error(), "request limit")
}
