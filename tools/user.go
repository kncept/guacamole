package tools

import (
	"context"
	"encoding/json"

	"github.com/kncept/guacamole/ai"
)

func User() []ai.Tool {
	return []ai.Tool{
		UserQuestion(),
	}
}

// needs a CALLBACK
func UserQuestion() ai.Tool {
	return ai.Tool{
		Name:        "User Question",
		Description: "Ask the user a question, and have them pick from a set of options",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"question": map[string]any{
					"type":        "string",
					"description": "Question to ask the user",
				},
				"options": map[string]any{
					"type":        "string",
					"description": "Ordered list of suggested responses",
				},
				"freetext": map[string]any{
					"type":        "boolean",
					"description": "Allow the user to enter a freetext response instead",
				},
			},
			"required": []string{"path"},
		},
		Handler: func(ctx context.Context, args json.RawMessage) (string, error) {
			return "", nil
		},
	}
}
