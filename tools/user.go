package tools

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/kncept/guacamole/ai"
)

func User(liveCallback UserQuestionCallback) []ai.Tool {
	return []ai.Tool{
		UserQuestion(liveCallback),
	}
}

type UserQuestionCallback func(questions string, responses []string, allowFreetext bool) (string, error)

// needs a CALLBACK
func UserQuestion(liveCallback UserQuestionCallback) ai.Tool {
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
					"description": "Ordered list of suggested responses with a maximum of 8 options",
				},
				"freetext": map[string]any{
					"type":        "boolean",
					"description": "Whether the user may answer with a freetext response instead of a suggested one",
				},
			},
			"required": []string{"question", "responses", "freetext"},
		},
		Handler: func(ctx context.Context, args json.RawMessage) (string, error) {
			var a struct {
				Question  string   `json:"question"`
				Responses []string `json:"responses"`
				Freetext  bool     `json:"freetext"`
			}
			if err := json.Unmarshal(args, &a); err != nil {
				return "", err
			}
			if len(a.Responses) > 8 {
				return "", fmt.Errorf("A maxiumum of 8 options are allowed")
			}
			return liveCallback(a.Question, a.Responses, a.Freetext)
		},
	}
}
