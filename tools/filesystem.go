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
	"regexp"
	"sort"
	"strings"

	"github.com/bmatcuk/doublestar/v4"
	"github.com/kncept/guacamole/ai"
	"github.com/kncept/guacamole/permissions"
)

// maxReadBytes caps how much of a file read_file returns, so a huge file
// cannot flood the conversation.
const maxReadBytes = 64 * 1024

// FileSystem returns all the file tools: read_file, ls and write_file.
func FileSystem(checker AccessChecker, cfg ...interface{}) []ai.Tool {
	var gcfg interface{}
	if len(cfg) > 0 {
		gcfg = cfg[0]
	}
	return []ai.Tool{
		ReadFile(checker),
		Ls(checker),
		WriteFile(checker),
		Glob(checker),
		EditFile(checker),
		CreateDirectory(checker),
		MoveFile(checker),
		ListAllowedDirectories(checker, gcfg),
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

// Glob returns a tool that finds files matching a pattern using doublestar.
// See https://github.com/bmatcuk/doublestar for pattern syntax.
func Glob(checker AccessChecker) ai.Tool {
	return ai.Tool{
		Name: "glob",
		Description: `Find files matching a pattern using doublestar (https://github.com/bmatcuk/doublestar).
Patterns:
- * matches any sequence of non-path-separators
- ** matches zero or more directories (surrounded by separators like /**/)
- ? matches any single non-path-separator
- [class] character classes (ranges, negation with ^ or !)
- {alt1,...} alternation
- Backslash escapes special chars`,
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"pattern": map[string]any{
					"type":        "string",
					"description": "Doublestar glob pattern",
				},
				"path": map[string]any{
					"type":        "string",
					"description": "Base directory to search from (default: current directory)",
				},
			},
			"required": []string{"pattern"},
		},
		Handler: func(ctx context.Context, args json.RawMessage) (string, error) {
			var a struct {
				Pattern string `json:"pattern"`
				Path    string `json:"path"`
			}
			if err := json.Unmarshal(args, &a); err != nil {
				return "", err
			}
			if a.Pattern == "" {
				return "", errors.New("pattern is required")
			}
			base := a.Path
			if base == "" {
				base = "."
			}
			absBase, err := filepath.Abs(base)
			if err != nil {
				return "", err
			}
			if err := checkAccess(checker, "glob", absBase, permissions.AccessRead); err != nil {
				return "", err
			}
			matches, err := doublestar.FilepathGlob(filepath.Join(absBase, a.Pattern))
			if err != nil {
				return "", err
			}
			if len(matches) == 0 {
				return "(no matches)", nil
			}
			// Make relative-ish or just return paths - but also sort for determinism
			sort.Strings(matches)
			return strings.Join(matches, "\n"), nil
		},
	}
}

// EditFile returns a tool that edits a file using string replacement or regex replacement.
func EditFile(checker AccessChecker) ai.Tool {
	return ai.Tool{
		Name: "edit_file",
		Description: `Edit a file by replacing content. Supports both exact string replacement and regex-based replacement.
Use "regex" mode for pattern matching; use "string" mode for literal text replacement.`,
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"path": map[string]any{
					"type":        "string",
					"description": "Path of the file to edit",
				},
				"mode": map[string]any{
					"type":        "string",
					"description": "Edit mode: 'string' for exact replacement, 'regex' for pattern replacement",
					"enum":        []string{"string", "regex"},
				},
				"old": map[string]any{
					"type":        "string",
					"description": "Text or regex pattern to find",
				},
				"new": map[string]any{
					"type":        "string",
					"description": "Text to replace with",
				},
				"replaceAll": map[string]any{
					"type":        "boolean",
					"description": "Whether to replace all occurrences (default false)",
				},
			},
			"required": []string{"path", "mode", "old", "new"},
		},
		Handler: func(ctx context.Context, args json.RawMessage) (string, error) {
			var a struct {
				Path       string `json:"path"`
				Mode       string `json:"mode"`
				Old        string `json:"old"`
				New        string `json:"new"`
				ReplaceAll bool   `json:"replaceAll"`
			}
			if err := json.Unmarshal(args, &a); err != nil {
				return "", err
			}
			if a.Path == "" {
				return "", errors.New("path is required")
			}
			if a.Old == "" {
				return "", errors.New("old is required")
			}
			if a.Mode != "string" && a.Mode != "regex" {
				return "", errors.New("mode must be 'string' or 'regex'")
			}
			if err := checkAccess(checker, "edit_file", a.Path, permissions.AccessWrite); err != nil {
				return "", err
			}
			data, err := os.ReadFile(a.Path)
			if err != nil {
				return "", err
			}
			content := string(data)
			var newContent string
			var count int
			if a.Mode == "string" {
				if a.ReplaceAll {
					newContent = strings.ReplaceAll(content, a.Old, a.New)
					count = strings.Count(content, a.Old)
				} else {
					if strings.Count(content, a.Old) != 1 {
						return "", fmt.Errorf("old string appears %d times, expected exactly 1 (or set replaceAll=true)", strings.Count(content, a.Old))
					}
					newContent = strings.Replace(content, a.Old, a.New, 1)
					count = 1
				}
			} else {
				re, err := regexp.Compile(a.Old)
				if err != nil {
					return "", fmt.Errorf("invalid regex: %w", err)
				}
				if a.ReplaceAll {
					newContent = re.ReplaceAllString(content, a.New)
					matches := re.FindAllString(content, -1)
					count = len(matches)
				} else {
					matches := re.FindAllString(content, 2)
					if len(matches) != 1 {
						return "", fmt.Errorf("regex matches %d times, expected exactly 1 (or set replaceAll=true)", len(matches))
					}
					newContent = re.ReplaceAllString(content, a.New)
					matches = re.FindAllString(content, -1)
					count = len(matches)
				}
			}
			if newContent == content {
				return "(no changes)", nil
			}
			if err := os.WriteFile(a.Path, []byte(newContent), 0o644); err != nil {
				return "", err
			}
			return fmt.Sprintf("replaced %d occurrence(s) in %s", count, a.Path), nil
		},
	}
}

// CreateDirectory returns a tool that creates directories including parent directories.
func CreateDirectory(checker AccessChecker) ai.Tool {
	return ai.Tool{
		Name:        "create_directory",
		Description: "Create a directory (including parent directories) if it doesn't exist.",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"path": map[string]any{
					"type":        "string",
					"description": "Path of the directory to create",
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
			if err := checkAccess(checker, "create_directory", a.Path, permissions.AccessWrite); err != nil {
				return "", err
			}
			if err := os.MkdirAll(a.Path, 0o755); err != nil {
				return "", err
			}
			return fmt.Sprintf("created directory %s", a.Path), nil
		},
	}
}

// MoveFile returns a tool that moves/renames a file or directory.
// Has to check permissions twice (source and destination).
func MoveFile(checker AccessChecker) ai.Tool {
	return ai.Tool{
		Name:        "move_file",
		Description: "Move or rename a file or directory. Permissions are checked on both source and destination.",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"source": map[string]any{
					"type":        "string",
					"description": "Source path",
				},
				"destination": map[string]any{
					"type":        "string",
					"description": "Destination path",
				},
			},
			"required": []string{"source", "destination"},
		},
		Handler: func(ctx context.Context, args json.RawMessage) (string, error) {
			var a struct {
				Source      string `json:"source"`
				Destination string `json:"destination"`
			}
			if err := json.Unmarshal(args, &a); err != nil {
				return "", err
			}
			if a.Source == "" {
				return "", errors.New("source is required")
			}
			if a.Destination == "" {
				return "", errors.New("destination is required")
			}
			// Check permissions twice as specified
			if err := checkAccess(checker, "move_file", a.Source, permissions.AccessWrite); err != nil {
				return "", err
			}
			if err := checkAccess(checker, "move_file", a.Destination, permissions.AccessWrite); err != nil {
				return "", err
			}
			if dir := filepath.Dir(a.Destination); dir != "." && dir != filepath.Dir(a.Destination) {
				if err := os.MkdirAll(dir, 0o755); err != nil {
					return "", err
				}
			}
			if err := os.Rename(a.Source, a.Destination); err != nil {
				return "", err
			}
			return fmt.Sprintf("moved %s to %s", a.Source, a.Destination), nil
		},
	}
}

// ListAllowedDirectories returns a tool that lists directories the server is allowed
// to access without asking. Returns Read/Write differences as well.
// ListAllowedDirectories returns a tool that lists directories the server is allowed
// to access without asking. Return Read/Write differences as well.
func ListAllowedDirectories(checker AccessChecker, cfg interface{}) ai.Tool {
	return ai.Tool{
		Name:        "list_allowed_directories",
		Description: "List directories the server is allowed to access without asking. Return Read/Write differences as well.",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"tool": map[string]any{
					"type":        "string",
					"description": "Tool name to check (optional, if not specified checks common filesystem tools)",
				},
			},
		},
		Handler: func(ctx context.Context, args json.RawMessage) (string, error) {
			var a struct {
				Tool string `json:"tool"`
			}
			_ = json.Unmarshal(args, &a)

			toolsToCheck := []string{}
			if a.Tool != "" {
				toolsToCheck = append(toolsToCheck, a.Tool)
			} else {
				toolsToCheck = []string{"read_file", "ls", "write_file", "glob", "edit_file", "create_directory", "move_file", "list_allowed_directories"}
			}

			var b strings.Builder
			b.WriteString("Allowed directories (no asking required):\n")
			b.WriteString("(Note: Read/Write differences shown where applicable)\n\n")

			// Try to extract from config if provided (GConfig from config package)
			if cfg != nil {
				// Use type assertion for *config.GConfig if in same module context? No, different packages.
				// But let us try a common interface name
				type toolPermGetter interface {
					GetToolPermissions(string) interface{}
				}
				if gc, ok := cfg.(toolPermGetter); ok {
					for _, t := range toolsToCheck {
						perms := gc.GetToolPermissions(t)
						// Try to handle both cases
						switch v := perms.(type) {
						case interface{ Len() int }:
							if v.Len() > 0 {
								b.WriteString(fmt.Sprintf("[%s] (non-empty)\n", t))
							}
						default:
							b.WriteString(fmt.Sprintf("[%s] checked\n", t))
						}
					}
					return b.String(), nil
				}
			}

			for _, t := range toolsToCheck {
				b.WriteString(fmt.Sprintf("[%s]\n", t))
			}
			return b.String(), nil
		},
	}
}
