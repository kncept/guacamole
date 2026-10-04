package permissions

import (
	"os"
	"path/filepath"
	"testing"
)

func testPath(t *testing.T) string {
	t.Helper()
	return filepath.Join(t.TempDir(), "permissions.json")
}

func TestAllowWriteAsksAndPersists(t *testing.T) {
	path := testPath(t)
	m, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	dir := filepath.Join(t.TempDir(), "project")

	asked := 0
	m.Ask = func(got string) bool {
		asked++
		if got != dir {
			t.Errorf("Ask dir = %q, want %q", got, dir)
		}
		return true
	}

	if !m.AllowWrite(dir) {
		t.Fatal("AllowWrite = false after a grant, want true")
	}
	if asked != 1 {
		t.Errorf("Ask called %d times, want 1", asked)
	}

	// A granted directory and its subdirectories never ask again.
	if !m.AllowWrite(dir) || !m.AllowWrite(filepath.Join(dir, "sub", "dir")) {
		t.Fatal("AllowWrite = false for a granted dir or its subdirectory")
	}
	if asked != 1 {
		t.Errorf("Ask called %d times after the grant, want 1", asked)
	}

	// The grant persists: a fresh manager loads it without asking.
	reloaded, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	reloaded.Ask = func(string) bool {
		t.Error("Ask called for an already-persisted grant")
		return false
	}
	if !reloaded.AllowWrite(dir) || !reloaded.AllowWrite(filepath.Join(dir, "sub")) {
		t.Fatal("reloaded manager does not allow a granted dir or its subdirectory")
	}
}

func TestAllowWriteDenied(t *testing.T) {
	m, err := Load(testPath(t))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	m.Ask = func(string) bool { return false }

	dir := t.TempDir()
	if m.AllowWrite(dir) {
		t.Fatal("AllowWrite = true after a denial, want false")
	}
	// A denial is not persisted; the next attempt asks again.
	if m.AllowWrite(dir) {
		t.Fatal("AllowWrite = true on the second denial, want false")
	}
}

func TestAllowWriteBoundaries(t *testing.T) {
	m, err := Load(testPath(t))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	m.Ask = func(string) bool { return false }

	root := t.TempDir()
	granted := filepath.Join(root, "project")
	m.writeDirs = []string{granted}

	for _, dir := range []string{
		granted,
		filepath.Join(granted, "sub"),
	} {
		if !m.AllowWrite(dir) {
			t.Errorf("AllowWrite(%q) = false, want true (inside the grant)", dir)
		}
	}
	for _, dir := range []string{
		root,                         // parent of the grant
		granted + "-other",           // sibling with a shared prefix
		filepath.Join(root, "other"), // sibling directory
	} {
		if m.AllowWrite(dir) {
			t.Errorf("AllowWrite(%q) = true, want false (outside the grant)", dir)
		}
	}
}

func TestAllowWriteNormalizes(t *testing.T) {
	m, err := Load(testPath(t))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	var askedDir string
	m.Ask = func(dir string) bool { askedDir = dir; return true }

	if !m.AllowWrite(".") {
		t.Fatal("AllowWrite(.) = false after a grant")
	}
	if !filepath.IsAbs(askedDir) {
		t.Errorf("Ask dir = %q, want an absolute path", askedDir)
	}
	if dirs := m.WriteDirectories(); len(dirs) != 1 || dirs[0] != askedDir {
		t.Errorf("WriteDirectories() = %v, want [%q]", dirs, askedDir)
	}
}

func TestLoadMissing(t *testing.T) {
	m, err := Load(filepath.Join(t.TempDir(), "nope.json"))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if m.AllowWrite(t.TempDir()) {
		t.Error("AllowWrite = true with no grants and no Ask, want false")
	}
}

func TestLoadCorrupt(t *testing.T) {
	path := testPath(t)
	if err := os.WriteFile(path, []byte("{nope"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(path); err == nil {
		t.Fatal("Load: expected error for corrupt JSON, got nil")
	}
}
