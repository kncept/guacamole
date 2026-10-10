package tools

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
)

// Summarize renders one tool call as a short human-readable line for display
// next to the user: the tool's name plus its meaningful arguments. Shell
// calls carry the full command line; file tools carry the path. Calls this
// function does not recognize fall back to a compact rendering of their
// string arguments, truncated.
func Summarize(name string, args json.RawMessage) string {
	var m map[string]any
	if err := json.Unmarshal(args, &m); err != nil || m == nil {
		return name + " " + truncateSummary(string(args))
	}

	str := func(key string) string {
		s, _ := m[key].(string)
		return s
	}

	// The shell tools (bash, sh, zsh, ...) carry the full command line in
	// "command"; it is shown in full, so the user sees exactly what runs.
	if cmd := str("command"); cmd != "" {
		return name + ": " + cmd
	}

	var line string
	switch name {
	case "ls":
		line = "ls " + orDefault(str("path"), ".")
	case "read_file", "write_file", "edit_file", "create_directory":
		line = name + " " + str("path")
	case "glob":
		line = name + " " + str("pattern")
		if p := str("path"); p != "" {
			line += " in " + p
		}
	case "move_file":
		line = name + " " + str("source") + " to " + str("destination")
	case "http_fetch":
		line = name + " " + str("url")
		if mth := strings.ToUpper(str("method")); mth != "" && mth != "GET" {
			line = name + " " + mth + " " + str("url")
		}
	case "user_question", "list_allowed_directories":
		line = name
	default:
		line = name + " " + compactArgs(m)
	}
	return strings.TrimSpace(line)
}

// orDefault returns v, or fallback when v is empty.
func orDefault(v, fallback string) string {
	if v == "" {
		return fallback
	}
	return v
}

// compactArgs renders the string arguments of an unrecognized tool as sorted
// key=value pairs, so the output is stable and only short values survive
// the truncation.
func compactArgs(m map[string]any) string {
	var parts []string
	for k, v := range m {
		if s, ok := v.(string); ok {
			parts = append(parts, fmt.Sprintf("%s=%q", k, s))
		}
	}
	sort.Strings(parts)
	return truncateSummary(strings.Join(parts, " "))
}

// truncateSummary shortens s to at most 80 characters, appending "..." if
// cut.
func truncateSummary(s string) string {
	if len(s) <= 80 {
		return s
	}
	return s[:77] + "..."
}
