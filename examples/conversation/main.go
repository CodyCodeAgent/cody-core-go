// Package main demonstrates multi-turn conversation with automatic history management.
//
// The Conversation type carries message history between turns automatically,
// so the model remembers previous exchanges.
package main

import (
	"context"
	"fmt"
	"log"

	"github.com/cloudwego/eino/schema"

	"github.com/CodyCodeAgent/cody-core-go/agent"
	"github.com/CodyCodeAgent/cody-core-go/testutil"
)

func main() {
	ctx := context.Background()

	callIdx := 0
	chatModel := testutil.NewFunctionModel(
		func(msgs []*schema.Message, _ []*schema.ToolInfo) (*schema.Message, error) {
			callIdx++
			switch callIdx {
			case 1:
				return &schema.Message{
					Role:    schema.Assistant,
					Content: "Nice to meet you, Alice! I'll remember your name.",
				}, nil
			case 2:
				// Check if history contains the name
				for _, m := range msgs {
					if m.Content == "My name is Alice." {
						return &schema.Message{
							Role:    schema.Assistant,
							Content: "Your name is Alice!",
						}, nil
					}
				}
				return &schema.Message{
					Role:    schema.Assistant,
					Content: "I'm sorry, I don't know your name.",
				}, nil
			default:
				return &schema.Message{
					Role:    schema.Assistant,
					Content: "Goodbye, Alice!",
				}, nil
			}
		},
	)

	a := agent.New[agent.NoDeps, string](chatModel,
		agent.WithSystemPrompt[agent.NoDeps, string]("You remember everything the user tells you."),
	)

	// Create a conversation — it tracks history automatically
	conv := agent.NewConversation(a)

	// Turn 1
	r1, err := conv.Send(ctx, "My name is Alice.", agent.NoDeps{})
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("Turn 1: %s\n", r1.Output)

	// Turn 2 — history is carried forward
	r2, err := conv.Send(ctx, "What's my name?", agent.NoDeps{})
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("Turn 2: %s\n", r2.Output)

	fmt.Printf("\nTotal messages in history: %d\n", len(conv.Messages()))
}
