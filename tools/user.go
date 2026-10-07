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
		Name:        "user_question",
		Description: "Ask the user a question and return their answer",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"question": map[string]any{
					"type":        "string",
					"description": "Question to ask the user",
				},
				"responses": map[string]any{
					"type":        "array",
					"items":       map[string]any{"type": "string"},
					"description": "Ordered list of suggested responses",
				},
				"freetext": map[string]any{
					"type":        "boolean",
					"description": "Whether the user may answer with a freetext response instead of a suggested one",
				},
			},
			"required": []string{"question", "responses", "freetext"},
		},
		Handler: func(ctx context.Context, args json.RawMessage) (string, error) {
			return "", nil
		},
	}
}
