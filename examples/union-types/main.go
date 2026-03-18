// Package main demonstrates using OneOf2 union types for multi-variant output.
//
// The agent returns either a VulnReport (vulnerability found) or a SafeReport
// (code is safe). Match() provides compile-time exhaustive dispatch.
package main

import (
	"context"
	"fmt"
	"log"

	"github.com/cloudwego/eino/schema"

	"github.com/codycode/cody-core-go/agent"
	"github.com/codycode/cody-core-go/testutil"
)

// VulnReport is returned when a vulnerability is found.
type VulnReport struct {
	Type     string `json:"type" description:"Vulnerability type (e.g. SQLi, XSS)"`
	Severity string `json:"severity" description:"Severity level" enum:"low,medium,high,critical"`
	Details  string `json:"details" description:"Detailed description"`
}

// SafeReport is returned when no vulnerabilities are found.
type SafeReport struct {
	Summary string `json:"summary" description:"Summary of findings"`
}

func main() {
	ctx := context.Background()

	// Simulate model returning a VulnReport
	chatModel := testutil.NewTestModel(testutil.TestResponse{
		ToolCalls: []schema.ToolCall{{
			ID:   "call_1",
			Type: "function",
			Function: schema.FunctionCall{
				Name:      "final_result_VulnReport",
				Arguments: `{"type":"SQLi","severity":"high","details":"Unsanitized input in query builder"}`,
			},
		}},
	})

	// Create a union-output agent
	a := agent.NewOneOf2[agent.NoDeps, VulnReport, SafeReport](chatModel,
		agent.WithSystemPrompt[agent.NoDeps, agent.OneOf2[VulnReport, SafeReport]](
			"You are a security scanner. Analyze code and report findings.",
		),
	)

	result, err := a.Run(ctx, "Scan this code: db.Query(userInput)", agent.NoDeps{})
	if err != nil {
		log.Fatal(err)
	}

	// Exhaustive pattern matching — compiler ensures both cases are handled
	result.Output.Match(
		func(v VulnReport) {
			fmt.Printf("VULNERABILITY FOUND\n")
			fmt.Printf("  Type:     %s\n", v.Type)
			fmt.Printf("  Severity: %s\n", v.Severity)
			fmt.Printf("  Details:  %s\n", v.Details)
		},
		func(s SafeReport) {
			fmt.Printf("CODE SAFE: %s\n", s.Summary)
		},
	)
}
