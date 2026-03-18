// Package main demonstrates multi-agent collaboration using the Agent-as-Tool pattern.
//
// An orchestrator agent delegates tasks to two specialist agents (researcher and writer)
// by calling them as tools. Each agent has its own system prompt and capabilities.
// This example uses testutil models so it runs without any API keys.
package main

import (
	"context"
	"fmt"
	"log"
	"strings"

	"github.com/cloudwego/eino/schema"

	"github.com/CodyCodeAgent/cody-core-go/agent"
	"github.com/CodyCodeAgent/cody-core-go/testutil"
)

// ResearchArgs are passed when the orchestrator calls the research tool.
type ResearchArgs struct {
	Topic string `json:"topic" description:"The topic to research"`
}

// WriteArgs are passed when the orchestrator calls the write tool.
type WriteArgs struct {
	Topic    string `json:"topic" description:"The topic to write about"`
	Research string `json:"research" description:"Research findings to base the article on"`
}

func main() {
	ctx := context.Background()

	// --- Specialist Agent 1: Researcher ---
	researchModel := testutil.NewFunctionModel(
		func(msgs []*schema.Message, _ []*schema.ToolInfo) (*schema.Message, error) {
			// Extract topic from user message
			for _, m := range msgs {
				if m.Role == schema.User && strings.Contains(m.Content, "Go") {
					return &schema.Message{
						Role: schema.Assistant,
						Content: "Research findings on Go concurrency:\n" +
							"1. Goroutines are lightweight threads managed by the Go runtime\n" +
							"2. Channels provide type-safe communication between goroutines\n" +
							"3. The select statement enables multiplexing on channel operations\n" +
							"4. sync.WaitGroup coordinates completion of multiple goroutines",
					}, nil
				}
			}
			return &schema.Message{Role: schema.Assistant, Content: "No relevant findings."}, nil
		},
	)

	researchAgent := agent.New[agent.NoDeps, string](researchModel,
		agent.WithSystemPrompt[agent.NoDeps, string](
			"You are a technical researcher. Provide detailed findings on the given topic.",
		),
	)

	// --- Specialist Agent 2: Writer ---
	writeModel := testutil.NewFunctionModel(
		func(msgs []*schema.Message, _ []*schema.ToolInfo) (*schema.Message, error) {
			for _, m := range msgs {
				if m.Role == schema.User && strings.Contains(m.Content, "Research findings") {
					return &schema.Message{
						Role: schema.Assistant,
						Content: "# Go Concurrency: A Practical Guide\n\n" +
							"Go's concurrency model is built on two pillars: goroutines and channels. " +
							"Goroutines are lightweight threads that cost only a few KB of stack space, " +
							"making it practical to run thousands concurrently. Channels connect these " +
							"goroutines with type-safe communication, preventing the data races common " +
							"in shared-memory concurrency. The select statement and sync.WaitGroup round " +
							"out the toolkit, enabling elegant concurrent designs.",
					}, nil
				}
			}
			return &schema.Message{Role: schema.Assistant, Content: "Unable to write without research."}, nil
		},
	)

	writeAgent := agent.New[agent.NoDeps, string](writeModel,
		agent.WithSystemPrompt[agent.NoDeps, string](
			"You are a technical writer. Write clear, concise articles based on the provided research.",
		),
	)

	// --- Orchestrator Agent ---
	// Simulates: call research tool → get findings → call write tool with findings → return article
	orchestratorCallIdx := 0
	orchestratorModel := testutil.NewFunctionModel(
		func(msgs []*schema.Message, _ []*schema.ToolInfo) (*schema.Message, error) {
			orchestratorCallIdx++
			switch orchestratorCallIdx {
			case 1:
				// Step 1: Call the research tool
				return &schema.Message{
					Role: schema.Assistant,
					ToolCalls: []schema.ToolCall{{
						ID: "call_1", Type: "function",
						Function: schema.FunctionCall{
							Name:      "research",
							Arguments: `{"topic":"Go concurrency patterns"}`,
						},
					}},
				}, nil
			case 2:
				// Step 2: Got research results, now call the write tool
				// Extract research from tool result message
				var research string
				for _, m := range msgs {
					if m.Role == schema.Tool && m.ToolCallID == "call_1" {
						research = m.Content
					}
				}
				args := fmt.Sprintf(`{"topic":"Go concurrency","research":%q}`, research)
				return &schema.Message{
					Role: schema.Assistant,
					ToolCalls: []schema.ToolCall{{
						ID: "call_2", Type: "function",
						Function: schema.FunctionCall{
							Name:      "write",
							Arguments: args,
						},
					}},
				}, nil
			default:
				// Step 3: Got article, return the final text
				var article string
				for _, m := range msgs {
					if m.Role == schema.Tool && m.ToolCallID == "call_2" {
						article = m.Content
					}
				}
				return &schema.Message{
					Role:    schema.Assistant,
					Content: article,
				}, nil
			}
		},
	)

	orchestrator := agent.New[agent.NoDeps, string](orchestratorModel,
		agent.WithSystemPrompt[agent.NoDeps, string](
			"You are a project coordinator. Use the research tool to gather information, "+
				"then use the write tool to produce a polished article.",
		),
		// Register specialist agents as tools
		agent.WithToolFunc[agent.NoDeps, string, ResearchArgs](
			"research", "Research a topic in depth",
			func(rc *agent.RunContext[agent.NoDeps], args ResearchArgs) (string, error) {
				result, err := researchAgent.Run(rc.Ctx, args.Topic, agent.NoDeps{})
				if err != nil {
					return "", fmt.Errorf("research agent failed: %w", err)
				}
				return result.Output, nil
			},
		),
		agent.WithToolFunc[agent.NoDeps, string, WriteArgs](
			"write", "Write an article based on research",
			func(rc *agent.RunContext[agent.NoDeps], args WriteArgs) (string, error) {
				prompt := fmt.Sprintf("Write about %s.\n\n%s", args.Topic, args.Research)
				result, err := writeAgent.Run(rc.Ctx, prompt, agent.NoDeps{})
				if err != nil {
					return "", fmt.Errorf("write agent failed: %w", err)
				}
				return result.Output, nil
			},
		),
	)

	// Run the orchestrator
	result, err := orchestrator.Run(ctx, "Write an article about Go concurrency patterns", agent.NoDeps{})
	if err != nil {
		log.Fatal(err)
	}

	fmt.Println("=== Multi-Agent Result ===")
	fmt.Println(result.Output)
	fmt.Printf("\nOrchestrator made %d model calls\n", result.Usage.Requests)
}
