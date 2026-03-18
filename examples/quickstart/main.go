// Package main demonstrates basic usage of cody-core-go.
//
// This example uses testutil.TestModel so it runs without any API keys.
// Replace TestModel with a real model (e.g. eino-ext/openai) for production use.
package main

import (
	"context"
	"fmt"
	"log"

	"github.com/cloudwego/eino/schema"

	"github.com/codycode/cody-core-go/agent"
	"github.com/codycode/cody-core-go/testutil"
)

// MovieReview is the structured output type.
type MovieReview struct {
	Title     string  `json:"title" description:"Movie title"`
	Rating    float64 `json:"rating" description:"Rating from 1 to 10"`
	Sentiment string  `json:"sentiment" description:"Overall sentiment" enum:"positive,negative,neutral"`
}

func main() {
	ctx := context.Background()

	// In production, replace TestModel with a real model:
	//
	//   chatModel, err := einoopenai.NewChatModel(ctx, &einoopenai.ChatModelConfig{
	//       Model:  "gpt-4o",
	//       APIKey: os.Getenv("OPENAI_API_KEY"),
	//   })
	//
	chatModel := testutil.NewTestModel(testutil.TestResponse{
		ToolCalls: []schema.ToolCall{{
			ID:   "call_1",
			Type: "function",
			Function: schema.FunctionCall{
				Name:      "final_result",
				Arguments: `{"title":"Interstellar","rating":9.2,"sentiment":"positive"}`,
			},
		}},
	})

	// Create a structured output agent
	a := agent.New[agent.NoDeps, MovieReview](chatModel,
		agent.WithSystemPrompt[agent.NoDeps, MovieReview](
			"You are a movie critic. Analyze the review and extract structured data.",
		),
		agent.WithModelSettings[agent.NoDeps, MovieReview](agent.ModelSettings{
			Temperature: agent.Ptr(float32(0.3)),
		}),
	)

	// Run the agent
	result, err := a.Run(ctx, "Just watched Interstellar. Nolan's storytelling is incredible!", agent.NoDeps{})
	if err != nil {
		log.Fatal(err)
	}

	// Access typed output
	fmt.Printf("Title:     %s\n", result.Output.Title)
	fmt.Printf("Rating:    %.1f\n", result.Output.Rating)
	fmt.Printf("Sentiment: %s\n", result.Output.Sentiment)
	fmt.Printf("Tokens:    %d\n", result.Usage.TotalTokens)
}
