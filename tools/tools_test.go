package tools

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kncept/guacamole/ai"
)

func run(t *testing.T, handler ai.ToolHandler, args string) (string, error) {
	t.Helper()
	return handler(context.Background(), json.RawMessage(args))
}

// path escapes a filesystem path for embedding in a JSON string.
func path(p string) string {
	return strings.ReplaceAll(p, `\`, `\\`)
}

func TestReadFile(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "hello.txt")
	if err := os.WriteFile(p, []byte("hello world"), 0o644); err != nil {
		t.Fatal(err)
	}

	out, err := run(t, ReadFile(nil).Handler, `{"path": "`+path(p)+`"}`)
	if err != nil {
		t.Fatalf("readFile: %v", err)
	}
	if out != "hello world" {
		t.Errorf("out = %q, want %q", out, "hello world")
	}
}

func TestReadFileMissing(t *testing.T) {
	if _, err := run(t, ReadFile(nil).Handler, `{"path": "`+path(filepath.Join(t.TempDir(), "nope.txt"))+`"}`); err == nil {
		t.Fatal("readFile: expected error for a missing file, got nil")
	}
	if _, err := run(t, ReadFile(nil).Handler, `{}`); err == nil {
		t.Fatal("readFile: expected error for empty path, got nil")
	}
}

func TestReadFileTruncates(t *testing.T) {
	p := filepath.Join(t.TempDir(), "big.txt")
	if err := os.WriteFile(p, make([]byte, maxReadBytes+10), 0o644); err != nil {
		t.Fatal(err)
	}

	out, err := run(t, ReadFile(nil).Handler, `{"path": "`+path(p)+`"}`)
	if err != nil {
		t.Fatalf("readFile: %v", err)
	}
	if !strings.Contains(out, "(truncated:") {
		t.Errorf("out should mention truncation, got %d bytes", len(out))
	}
}

func TestLs(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "afile.txt"), []byte("12345"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(dir, "adir"), 0o755); err != nil {
		t.Fatal(err)
	}

	out, err := run(t, Ls(nil).Handler, `{"path": "`+path(dir)+`"}`)
	if err != nil {
		t.Fatalf("ls: %v", err)
	}
	if !strings.Contains(out, "adir/") {
		t.Errorf("out should list adir/ as a directory:\n%s", out)
	}
	if !strings.Contains(out, "5") || !strings.Contains(out, "afile.txt") {
		t.Errorf("out should list afile.txt with its size:\n%s", out)
	}
}

func TestLsEmptyAndMissing(t *testing.T) {
	out, err := run(t, Ls(nil).Handler, `{"path": "`+path(t.TempDir())+`"}`)
	if err != nil {
		t.Fatalf("ls: %v", err)
	}
	if out != "(empty directory)" {
		t.Errorf("out = %q, want %q", out, "(empty directory)")
	}

	if _, err := run(t, Ls(nil).Handler, `{"path": "`+path(filepath.Join(t.TempDir(), "nope"))+`"}`); err == nil {
		t.Fatal("ls: expected error for a missing directory, got nil")
	}
}

func TestWriteFile(t *testing.T) {
	target := filepath.Join(t.TempDir(), "sub", "out.txt")

	out, err := run(t, WriteFile(nil).Handler, `{"path": "`+path(target)+`", "content": "written"}`)
	if err != nil {
		t.Fatalf("writeFile: %v", err)
	}
	if !strings.Contains(out, "wrote 7 bytes") {
		t.Errorf("out = %q, want a byte count", out)
	}

	data, err := os.ReadFile(target)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if string(data) != "written" {
		t.Errorf("file contents = %q, want %q", data, "written")
	}
}

func TestWriteFileEmptyPath(t *testing.T) {
	if _, err := run(t, WriteFile(nil).Handler, `{"content": "x"}`); err == nil {
		t.Fatal("writeFile: expected error for empty path, got nil")
	}
}
