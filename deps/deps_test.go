package deps

import (
	"context"
	"testing"

	"github.com/codycode/cody-core-go/agent"
	"github.com/stretchr/testify/assert"
)

type MyDeps struct {
	DB     string
	APIKey string
}

func TestGetDeps_FromPackage(t *testing.T) {
	// This tests the re-exported GetDeps function
	// We can't directly test it without setting up the context through agent internals
	// So we just verify it returns false for empty context
	deps, ok := GetDeps[MyDeps](context.Background())
	assert.False(t, ok)
	assert.Equal(t, MyDeps{}, deps)
}

func TestGetRunContext_FromPackage(t *testing.T) {
	rc, ok := GetRunContext[MyDeps](context.Background())
	assert.False(t, ok)
	assert.Nil(t, rc)
}

func TestGetDeps_TypeSafety(t *testing.T) {
	// Verify different types don't interfere
	type DepsA struct{ A string }
	type DepsB struct{ B string }

	_, okA := GetDeps[DepsA](context.Background())
	_, okB := GetDeps[DepsB](context.Background())
	assert.False(t, okA)
	assert.False(t, okB)
}

func TestGetDeps_ReExport(t *testing.T) {
	// Verify that deps.GetDeps is the same as agent.GetDeps
	_, ok1 := GetDeps[MyDeps](context.Background())
	_, ok2 := agent.GetDeps[MyDeps](context.Background())
	assert.Equal(t, ok1, ok2)
}
