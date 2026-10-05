package tools

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"

	"github.com/kncept/guacamole/ai"
)

// shells are the consoles Console may expose.
var shells = []string{"bash", "sh", "zsh"}

// Console returns one tool per shell available on the system (found via
// the PATH). Each tool is named after its shell and runs commands through
// it.
func Console() []ai.Tool {
	var out []ai.Tool
	for _, shell := range shells {
		if _, err := exec.LookPath(shell); err == nil {
			out = append(out, Shell(shell))
		}
	}
	return out
}

// Shell returns a tool that runs a command through the named console
// shell (e.g. "bash" invokes `bash -c <command>`).
func Shell(name string) ai.Tool {
	return ai.Tool{
		Name:        name,
		Description: fmt.Sprintf("Run a command using %s.", name),
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"command": map[string]any{
					"type":        "string",
					"description": "The command line to execute",
				},
			},
			"required": []string{"command"},
		},
		Handler: func(ctx context.Context, args json.RawMessage) (string, error) {
			var a struct {
				Command string `json:"command"`
			}
			if err := json.Unmarshal(args, &a); err != nil {
				return "", err
			}
			if a.Command == "" {
				return "", errors.New("command is required")
			}
			cmd := exec.CommandContext(ctx, name, "-c", a.Command)
			var buf bytes.Buffer
			cmd.Stdout = &buf
			cmd.Stderr = &buf
			err := cmd.Run()
			out := buf.String()
			if err != nil {
				return out, fmt.Errorf("%s: %w", name, err)
			}
			return out, nil
		},
	}
}
