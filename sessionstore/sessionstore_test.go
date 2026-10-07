package sessionstore

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/kncept/guacamole/ai"
)

func testSession() *ai.Session {
	session := ai.NewSession(nil,
		ai.Message{Role: ai.RoleUser, Content: "hello"},
		ai.Message{Role: ai.RoleAssistant, Content: "hi there"},
	)
	session.Model = "m1"
	session.SystemPrompt = "be brief"
	return session
}

func TestSaveLoadRoundTrip(t *testing.T) {
	dir := t.TempDir()
	session := testSession()

	path, err := Save(dir, session)
	if err != nil {
		t.Fatalf("Save: %v", err)
	}
	if want := filepath.Join(dir, session.ID+".json"); path != want {
		t.Errorf("path = %q, want %q", path, want)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	var f map[string]any
	if err := json.Unmarshal(data, &f); err != nil {
		t.Fatalf("saved file is not JSON: %v", err)
	}
	if f["id"] != session.ID {
		t.Errorf("saved id = %v, want %q", f["id"], session.ID)
	}
	if msgs, ok := f["messages"].([]any); !ok || len(msgs) != 2 {
		t.Errorf("saved messages = %v, want 2 entries", f["messages"])
	}

	loaded, err := Load(dir, session.ID, nil)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if loaded.ID != session.ID {
		t.Errorf("ID = %q, want %q", loaded.ID, session.ID)
	}
	if loaded.Model != "m1" || loaded.SystemPrompt != "be brief" {
		t.Errorf("Model = %q, SystemPrompt = %q; want %q, %q", loaded.Model, loaded.SystemPrompt, "m1", "be brief")
	}
	got := loaded.Messages()
	want := []ai.Message{
		{Role: ai.RoleUser, Content: "hello"},
		{Role: ai.RoleAssistant, Content: "hi there"},
	}
	if len(got) != len(want) {
		t.Fatalf("Messages() = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("Messages()[%d] = %+v, want %+v", i, got[i], want[i])
		}
	}
}

func TestLoadMissing(t *testing.T) {
	if _, err := Load(t.TempDir(), "abcdef0123456789", nil); err == nil {
		t.Fatal("Load: expected error for a missing session, got nil")
	}
}

func TestInvalidIDs(t *testing.T) {
	for _, id := range []string{"", "..", "../escape", "a/b", `a\b`} {
		if _, err := Load(t.TempDir(), id, nil); err == nil {
			t.Errorf("Load(%q): expected error, got nil", id)
		}

		session := ai.NewSession(nil)
		session.ID = id
		if _, err := Save(t.TempDir(), session); err == nil {
			t.Errorf("Save with ID %q: expected error, got nil", id)
		}
	}
}

func TestList(t *testing.T) {
	// Nothing saved yet: no sessions, and no error.
	if ids, err := List(t.TempDir()); err != nil || len(ids) != 0 {
		t.Fatalf("List(empty dir) = %v, %v; want no sessions and no error", ids, err)
	}

	dir := t.TempDir()
	for _, id := range []string{"bravo", "alpha", "charlie"} {
		session := ai.NewSession(nil)
		session.ID = id
		if _, err := Save(dir, session); err != nil {
			t.Fatalf("Save(%q): %v", id, err)
		}
	}
	// Files that are not saved sessions must not show up.
	if err := os.WriteFile(filepath.Join(dir, "notes.txt"), []byte("hi"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "not an id.json"), []byte("{}"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	ids, err := List(dir)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	want := []string{"alpha", "bravo", "charlie"}
	if len(ids) != len(want) {
		t.Fatalf("List = %v, want %v", ids, want)
	}
	for i := range want {
		if ids[i] != want[i] {
			t.Errorf("List[%d] = %q, want %q", i, ids[i], want[i])
		}
	}
}

func TestDelete(t *testing.T) {
	dir := t.TempDir()
	session := ai.NewSession(nil)
	session.ID = "gone"
	if _, err := Save(dir, session); err != nil {
		t.Fatalf("Save: %v", err)
	}

	if err := Delete(dir, "gone"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if ids, err := List(dir); err != nil || len(ids) != 0 {
		t.Errorf("List after Delete = %v, %v; want no sessions", ids, err)
	}
	if err := Delete(dir, "gone"); err == nil {
		t.Error("Delete of an already deleted session: expected error, got nil")
	}
	for _, id := range []string{"", "..", "../escape"} {
		if err := Delete(dir, id); err == nil {
			t.Errorf("Delete(%q): expected error, got nil", id)
		}
	}
}
