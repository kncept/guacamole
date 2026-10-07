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
	"github.com/kncept/guacamole/permissions"
)

// maxReadBytes caps how much of a file read_file returns, so a huge file
// cannot flood the conversation.
const maxReadBytes = 64 * 1024

// FileSystem returns all the file tools: read_file, ls and write_file.
func FileSystem(checker AccessChecker) []ai.Tool {
	return []ai.Tool{
		ReadFile(checker),
		Ls(checker),
		WriteFile(checker),

		// TODO:
		// Glob // use https://github.com/bmatcuk/doublestar and include notes on the patterns from the docs
		// EditFile // regex/string style replace
		// CreateDirectory // including parent directories
		// MoveFile // has to check permissions _twice_
		// ListAllowedDirectories // list of directories that server is allowed to access without asking. Return Read/Write differences as well
	}
}

// ReadFile returns a tool that reads the contents of a file.
func ReadFile(checker AccessChecker) ai.Tool {
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
		Handler: func(ctx context.Context, args json.RawMessage) (string, error) {
			var a struct {
				Path string `json:"path"`
			}
			if err := json.Unmarshal(args, &a); err != nil {
				return "", err
			}
			if a.Path == "" {
				return "", errors.New("path is required")
			}
			if err := checkAccess(checker, "read_file", a.Path, permissions.AccessRead); err != nil {
				return "", err
			}
			return readFile(ctx, args)
		},
	}
}

// Ls returns a tool that lists a directory's entries with basic info:
// permissions, size in bytes, and name.
func Ls(checker AccessChecker) ai.Tool {
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
		Handler: func(ctx context.Context, args json.RawMessage) (string, error) {
			var a struct {
				Path string `json:"path"`
			}
			if err := json.Unmarshal(args, &a); err != nil {
				return "", err
			}
			if a.Path == "" {
				a.Path = "."
			}
			// make absolute
			absPath, err := filepath.Abs(a.Path)
			if err != nil {
				return "", err
			}
			a.Path = absPath

			if err := checkAccess(checker, "ls", a.Path, permissions.AccessRead); err != nil {
				return "", err
			}
			return ls(ctx, args)
		},
	}
}

// WriteFile returns a tool that writes content to a file, creating parent
// directories as needed.
func WriteFile(checker AccessChecker) ai.Tool {
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
			if err := checkAccess(checker, "write_file", a.Path, permissions.AccessWrite); err != nil {
				return "", err
			}
			return writeFile(ctx, args)
		},
	}
}

func checkAccess(checker AccessChecker, toolName, path string, access permissions.AccessKind) error {
	if checker == nil {
		return nil
	}
	allowed, err := checker.IsAllowedDirectory(toolName, path, access)
	if err != nil {
		return err
	}
	if !allowed {
		if access == permissions.AccessWrite {
			return fmt.Errorf("write access to %s denied", path)
		}
		return fmt.Errorf("read access to %s denied", path)
	}
	return nil
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
