// Package agent provides the core Agent[D, O] abstraction for building
// LLM-powered agents with structured output, dependency injection, and
// automatic validation retries. It builds on top of the Eino framework.
package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"

	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/schema"

	"github.com/codycode/cody-core-go/output"
)

const defaultMaxIterations = 20

// Agent is the core abstraction for building LLM-powered agents.
// D is the dependency type (passed to tools and system prompts via RunContext).
// O is the output type (automatically validated and deserialized).
type Agent[D any, O any] struct {
	chatModel        model.BaseChatModel
	systemPrompts    []SystemPromptFunc[D]
	staticPrompts    []string
	tools            []toolEntry[D]
	outputMode       output.Mode
	outputValidators []output.ValidatorFunc[O]
	maxToolRetries   int
	maxResultRetries int
	maxIterations    int
	modelSettings    map[string]any
	initErrors       []error

	// outputTools overrides the default single output tool (used for union types).
	outputTools []outputToolEntry
	// outputParser overrides the default ParseStructuredOutput[O] (used for union types).
	outputParser func(toolName string, argsJSON []byte) (O, error)
}

// outputToolEntry holds an output tool and its name for union type support.
type outputToolEntry struct {
	tool tool.InvokableTool
	name string
}

// New creates a new Agent with the given ChatModel and options.
func New[D any, O any](chatModel model.BaseChatModel, opts ...Option[D, O]) *Agent[D, O] {
	a := &Agent[D, O]{
		chatModel:        chatModel,
		maxToolRetries:   1,
		maxResultRetries: 1,
		maxIterations:    defaultMaxIterations,
	}
	for _, opt := range opts {
		opt(a)
	}
	return a
}

// WithModel returns a shallow copy of the agent with a different model.
// Useful for testing with TestModel.
func (a *Agent[D, O]) WithModel(m model.BaseChatModel) *Agent[D, O] {
	cp := *a
	cp.chatModel = m
	return &cp
}

// Run executes the agent synchronously and returns a typed result.
func (a *Agent[D, O]) Run(ctx context.Context, prompt string, deps D, opts ...RunOption) (*Result[O], error) {
	if len(a.initErrors) > 0 {
		return nil, fmt.Errorf("agent initialization errors: %v", a.initErrors)
	}

	cfg := &runConfig{}
	for _, o := range opts {
		o(cfg)
	}

	// Create RunContext
	tracker := &UsageTracker{}
	rc := &RunContext[D]{
		Ctx:      ctx,
		Deps:     deps,
		Usage:    tracker,
		Metadata: cfg.metadata,
	}

	// Inject RunContext into context
	ctx = withRunContext[D](ctx, rc)

	// Build system messages
	systemMsgs, err := a.buildSystemMessages(rc)
	if err != nil {
		return nil, fmt.Errorf("build system messages: %w", err)
	}

	// Build messages
	messages := make([]*schema.Message, 0, len(systemMsgs)+len(cfg.history)+1)
	messages = append(messages, systemMsgs...)
	if len(cfg.history) > 0 {
		messages = append(messages, cfg.history...)
	}
	messages = append(messages, &schema.Message{
		Role:    schema.User,
		Content: prompt,
	})

	// Build tool map (user tools only, used for execution)
	toolMap := make(map[string]tool.InvokableTool)
	for _, entry := range a.tools {
		info, infoErr := entry.tool.Info(ctx)
		if infoErr != nil {
			return nil, fmt.Errorf("get tool info: %w", infoErr)
		}
		toolMap[info.Name] = entry.tool
	}

	// Track retries
	toolRetries := make(map[string]int) // per-tool retry counts
	resultRetries := 0

	// Determine new messages start index
	newMessageStart := len(cfg.history)

	// Agent Loop
	for iteration := 0; iteration < a.maxIterations; iteration++ {
		// Check usage limits before each model call
		if err := checkUsageLimits(tracker, cfg.usageLimits); err != nil {
			return nil, err
		}

		// Prepare tools per-iteration (run prepare callbacks each time)
		allToolInfos, outputToolNames, err := a.buildToolInfos(ctx, rc)
		if err != nil {
			return nil, fmt.Errorf("build tool infos: %w", err)
		}

		// Prepare model options
		modelOpts := a.buildModelOptions(cfg, allToolInfos)

		// Call model
		resp, err := a.chatModel.Generate(ctx, messages, modelOpts...)
		if err != nil {
			return nil, fmt.Errorf("model generate: %w", err)
		}

		// Track usage
		if resp.ResponseMeta != nil && resp.ResponseMeta.Usage != nil {
			u := resp.ResponseMeta.Usage
			tracker.AddTokens(u.PromptTokens, u.CompletionTokens, u.TotalTokens)
		}

		// Analyze response
		hasToolCalls := len(resp.ToolCalls) > 0

		if !hasToolCalls {
			// Pure text response
			if output.IsString[O]() {
				// O = string, return text directly
				var result O
				if v, ok := any(&result).(*string); ok {
					*v = resp.Content
				}

				messages = append(messages, resp)
				nonSystemMsgs := extractNonSystemMessages(messages, len(systemMsgs))

				return &Result[O]{
					Output:          result,
					Usage:           usageFromTracker(tracker),
					allMessages:     nonSystemMsgs,
					newMessageStart: newMessageStart,
				}, nil
			}

			// O != string but got text — try JSON parse first (per pydantic-ai-behavior.md §3.5)
			if resp.Content != "" {
				trimmed := strings.TrimSpace(resp.Content)
				if trimmed != "" {
					parsedOutput, parseErr := output.ParseStructuredOutput[O]([]byte(trimmed))
					if parseErr == nil {
						// Successfully parsed text as JSON — validate
						validatedOutput, valErr := output.RunValidators(ctx, parsedOutput, a.outputValidators)
						if valErr == nil {
							messages = append(messages, resp)
							nonSystemMsgs := extractNonSystemMessages(messages, len(systemMsgs))
							return &Result[O]{
								Output:          validatedOutput,
								Usage:           usageFromTracker(tracker),
								allMessages:     nonSystemMsgs,
								newMessageStart: newMessageStart,
							}, nil
						}
						// Validation failed — fall through to retry
					}
					// JSON parse failed — fall through to retry
				}
			}

			// Text could not be parsed as JSON or was empty — trigger retry
			resultRetries++
			if resultRetries > a.maxResultRetries {
				return nil, &ResultRetriesExceededError{
					MaxRetries: a.maxResultRetries,
					LastError:  fmt.Errorf("model returned plain text but structured output expected"),
				}
			}

			messages = append(messages, resp)
			messages = append(messages, &schema.Message{
				Role:    schema.User,
				Content: "Please use the provided tool to return your response in the required structured format.",
			})
			continue
		}

		// Has tool calls — check for output tool first (early strategy)
		var outputToolCall *schema.ToolCall
		var regularToolCalls []schema.ToolCall
		for i := range resp.ToolCalls {
			tc := &resp.ToolCalls[i]
			if isOutputToolCall(tc.Function.Name, outputToolNames) {
				outputToolCall = tc
			} else {
				regularToolCalls = append(regularToolCalls, *tc)
			}
		}

		if outputToolCall != nil {
			// Output tool called — parse and validate
			messages = append(messages, resp)

			var parsedOutput O
			if a.outputParser != nil {
				parsedOutput, err = a.outputParser(outputToolCall.Function.Name, []byte(outputToolCall.Function.Arguments))
			} else {
				parsedOutput, err = output.ParseStructuredOutput[O]([]byte(outputToolCall.Function.Arguments))
			}
			if err != nil {
				// Parse error — retry
				resultRetries++
				if resultRetries > a.maxResultRetries {
					return nil, &ResultRetriesExceededError{
						MaxRetries: a.maxResultRetries,
						LastError:  err,
					}
				}
				messages = append(messages, &schema.Message{
					Role:       schema.Tool,
					Content:    fmt.Sprintf("Error parsing output: %s", err.Error()),
					ToolCallID: outputToolCall.ID,
				})
				continue
			}

			// Run validators
			validatedOutput, err := output.RunValidators(ctx, parsedOutput, a.outputValidators)
			if err != nil {
				if retryErr, ok := IsModelRetry(err); ok {
					resultRetries++
					if resultRetries > a.maxResultRetries {
						return nil, &ResultRetriesExceededError{
							MaxRetries: a.maxResultRetries,
							LastError:  err,
						}
					}
					messages = append(messages, &schema.Message{
						Role:       schema.Tool,
						Content:    retryErr.Message,
						ToolCallID: outputToolCall.ID,
					})
					continue
				}
				return nil, fmt.Errorf("output validation: %w", err)
			}

			// Add skipped tool responses for regular tools (early strategy)
			for _, tc := range regularToolCalls {
				messages = append(messages, &schema.Message{
					Role:       schema.Tool,
					Content:    "Tool not executed - a final result was already provided.",
					ToolCallID: tc.ID,
				})
			}

			nonSystemMsgs := extractNonSystemMessages(messages, len(systemMsgs))

			return &Result[O]{
				Output:          validatedOutput,
				Usage:           usageFromTracker(tracker),
				allMessages:     nonSystemMsgs,
				newMessageStart: newMessageStart,
			}, nil
		}

		// Only regular tool calls — execute them
		messages = append(messages, resp)
		toolResults, err := a.executeToolCalls(ctx, regularToolCalls, toolMap, toolRetries)
		if err != nil {
			return nil, err
		}
		messages = append(messages, toolResults...)
	}

	return nil, fmt.Errorf("agent loop exceeded max iterations (%d)", a.maxIterations)
}

// RunStream executes the agent with streaming output.
func (a *Agent[D, O]) RunStream(ctx context.Context, prompt string, deps D, opts ...RunOption) (*StreamResult[O], error) {
	if len(a.initErrors) > 0 {
		return nil, fmt.Errorf("agent initialization errors: %v", a.initErrors)
	}

	sr := &StreamResult[O]{
		done: make(chan struct{}),
	}

	sr.agentLoop = func() {
		result, err := a.Run(ctx, prompt, deps, opts...)
		sr.finalResult = result
		sr.finalErr = err
		if result != nil && output.IsString[O]() {
			if v, ok := any(&result.Output).(*string); ok && sr.textCh != nil {
				sr.textCh <- *v
			}
		}
	}

	return sr, nil
}

// RunWithHistory is a convenience method that runs with message history.
func (a *Agent[D, O]) RunWithHistory(
	ctx context.Context,
	prompt string,
	deps D,
	history []*schema.Message,
	opts ...RunOption,
) (*Result[O], error) {
	opts = append([]RunOption{WithHistory(history)}, opts...)
	return a.Run(ctx, prompt, deps, opts...)
}

// buildSystemMessages constructs all system messages from static and dynamic prompts.
func (a *Agent[D, O]) buildSystemMessages(rc *RunContext[D]) ([]*schema.Message, error) {
	var msgs []*schema.Message

	// Static prompts first
	for _, p := range a.staticPrompts {
		msgs = append(msgs, &schema.Message{
			Role:    schema.System,
			Content: p,
		})
	}

	// Dynamic prompts
	for _, fn := range a.systemPrompts {
		content, err := fn(rc)
		if err != nil {
			return nil, fmt.Errorf("dynamic system prompt: %w", err)
		}
		msgs = append(msgs, &schema.Message{
			Role:    schema.System,
			Content: content,
		})
	}

	return msgs, nil
}

// buildToolInfos builds tool info list per-iteration, running prepare callbacks each time.
// Returns the combined tool infos and a set of output tool names.
func (a *Agent[D, O]) buildToolInfos(ctx context.Context, rc *RunContext[D]) (
	[]*schema.ToolInfo, map[string]bool, error,
) {
	var allToolInfos []*schema.ToolInfo
	outputToolNames := make(map[string]bool)

	// User tools (with prepare if configured)
	for _, entry := range a.tools {
		info, err := entry.tool.Info(ctx)
		if err != nil {
			return nil, nil, fmt.Errorf("get tool info: %w", err)
		}
		// Apply prepare callback if present
		if entry.prepare != nil {
			modifiedInfo, prepErr := entry.prepare(rc, info)
			if prepErr != nil {
				return nil, nil, fmt.Errorf("prepare tool %q: %w", info.Name, prepErr)
			}
			info = modifiedInfo
		}
		allToolInfos = append(allToolInfos, info)
	}

	// Output tools (if O is not string)
	if !output.IsString[O]() {
		if len(a.outputTools) > 0 {
			// Union type — use pre-built output tools
			for _, entry := range a.outputTools {
				info, err := entry.tool.Info(ctx)
				if err != nil {
					return nil, nil, fmt.Errorf("get union output tool info: %w", err)
				}
				allToolInfos = append(allToolInfos, info)
				outputToolNames[entry.name] = true
			}
		} else {
			// Standard single output tool
			paramsOneOf, err := output.BuildParamsOneOf[O]()
			if err != nil {
				return nil, nil, fmt.Errorf("build output schema: %w", err)
			}
			outTool := output.GenerateOutputTool[O](paramsOneOf)
			outInfo, err := outTool.Info(ctx)
			if err != nil {
				return nil, nil, fmt.Errorf("get output tool info: %w", err)
			}
			allToolInfos = append(allToolInfos, outInfo)
			outputToolNames[outInfo.Name] = true
		}
	}

	return allToolInfos, outputToolNames, nil
}

// isOutputToolCall checks if a tool call name matches any output tool.
func isOutputToolCall(name string, outputToolNames map[string]bool) bool {
	return outputToolNames[name] || output.IsOutputToolName(name)
}

// executeToolCalls executes tool calls in parallel and returns tool result messages.
func (a *Agent[D, O]) executeToolCalls(
	ctx context.Context,
	toolCalls []schema.ToolCall,
	toolMap map[string]tool.InvokableTool,
	toolRetries map[string]int,
) ([]*schema.Message, error) {
	results := make([]*schema.Message, len(toolCalls))
	var errs []error
	var mu sync.Mutex
	var wg sync.WaitGroup

	for i, tc := range toolCalls {
		wg.Add(1)
		go func(idx int, tc schema.ToolCall) {
			defer wg.Done()

			t, ok := toolMap[tc.Function.Name]
			if !ok {
				mu.Lock()
				results[idx] = &schema.Message{
					Role:       schema.Tool,
					Content:    fmt.Sprintf("Error: unknown tool %q", tc.Function.Name),
					ToolCallID: tc.ID,
					ToolName:   tc.Function.Name,
				}
				mu.Unlock()
				return
			}

			result, err := a.executeSingleTool(ctx, t, tc, toolRetries, &mu)
			mu.Lock()
			if err != nil {
				errs = append(errs, err)
			}
			results[idx] = result
			mu.Unlock()
		}(i, tc)
	}

	wg.Wait()

	if len(errs) > 0 {
		return nil, errs[0]
	}

	return results, nil
}

// executeSingleTool executes a single tool call with retry handling.
// The mu mutex protects access to the shared toolRetries map.
func (a *Agent[D, O]) executeSingleTool(
	ctx context.Context,
	t tool.InvokableTool,
	tc schema.ToolCall,
	toolRetries map[string]int,
	mu *sync.Mutex,
) (*schema.Message, error) {
	result, err := func() (res string, resErr error) {
		defer func() {
			if r := recover(); r != nil {
				resErr = fmt.Errorf("tool %q panicked: %v", tc.Function.Name, r)
			}
		}()
		return t.InvokableRun(ctx, tc.Function.Arguments)
	}()

	if err != nil {
		if retryErr, ok := IsModelRetry(err); ok {
			// ModelRetry — consume per-tool retry count (under lock)
			mu.Lock()
			toolRetries[tc.Function.Name]++
			count := toolRetries[tc.Function.Name]
			mu.Unlock()

			if count > a.maxToolRetries {
				return nil, &ToolRetriesExceededError{
					ToolName:   tc.Function.Name,
					MaxRetries: a.maxToolRetries,
					LastError:  err,
				}
			}
			return &schema.Message{
				Role:       schema.Tool,
				Content:    retryErr.Message,
				ToolCallID: tc.ID,
				ToolName:   tc.Function.Name,
			}, nil
		}

		// Regular error — feedback to model, no retry count consumed
		return &schema.Message{
			Role:       schema.Tool,
			Content:    fmt.Sprintf("Error running tool: %s", err.Error()),
			ToolCallID: tc.ID,
			ToolName:   tc.Function.Name,
		}, nil
	}

	return &schema.Message{
		Role:       schema.Tool,
		Content:    result,
		ToolCallID: tc.ID,
		ToolName:   tc.Function.Name,
	}, nil
}

// buildModelOptions builds model.Option slice from agent and run settings.
func (a *Agent[D, O]) buildModelOptions(cfg *runConfig, toolInfos []*schema.ToolInfo) []model.Option {
	var opts []model.Option

	if len(toolInfos) > 0 {
		opts = append(opts, model.WithTools(toolInfos))
	}

	// Apply agent-level settings
	settings := a.modelSettings
	if cfg.modelSettings != nil {
		// Run-level settings override agent-level
		merged := make(map[string]any)
		for k, v := range a.modelSettings {
			merged[k] = v
		}
		for k, v := range cfg.modelSettings {
			merged[k] = v
		}
		settings = merged
	}

	if settings != nil {
		if v, ok := settings["temperature"]; ok {
			if f, ok := toFloat32(v); ok {
				opts = append(opts, model.WithTemperature(f))
			}
		}
		if v, ok := settings["max_tokens"]; ok {
			if n, ok := toInt(v); ok {
				opts = append(opts, model.WithMaxTokens(n))
			}
		}
		if v, ok := settings["top_p"]; ok {
			if f, ok := toFloat32(v); ok {
				opts = append(opts, model.WithTopP(f))
			}
		}
		if v, ok := settings["stop"]; ok {
			if s, ok := v.([]string); ok {
				opts = append(opts, model.WithStop(s))
			}
		}
	}

	return opts
}

// checkUsageLimits verifies that usage limits have not been exceeded.
func checkUsageLimits(tracker *UsageTracker, limits *UsageLimits) error {
	if limits == nil {
		return nil
	}
	if limits.RequestLimit > 0 && tracker.Requests >= limits.RequestLimit {
		return &UsageLimitExceededError{
			Message: fmt.Sprintf("request limit %d reached", limits.RequestLimit),
		}
	}
	if limits.RequestTokensLimit > 0 && tracker.RequestTokens >= limits.RequestTokensLimit {
		return &UsageLimitExceededError{
			Message: fmt.Sprintf("request tokens limit %d reached", limits.RequestTokensLimit),
		}
	}
	if limits.ResponseTokensLimit > 0 && tracker.ResponseTokens >= limits.ResponseTokensLimit {
		return &UsageLimitExceededError{
			Message: fmt.Sprintf("response tokens limit %d reached", limits.ResponseTokensLimit),
		}
	}
	if limits.TotalTokensLimit > 0 && tracker.TotalTokens >= limits.TotalTokensLimit {
		return &UsageLimitExceededError{
			Message: fmt.Sprintf("total tokens limit %d reached", limits.TotalTokensLimit),
		}
	}
	return nil
}

// extractNonSystemMessages filters out system messages from the message list.
func extractNonSystemMessages(messages []*schema.Message, systemCount int) []*schema.Message {
	if systemCount >= len(messages) {
		return nil
	}
	return messages[systemCount:]
}

// usageFromTracker converts a UsageTracker to a Usage summary.
func usageFromTracker(tracker *UsageTracker) Usage {
	return Usage{
		Requests:       tracker.Requests,
		RequestTokens:  tracker.RequestTokens,
		ResponseTokens: tracker.ResponseTokens,
		TotalTokens:    tracker.TotalTokens,
	}
}

// IsModelRetry checks if an error is (or wraps) an ErrModelRetry.
// Uses errors.As to support wrapped errors.
func IsModelRetry(err error) (*ErrModelRetry, bool) {
	var e *ErrModelRetry
	if errors.As(err, &e) {
		return e, true
	}
	return nil, false
}

// Helper type conversion functions.
func toFloat32(v any) (float32, bool) {
	switch val := v.(type) {
	case float32:
		return val, true
	case float64:
		return float32(val), true
	case int:
		return float32(val), true
	case json.Number:
		f, err := val.Float64()
		if err != nil {
			return 0, false
		}
		return float32(f), true
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
	case json.Number:
		n, err := val.Int64()
		if err != nil {
			return 0, false
		}
		return int(n), true
	}
	return 0, false
}
