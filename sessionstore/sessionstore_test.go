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
