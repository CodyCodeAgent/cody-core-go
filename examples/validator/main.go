// Package main demonstrates output validation with automatic retry.
//
// The agent validates that the model's output meets business rules. If validation
// fails, it returns feedback to the model and retries automatically.
package main

import (
	"context"
	"fmt"
	"log"

	"github.com/cloudwego/eino/schema"

	"github.com/CodyCodeAgent/cody-core-go/agent"
	"github.com/CodyCodeAgent/cody-core-go/testutil"
)

// WeatherReport is the structured output type with validation constraints.
type WeatherReport struct {
	City        string  `json:"city" description:"City name"`
	Temperature float64 `json:"temperature" description:"Temperature in Celsius"`
	Humidity    int     `json:"humidity" description:"Humidity percentage (0-100)"`
}

func main() {
	ctx := context.Background()

	// First response has invalid humidity (200%), second is corrected
	chatModel := testutil.NewTestModel(
		testutil.TestResponse{
			ToolCalls: []schema.ToolCall{{
				ID: "call_1", Type: "function",
				Function: schema.FunctionCall{
					Name:      "final_result",
					Arguments: `{"city":"Shanghai","temperature":28,"humidity":200}`,
				},
			}},
		},
		testutil.TestResponse{
			ToolCalls: []schema.ToolCall{{
				ID: "call_2", Type: "function",
				Function: schema.FunctionCall{
					Name:      "final_result",
					Arguments: `{"city":"Shanghai","temperature":28,"humidity":75}`,
				},
			}},
		},
	)

	a := agent.New[agent.NoDeps, WeatherReport](chatModel,
		agent.WithSystemPrompt[agent.NoDeps, WeatherReport](
			"You are a weather reporter.",
		),
		// Validator: humidity must be 0-100
		agent.WithOutputValidator[agent.NoDeps, WeatherReport](
			func(_ context.Context, w WeatherReport) (WeatherReport, error) {
				if w.Humidity < 0 || w.Humidity > 100 {
					return w, agent.NewModelRetry(
						fmt.Sprintf("humidity must be 0-100, got %d", w.Humidity),
					)
				}
				return w, nil
			},
		),
		agent.WithMaxResultRetries[agent.NoDeps, WeatherReport](3),
	)

	result, err := a.Run(ctx, "Weather in Shanghai?", agent.NoDeps{})
	if err != nil {
		log.Fatal(err)
	}

	fmt.Printf("City:        %s\n", result.Output.City)
	fmt.Printf("Temperature: %.0f°C\n", result.Output.Temperature)
	fmt.Printf("Humidity:    %d%%\n", result.Output.Humidity)
}
