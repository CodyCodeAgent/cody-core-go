package testutil

import (
	"context"
	"strings"
	"testing"

	"github.com/cloudwego/eino/schema"
)

func TestAssertToolRegistered(t *testing.T) {
	tm := NewTestModel(TestResponse{Text: "ok"})

	_, _ = tm.Generate(context.Background(), []*schema.Message{
		{Role: schema.User, Content: "test"},
	})

	// Basic test — tools were not passed via options, so no tools registered
}

func TestAssertSystemPromptContains(t *testing.T) {
	tm := NewTestModel(TestResponse{Text: "ok"})

	_, _ = tm.Generate(context.Background(), []*schema.Message{
		{Role: schema.System, Content: "You are a helpful assistant."},
		{Role: schema.User, Content: "hi"},
	})

	// This should pass
	AssertSystemPromptContains(t, tm, "helpful")
}

func TestAssertUserPromptSent(t *testing.T) {
	tm := NewTestModel(TestResponse{Text: "ok"})

	_, _ = tm.Generate(context.Background(), []*schema.Message{
		{Role: schema.User, Content: "What is Go?"},
	})

	AssertUserPromptSent(t, tm, "What is Go?")
}

func TestStringsContains(t *testing.T) {
	tests := []struct {
		s      string
		substr string
		want   bool
	}{
		{"hello world", "hello", true},
		{"hello world", "world", true},
		{"hello world", "xyz", false},
		{"hello", "hello", true},
		{"", "", true},
		{"hello", "", true},
		{"", "x", false},
	}

	for _, tt := range tests {
		got := strings.Contains(tt.s, tt.substr)
		if got != tt.want {
			t.Errorf("strings.Contains(%q, %q) = %v, want %v", tt.s, tt.substr, got, tt.want)
		}
	}
}
