package agent

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/schema"

	"github.com/codycode/cody-core-go/output"
)

// SystemPromptFunc generates a dynamic system prompt based on runtime dependencies.
type SystemPromptFunc[D any] func(ctx *RunContext[D]) (string, error)

// PrepareFunc is called before each model invocation to dynamically modify a tool's parameter schema.
type PrepareFunc[D any] func(ctx *RunContext[D], toolInfo *schema.ToolInfo) (*schema.ToolInfo, error)

// toolEntry holds a tool along with its optional prepare callback.
type toolEntry[D any] struct {
	tool    tool.InvokableTool
	prepare PrepareFunc[D]
}

// Option configures an Agent during construction.
type Option[D, O any] func(a *Agent[D, O])

// WithSystemPrompt adds a static system prompt.
func WithSystemPrompt[D, O any](prompt string) Option[D, O] {
	return func(a *Agent[D, O]) {
		a.staticPrompts = append(a.staticPrompts, prompt)
	}
}

// WithDynamicSystemPrompt adds a dynamic system prompt that is evaluated at run time.
func WithDynamicSystemPrompt[D, O any](fn SystemPromptFunc[D]) Option[D, O] {
	return func(a *Agent[D, O]) {
		a.systemPrompts = append(a.systemPrompts, fn)
	}
}

// WithTool adds an existing Eino InvokableTool to the agent.
func WithTool[D, O any](t tool.InvokableTool) Option[D, O] {
	return func(a *Agent[D, O]) {
		a.tools = append(a.tools, toolEntry[D]{tool: t})
	}
}

// ToolOption configures a tool registered via WithToolFunc.
type ToolOption[D any] func(*toolFuncConfig[D])

// WithPrepare sets a prepare callback for dynamic tool schema modification.
func WithPrepare[D any](fn PrepareFunc[D]) ToolOption[D] {
	return func(c *toolFuncConfig[D]) {
		c.prepare = fn
	}
}

type toolFuncConfig[D any] struct {
	prepare PrepareFunc[D]
}

// wrappedTool implements tool.InvokableTool for WithToolFunc-created tools.
type wrappedTool struct {
	info    *schema.ToolInfo
	runFunc func(ctx context.Context, argsJSON string) (string, error)
}

func (w *wrappedTool) Info(_ context.Context) (*schema.ToolInfo, error) {
	return w.info, nil
}

func (w *wrappedTool) InvokableRun(ctx context.Context, argumentsInJSON string, _ ...tool.Option) (string, error) {
	return w.runFunc(ctx, argumentsInJSON)
}

// WithToolFunc creates and registers a tool from a function with typed arguments and dependency injection.
func WithToolFunc[D, O any, Args any](
	name, desc string,
	fn func(ctx *RunContext[D], args Args) (string, error),
	opts ...ToolOption[D],
) Option[D, O] {
	cfg := &toolFuncConfig[D]{}
	for _, o := range opts {
		o(cfg)
	}

	return func(a *Agent[D, O]) {
		// Build tool info from Args struct
		paramsOneOf, err := output.BuildParamsOneOf[Args]()
		if err != nil {
			// Store error to be returned during Run
			a.initErrors = append(a.initErrors, fmt.Errorf("build tool %q schema: %w", name, err))
			return
		}

		info := &schema.ToolInfo{
			Name:        name,
			Desc:        desc,
			ParamsOneOf: paramsOneOf,
		}

		wt := &wrappedTool{
			info: info,
			runFunc: func(ctx context.Context, argsJSON string) (string, error) {
				// Extract RunContext from context
				rc, ok := GetRunContext[D](ctx)
				if !ok {
					return "", fmt.Errorf("RunContext not found in context for tool %q", name)
				}

				// Parse arguments
				var args Args
				if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
					return "", fmt.Errorf("parse args for tool %q: %w", name, err)
				}

				return fn(rc, args)
			},
		}

		a.tools = append(a.tools, toolEntry[D]{
			tool:    wt,
			prepare: cfg.prepare,
		})
	}
}

// WithOutputMode sets the structured output mode (Tool, Native, Prompted).
func WithOutputMode[D, O any](mode output.Mode) Option[D, O] {
	return func(a *Agent[D, O]) {
		a.outputMode = mode
	}
}

// WithOutputValidator adds an output validation function.
func WithOutputValidator[D, O any](fn output.ValidatorFunc[O]) Option[D, O] {
	return func(a *Agent[D, O]) {
		a.outputValidators = append(a.outputValidators, fn)
	}
}

// WithMaxRetries sets the maximum number of retries for tool calls (per-tool).
// Default is 1.
func WithMaxRetries[D, O any](n int) Option[D, O] {
	return func(a *Agent[D, O]) {
		a.maxToolRetries = n
	}
}

// WithMaxResultRetries sets the maximum number of retries for output validation.
// Default is 1.
func WithMaxResultRetries[D, O any](n int) Option[D, O] {
	return func(a *Agent[D, O]) {
		a.maxResultRetries = n
	}
}

// WithModelSettings sets model parameters (temperature, max_tokens, etc.).
func WithModelSettings[D, O any](settings map[string]any) Option[D, O] {
	return func(a *Agent[D, O]) {
		a.modelSettings = settings
	}
}

// RunOption configures a single agent run.
type RunOption func(r *runConfig)

type runConfig struct {
	history       []*schema.Message
	usageLimits   *UsageLimits
	modelSettings map[string]any
	metadata      map[string]any
}

// UsageLimits defines limits for a single agent run.
type UsageLimits struct {
	RequestLimit       int // Max number of model calls (0 = unlimited)
	RequestTokensLimit int // Max prompt tokens (0 = unlimited)
	ResponseTokensLimit int // Max completion tokens (0 = unlimited)
	TotalTokensLimit   int // Max total tokens (0 = unlimited)
}

// WithHistory passes message history for multi-turn conversations.
func WithHistory(history []*schema.Message) RunOption {
	return func(r *runConfig) {
		r.history = history
	}
}

// WithUsageLimits sets usage limits for this run.
func WithUsageLimits(limits UsageLimits) RunOption {
	return func(r *runConfig) {
		r.usageLimits = &limits
	}
}

// WithRunModelSettings overrides model settings for this run.
func WithRunModelSettings(settings map[string]any) RunOption {
	return func(r *runConfig) {
		r.modelSettings = settings
	}
}

// WithRunMetadata sets metadata for this run.
func WithRunMetadata(meta map[string]any) RunOption {
	return func(r *runConfig) {
		r.metadata = meta
	}
}
