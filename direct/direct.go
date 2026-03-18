// Package direct provides lightweight functions for making direct model requests
// without the full Agent machinery (no tool execution, no retries, no agent loop).
package direct

import (
	"context"
	"fmt"

	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"

	"github.com/codycode/cody-core-go/output"
)

// RequestOption configures a direct model request.
type RequestOption func(*requestConfig)

type requestConfig struct {
	systemPrompt  string
	messages      []*schema.Message
	modelSettings map[string]any
}

// WithSystemPrompt sets a system prompt for the request.
func WithSystemPrompt(prompt string) RequestOption {
	return func(c *requestConfig) {
		c.systemPrompt = prompt
	}
}

// WithMessages sets custom messages instead of using the prompt string.
func WithMessages(msgs []*schema.Message) RequestOption {
	return func(c *requestConfig) {
		c.messages = msgs
	}
}

// WithModelSettings sets model parameters for the request.
func WithModelSettings(settings map[string]any) RequestOption {
	return func(c *requestConfig) {
		c.modelSettings = settings
	}
}

// RequestText makes a simple text request to the model without structured output.
func RequestText(ctx context.Context, chatModel model.BaseChatModel, prompt string, opts ...RequestOption) (string, error) {
	cfg := &requestConfig{}
	for _, o := range opts {
		o(cfg)
	}

	messages := buildMessages(cfg, prompt)
	modelOpts := buildModelOpts(cfg, nil)

	resp, err := chatModel.Generate(ctx, messages, modelOpts...)
	if err != nil {
		return "", fmt.Errorf("direct request text: %w", err)
	}

	return resp.Content, nil
}

// Request makes a structured output request to the model.
// For primitive types (int, bool, etc.), the output is wrapped in {"result": ...}.
func Request[T any](ctx context.Context, chatModel model.BaseChatModel, prompt string, opts ...RequestOption) (T, error) {
	var zero T

	cfg := &requestConfig{}
	for _, o := range opts {
		o(cfg)
	}

	// Build output tool
	paramsOneOf, err := output.BuildParamsOneOf[T]()
	if err != nil {
		return zero, fmt.Errorf("build output schema: %w", err)
	}
	outTool := output.GenerateOutputTool[T](paramsOneOf)
	outToolInfo, err := outTool.Info(ctx)
	if err != nil {
		return zero, fmt.Errorf("get output tool info: %w", err)
	}

	messages := buildMessages(cfg, prompt)
	modelOpts := buildModelOpts(cfg, []*schema.ToolInfo{outToolInfo})

	resp, err := chatModel.Generate(ctx, messages, modelOpts...)
	if err != nil {
		return zero, fmt.Errorf("direct request: %w", err)
	}

	// Find output tool call
	for _, tc := range resp.ToolCalls {
		if output.IsOutputToolName(tc.Function.Name) {
			result, parseErr := output.ParseStructuredOutput[T]([]byte(tc.Function.Arguments))
			if parseErr != nil {
				return zero, fmt.Errorf("parse structured output: %w", parseErr)
			}
			return result, nil
		}
	}

	return zero, fmt.Errorf("model did not call the output tool; got text: %q", resp.Content)
}

func buildMessages(cfg *requestConfig, prompt string) []*schema.Message {
	if cfg.messages != nil {
		return cfg.messages
	}

	var msgs []*schema.Message
	if cfg.systemPrompt != "" {
		msgs = append(msgs, &schema.Message{
			Role:    schema.System,
			Content: cfg.systemPrompt,
		})
	}
	msgs = append(msgs, &schema.Message{
		Role:    schema.User,
		Content: prompt,
	})
	return msgs
}

func buildModelOpts(cfg *requestConfig, toolInfos []*schema.ToolInfo) []model.Option {
	var opts []model.Option
	if len(toolInfos) > 0 {
		opts = append(opts, model.WithTools(toolInfos))
	}
	if cfg.modelSettings != nil {
		if v, ok := cfg.modelSettings["temperature"]; ok {
			if f, ok := toFloat32(v); ok {
				opts = append(opts, model.WithTemperature(f))
			}
		}
		if v, ok := cfg.modelSettings["max_tokens"]; ok {
			if n, ok := toInt(v); ok {
				opts = append(opts, model.WithMaxTokens(n))
			}
		}
		if v, ok := cfg.modelSettings["top_p"]; ok {
			if f, ok := toFloat32(v); ok {
				opts = append(opts, model.WithTopP(f))
			}
		}
		if v, ok := cfg.modelSettings["stop"]; ok {
			if s, ok := v.([]string); ok {
				opts = append(opts, model.WithStop(s))
			}
		}
	}
	return opts
}

// Type conversion helpers (consistent with agent package).
func toFloat32(v any) (float32, bool) {
	switch val := v.(type) {
	case float32:
		return val, true
	case float64:
		return float32(val), true
	case int:
		return float32(val), true
	}
	return 0, false
}

func toInt(v any) (int, bool) {
	switch val := v.(type) {
	case int:
		return val, true
	case int64:
		return int(val), true
	case float64:
		return int(val), true
	}
	return 0, false
}
