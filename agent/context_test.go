package agent

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestRunContext_InjectionAndExtraction(t *testing.T) {
	type MyDeps struct {
		DB string
	}

	rc := &RunContext[MyDeps]{
		Ctx:  context.Background(),
		Deps: MyDeps{DB: "test_db"},
	}

	ctx := withRunContext[MyDeps](context.Background(), rc)

	extracted, ok := GetRunContext[MyDeps](ctx)
	assert.True(t, ok)
	assert.Equal(t, "test_db", extracted.Deps.DB)
}

func TestGetDeps(t *testing.T) {
	type MyDeps struct {
		APIKey string
	}

	rc := &RunContext[MyDeps]{
		Ctx:  context.Background(),
		Deps: MyDeps{APIKey: "secret"},
	}

	ctx := withRunContext[MyDeps](context.Background(), rc)

	deps, ok := GetDeps[MyDeps](ctx)
	assert.True(t, ok)
	assert.Equal(t, "secret", deps.APIKey)
}

func TestGetDeps_NotFound(t *testing.T) {
	type MyDeps struct {
		APIKey string
	}

	deps, ok := GetDeps[MyDeps](context.Background())
	assert.False(t, ok)
	assert.Equal(t, MyDeps{}, deps)
}

func TestUsageTracker(t *testing.T) {
	tracker := &UsageTracker{}
	tracker.AddTokens(100, 50, 150)
	tracker.AddTokens(200, 100, 300)

	assert.Equal(t, 2, tracker.Requests())
	assert.Equal(t, 300, tracker.RequestTokens())
	assert.Equal(t, 150, tracker.ResponseTokens())
	assert.Equal(t, 450, tracker.TotalTokens())
}

func TestUsageTracker_Snapshot(t *testing.T) {
	tracker := &UsageTracker{}
	tracker.AddTokens(100, 50, 150)

	requests, reqTokens, respTokens, totalTokens := tracker.Snapshot()
	assert.Equal(t, 1, requests)
	assert.Equal(t, 100, reqTokens)
	assert.Equal(t, 50, respTokens)
	assert.Equal(t, 150, totalTokens)
}
