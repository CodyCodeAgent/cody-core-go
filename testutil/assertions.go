package testutil

import (
	"strings"
	"testing"

	"github.com/cloudwego/eino/schema"
)

// AssertToolCalled checks that a tool with the given name was called in the model's recorded calls.
func AssertToolCalled(t *testing.T, tm *TestModel, toolName string) {
	t.Helper()
	for _, call := range tm.AllCalls() {
		for _, msg := range call.Messages {
			if msg.Role == schema.Tool && msg.ToolName == toolName {
				return
			}
			for _, tc := range msg.ToolCalls {
				if tc.Function.Name == toolName {
					return
				}
			}
		}
	}
	t.Errorf("expected tool %q to be called, but it was not", toolName)
}

// AssertToolRegistered checks that a tool with the given name was registered (sent to model).
func AssertToolRegistered(t *testing.T, tm *TestModel, toolName string) {
	t.Helper()
	for _, call := range tm.AllCalls() {
		for _, ti := range call.Tools {
			if ti.Name == toolName {
				return
			}
		}
	}
	t.Errorf("expected tool %q to be registered, but it was not", toolName)
}

// AssertSystemPromptContains checks that at least one system message contains the substring.
func AssertSystemPromptContains(t *testing.T, tm *TestModel, substr string) {
	t.Helper()
	for _, call := range tm.AllCalls() {
		for _, msg := range call.Messages {
			if msg.Role == schema.System && strings.Contains(msg.Content, substr) {
				return
			}
		}
	}
	t.Errorf("expected system prompt to contain %q, but it did not", substr)
}

// AssertNoSystemPrompt checks that no system messages were sent.
func AssertNoSystemPrompt(t *testing.T, tm *TestModel) {
	t.Helper()
	for _, call := range tm.AllCalls() {
		for _, msg := range call.Messages {
			if msg.Role == schema.System {
				t.Errorf("expected no system prompt, but found: %q", msg.Content)
				return
			}
		}
	}
}

// AssertUserPromptSent checks that a user message with the given content was sent.
func AssertUserPromptSent(t *testing.T, tm *TestModel, content string) {
	t.Helper()
	for _, call := range tm.AllCalls() {
		for _, msg := range call.Messages {
			if msg.Role == schema.User && msg.Content == content {
				return
			}
		}
	}
	t.Errorf("expected user prompt %q to be sent, but it was not", content)
}
