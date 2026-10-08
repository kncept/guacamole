package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kncept/guacamole/ai"
	"github.com/kncept/guacamole/config"
	"github.com/kncept/guacamole/permissions"
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

func TestGlob(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "hello.txt"), []byte("hi"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(dir, "sub"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "sub", "world.txt"), []byte("hi"), 0o644); err != nil {
		t.Fatal(err)
	}

	out, err := run(t, Glob(nil).Handler, `{"pattern": "*.txt", "path": "`+path(dir)+`"}`)
	if err != nil {
		t.Fatalf("glob: %v", err)
	}
	if !strings.Contains(out, "hello.txt") {
		t.Errorf("out should contain hello.txt:\n%s", out)
	}

	out, err = run(t, Glob(nil).Handler, `{"pattern": "**/*.txt", "path": "`+path(dir)+`"}`)
	if err != nil {
		t.Fatalf("glob: %v", err)
	}
	if !strings.Contains(out, "hello.txt") || !strings.Contains(out, "world.txt") {
		t.Errorf("out should contain both txt files:\n%s", out)
	}
}

func TestGlobNoMatches(t *testing.T) {
	out, err := run(t, Glob(nil).Handler, `{"pattern": "*.nonexist", "path": "`+path(t.TempDir())+`"}`)
	if err != nil {
		t.Fatalf("glob: %v", err)
	}
	if out != "(no matches)" {
		t.Errorf("out = %q, want %q", out, "(no matches)")
	}
}

func TestEditFileString(t *testing.T) {
	p := filepath.Join(t.TempDir(), "file.txt")
	if err := os.WriteFile(p, []byte("hello world hello"), 0o644); err != nil {
		t.Fatal(err)
	}

	_, err := run(t, EditFile(nil).Handler, `{"path": "`+path(p)+`", "mode": "string", "old": "hello", "new": "hi"}`)
	if err == nil {
		t.Fatal("expected error when old appears multiple times without replaceAll")
	}

	out, err := run(t, EditFile(nil).Handler, `{"path": "`+path(p)+`", "mode": "string", "old": "world", "new": "universe"}`)
	if err != nil {
		t.Fatalf("edit: %v", err)
	}
	if !strings.Contains(out, "replaced") {
		t.Errorf("out = %q", out)
	}

	data, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "hello universe hello" {
		t.Errorf("got %q", data)
	}
}

func TestEditFileRegex(t *testing.T) {
	p := filepath.Join(t.TempDir(), "file.txt")
	if err := os.WriteFile(p, []byte("foo123 bar456"), 0o644); err != nil {
		t.Fatal(err)
	}

	out, err := run(t, EditFile(nil).Handler, `{"path": "`+path(p)+`", "mode": "regex", "old": "[0-9]+", "new": "NUM", "replaceAll": true}`)
	if err != nil {
		t.Fatalf("edit: %v", err)
	}
	if !strings.Contains(out, "replaced") {
		t.Errorf("out = %q", out)
	}

	data, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "fooNUM barNUM" {
		t.Errorf("got %q", data)
	}
}

func TestCreateDirectory(t *testing.T) {
	p := filepath.Join(t.TempDir(), "a", "b", "c")
	out, err := run(t, CreateDirectory(nil).Handler, `{"path": "`+path(p)+`"}`)
	if err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if !strings.Contains(out, "created") {
		t.Errorf("out = %q", out)
	}
	if fi, err := os.Stat(p); err != nil || !fi.IsDir() {
		t.Fatalf("dir not created: %v", err)
	}
}

func TestMoveFile(t *testing.T) {
	src := filepath.Join(t.TempDir(), "src.txt")
	dst := filepath.Join(t.TempDir(), "dst.txt")
	if err := os.WriteFile(src, []byte("data"), 0o644); err != nil {
		t.Fatal(err)
	}

	out, err := run(t, MoveFile(nil).Handler, `{"source": "`+path(src)+`", "destination": "`+path(dst)+`"}`)
	if err != nil {
		t.Fatalf("move: %v", err)
	}
	if !strings.Contains(out, "moved") {
		t.Errorf("out = %q", out)
	}
	if _, err := os.Stat(src); !os.IsNotExist(err) {
		t.Errorf("src should be gone")
	}
	if data, err := os.ReadFile(dst); err != nil || string(data) != "data" {
		t.Errorf("dst wrong: %v %s", err, data)
	}
}

func TestListAllowedDirectories(t *testing.T) {
	out, err := run(t, ListAllowedDirectories(nil).Handler, `{}`)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if !strings.Contains(out, "allowed") {
		t.Errorf("expected an explanation for the nil checker, got:\n%s", out)
	}
}

// fakeChecker is an AccessChecker with a fixed answer per category.
type fakeChecker struct {
	pathAllowed   bool
	shellAllowed  bool
	domainAllowed bool
	fs            config.FilesystemPermissions
}

func (f *fakeChecker) IsAllowedPath(path string, access permissions.AccessKind) (bool, error) {
	return f.pathAllowed, nil
}

func (f *fakeChecker) IsAllowedShellCommand(command string) (bool, error) {
	return f.shellAllowed, nil
}

func (f *fakeChecker) IsAllowedDomain(domain string) (bool, error) {
	return f.domainAllowed, nil
}

func (f *fakeChecker) FilesystemPermissions() config.FilesystemPermissions {
	return f.fs
}

func TestListAllowedDirectoriesFromFilesystemPermissions(t *testing.T) {
	checker := &fakeChecker{fs: config.FilesystemPermissions{
		Directories: []config.DirectoryPermission{
			{Directory: "/tmp", Read: config.PolicyAllow, Write: config.PolicyAllow},
			{Directory: "/etc", Read: config.PolicyDeny, Write: config.PolicyDeny},
		},
	}}

	out, err := run(t, ListAllowedDirectories(checker).Handler, `{}`)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if !strings.Contains(out, "/tmp (read: allow, write: allow)") {
		t.Errorf("output should carry /tmp's grants, got:\n%s", out)
	}
	if !strings.Contains(out, "/etc (read: deny, write: deny)") {
		t.Errorf("output should carry /etc's grants, got:\n%s", out)
	}
}

func TestListAllowedDirectoriesAllowAll(t *testing.T) {
	checker := &fakeChecker{fs: config.FilesystemPermissions{AllowAll: true}}

	out, err := run(t, ListAllowedDirectories(checker).Handler, `{}`)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if !strings.Contains(out, "allow-all") {
		t.Errorf("output should mention the allow-all override, got:\n%s", out)
	}
}

func TestReadFileDenied(t *testing.T) {
	p := filepath.Join(t.TempDir(), "secret.txt")
	if err := os.WriteFile(p, []byte("secret"), 0o644); err != nil {
		t.Fatal(err)
	}

	_, err := run(t, ReadFile(&fakeChecker{pathAllowed: false}).Handler, `{"path": "`+path(p)+`"}`)
	if err == nil || !strings.Contains(err.Error(), "read access") {
		t.Fatalf("expected a read denial, got %v", err)
	}
}

func TestWriteFileDenied(t *testing.T) {
	target := filepath.Join(t.TempDir(), "out.txt")

	_, err := run(t, WriteFile(&fakeChecker{pathAllowed: false}).Handler, `{"path": "`+path(target)+`", "content": "x"}`)
	if err == nil || !strings.Contains(err.Error(), "write access") {
		t.Fatalf("expected a write denial, got %v", err)
	}
	if _, statErr := os.Stat(target); !os.IsNotExist(statErr) {
		t.Errorf("denied write must not create the file")
	}
}

func TestShellDenied(t *testing.T) {
	_, err := run(t, Shell(&fakeChecker{shellAllowed: false}, "sh").Handler, `{"command": "echo hi"}`)
	if err == nil || !strings.Contains(err.Error(), "shell access denied") {
		t.Fatalf("expected a shell denial, got %v", err)
	}
}

func TestShellAllowed(t *testing.T) {
	if _, err := exec.LookPath("sh"); err != nil {
		t.Skip("sh not available")
	}
	out, err := run(t, Shell(&fakeChecker{shellAllowed: true}, "sh").Handler, `{"command": "echo shell ok"}`)
	if err != nil {
		t.Fatalf("shell: %v", err)
	}
	if !strings.Contains(out, "shell ok") {
		t.Errorf("out = %q, want the command output", out)
	}
}

func TestHTTPFetchDenied(t *testing.T) {
	_, err := run(t, HTTPFetch(&fakeChecker{domainAllowed: false}).Handler, `{"url": "http://example.com/"}`)
	if err == nil || !strings.Contains(err.Error(), "web access to example.com denied") {
		t.Fatalf("expected a web denial, got %v", err)
	}
}

func TestHTTPFetchAllowed(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, "hello from the web")
	}))
	defer srv.Close()

	out, err := run(t, HTTPFetch(&fakeChecker{domainAllowed: true}).Handler, `{"url": "`+srv.URL+`"}`)
	if err != nil {
		t.Fatalf("http_fetch: %v", err)
	}
	if !strings.Contains(out, "HTTP 200 OK") {
		t.Errorf("out should carry the status line, got:\n%s", out)
	}
	if !strings.Contains(out, "hello from the web") {
		t.Errorf("out should carry the body, got:\n%s", out)
	}
}

func TestHTTPFetchRejectsNonHTTPSchemes(t *testing.T) {
	if _, err := run(t, HTTPFetch(nil).Handler, `{"url": "ftp://example.com/"}`); err == nil {
		t.Fatal("expected an error for an unsupported scheme")
	}
	if _, err := run(t, HTTPFetch(nil).Handler, `{}`); err == nil {
		t.Fatal("expected an error for a missing url")
	}
}
