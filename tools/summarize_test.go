package tools

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestSummarize(t *testing.T) {
	tests := []struct {
		name string
		args string
		want string
	}{
		// Shell tools show the full command line.
		{"bash", `{"command": "git status"}`, "bash: git status"},
		{"sh", `{"command": "ls -la /tmp"}`, "sh: ls -la /tmp"},
		{"zsh", `{"command": "find . -name '*.go' | xargs wc -l"}`, "zsh: find . -name '*.go' | xargs wc -l"},
		{"bash", `{}`, "bash"},

		// Filesystem tools show the path (or directory).
		{"ls", `{"path": "/tmp"}`, "ls /tmp"},
		{"ls", `{}`, "ls ."},
		{"read_file", `{"path": "go.mod"}`, "read_file go.mod"},
		{"write_file", `{"path": "notes/todo.md", "content": "buy milk"}`, "write_file notes/todo.md"},
		{"edit_file", `{"path": "src/main.go", "mode": "string", "old": "a", "new": "b"}`, "edit_file src/main.go"},
		{"create_directory", `{"path": "a/b"}`, "create_directory a/b"},
		{"glob", `{"pattern": "**/*.go"}`, "glob **/*.go"},
		{"glob", `{"pattern": "*.txt", "path": "docs"}`, "glob *.txt in docs"},
		{"move_file", `{"source": "a.txt", "destination": "b.txt"}`, "move_file a.txt to b.txt"},

		// Web and user tools.
		{"http_fetch", `{"url": "https://example.com/"}`, "http_fetch https://example.com/"},
		{"http_fetch", `{"url": "https://example.com/", "method": "post"}`, "http_fetch POST https://example.com/"},
		{"user_question", `{"question": "What?", "responses": ["a"], "freetext": false}`, "user_question"},
		{"list_allowed_directories", `{}`, "list_allowed_directories"},

		// Unrecognized tools fall back to their string arguments.
		{"mystery_tool", `{"flag": "on", "count": 3}`, "mystery_tool flag=\"on\""},
		{"mystery_tool", `{}`, "mystery_tool"},
		{"mystery_tool", `not json at all`, "mystery_tool not json at all"},
	}
	for _, tt := range tests {
		t.Run(tt.name+"/"+tt.args, func(t *testing.T) {
			if got := Summarize(tt.name, json.RawMessage(tt.args)); got != tt.want {
				t.Errorf("Summarize(%q, %s) = %q, want %q", tt.name, tt.args, got, tt.want)
			}
		})
	}
}

func TestSummarizeTruncatesLongFallbacks(t *testing.T) {
	long := strings.Repeat("x", 200)
	got := Summarize("mystery_tool", json.RawMessage(`{"data": "`+long+`"}`))
	if !strings.HasSuffix(got, "...") {
		t.Errorf("long fallback should be truncated, got %d chars", len(got))
	}
	if len(got) > len("mystery_tool ")+80 {
		t.Errorf("fallback should be truncated to about 80 chars, got %d", len(got))
	}

	// The full command is never truncated: it is what the user must see.
	cmd := strings.Repeat("echo long; ", 20)
	got = Summarize("bash", json.RawMessage(`{"command": "`+cmd+`"}`))
	if got != "bash: "+cmd {
		t.Errorf("shell summary should carry the full command, got %d chars", len(got))
	}
}
