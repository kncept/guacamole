// Package tools implements the ai.Tool functions the model can call:
// read_file, ls and write_file.
package tools

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/kncept/guacamole/ai"
)

// maxReadBytes caps how much of a file read_file returns, so a huge file
// cannot flood the conversation.
const maxReadBytes = 64 * 1024

// FileSystem returns all the file tools: read_file, ls and write_file.
// allowWrite is passed to WriteFile; nil allows all writes.
func FileSystem(allowWrite func(dir string) bool) []ai.Tool {
	return []ai.Tool{ReadFile(), Ls(), WriteFile(allowWrite)}
}

// ReadFile returns a tool that reads the contents of a file.
func ReadFile() ai.Tool {
	return ai.Tool{
		Name:        "read_file",
		Description: "Read the contents of a file.",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"path": map[string]any{
					"type":        "string",
					"description": "Path of the file to read",
				},
			},
			"required": []string{"path"},
		},
		Handler: readFile,
	}
}

// Ls returns a tool that lists a directory's entries with basic info:
// permissions, size in bytes, and name.
func Ls() ai.Tool {
	return ai.Tool{
		Name:        "ls",
		Description: "List a directory's entries with basic info: permissions, size in bytes, name.",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"path": map[string]any{
					"type":        "string",
					"description": `Path of the directory to list (default ".")`,
				},
			},
		},
		Handler: ls,
	}
}

// WriteFile returns a tool that writes content to a file, creating parent
// directories as needed.
//
// The allowWrite callback is consulted with the absolute target directory
// before every write; nil allows all writes.
func WriteFile(allowWrite func(dir string) bool) ai.Tool {
	return ai.Tool{
		Name:        "write_file",
		Description: "Write content to a file, creating parent directories as needed.",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"path": map[string]any{
					"type":        "string",
					"description": "Path of the file to write",
				},
				"content": map[string]any{
					"type":        "string",
					"description": "Content to write to the file",
				},
			},
			"required": []string{"path", "content"},
		},
		Handler: func(ctx context.Context, args json.RawMessage) (string, error) {
			if allowWrite != nil {
				var a struct {
					Path string `json:"path"`
				}
				if err := json.Unmarshal(args, &a); err != nil {
					return "", err
				}
				if a.Path == "" {
					return "", errors.New("path is required")
				}
				dir, err := filepath.Abs(filepath.Dir(a.Path))
				if err != nil {
					return "", err
				}
				if !allowWrite(dir) {
					return "", fmt.Errorf("write access to %s denied", dir)
				}
			}
			return writeFile(ctx, args)
		},
	}
}

func readFile(ctx context.Context, args json.RawMessage) (string, error) {
	var a struct {
		Path string `json:"path"`
	}
	if err := json.Unmarshal(args, &a); err != nil {
		return "", err
	}
	if a.Path == "" {
		return "", errors.New("path is required")
	}

	data, err := os.ReadFile(a.Path)
	if err != nil {
		return "", err
	}
	if len(data) == 0 {
		return "(empty file)", nil
	}
	if len(data) > maxReadBytes {
		return fmt.Sprintf("%s\n... (truncated: file is %d bytes, showing first %d)", data[:maxReadBytes], len(data), maxReadBytes), nil
	}
	return string(data), nil
}

func ls(ctx context.Context, args json.RawMessage) (string, error) {
	var a struct {
		Path string `json:"path"`
	}
	if err := json.Unmarshal(args, &a); err != nil {
		return "", err
	}
	if a.Path == "" {
		a.Path = "."
	}

	entries, err := os.ReadDir(a.Path)
	if err != nil {
		return "", err
	}
	if len(entries) == 0 {
		return "(empty directory)", nil
	}

	var b strings.Builder
	for _, e := range entries {
		info, err := e.Info()
		if err != nil {
			fmt.Fprintf(&b, "%s (error: %v)\n", e.Name(), err)
			continue
		}
		if e.IsDir() {
			fmt.Fprintf(&b, "%s %10s %s/\n", info.Mode(), "-", e.Name())
		} else {
			fmt.Fprintf(&b, "%s %10d %s\n", info.Mode(), info.Size(), e.Name())
		}
	}
	return b.String(), nil
}

func writeFile(ctx context.Context, args json.RawMessage) (string, error) {
	var a struct {
		Path    string `json:"path"`
		Content string `json:"content"`
	}
	if err := json.Unmarshal(args, &a); err != nil {
		return "", err
	}
	if a.Path == "" {
		return "", errors.New("path is required")
	}

	if dir := filepath.Dir(a.Path); dir != "." {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return "", err
		}
	}
	if err := os.WriteFile(a.Path, []byte(a.Content), 0o644); err != nil {
		return "", err
	}
	return fmt.Sprintf("wrote %d bytes to %s", len(a.Content), a.Path), nil
}
