package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

// withHome points HOME at a fresh temp dir so tests never touch the real
// ~/.guac config.
func withHome(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	return home
}

func TestLoadMissingUsesDefaults(t *testing.T) {
	home := withHome(t)

	c, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	want := filepath.Join(home, ".guac", "session")
	if c.SessionsDir != want {
		t.Errorf("SessionsDir = %q, want %q", c.SessionsDir, want)
	}
	if c.Permissions == nil {
		t.Error("Permissions = nil, want an initialized map")
	}
	if got := c.GetToolPermissions("read_file"); len(got) != 0 {
		t.Errorf("GetToolPermissions(read_file) = %v, want empty", got)
	}
}

func TestLoadInvalidJSON(t *testing.T) {
	home := withHome(t)
	dir := filepath.Join(home, ".guac")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, configFileName), []byte("{not json"), 0o644); err != nil {
		t.Fatal(err)
	}

	if _, err := Load(); err == nil {
		t.Fatal("Load: expected an error for a corrupt config file, got nil")
	}
}

func TestLoadReadsExistingFile(t *testing.T) {
	home := withHome(t)
	dir := filepath.Join(home, ".guac")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	raw := `{
		"sessionsDir": "/custom/session",
		"permissions": {
			"read_file": [
				{"value": "/tmp", "policy": "allow"},
				{"value": "/etc", "policy": "deny"}
			]
		}
	}`
	if err := os.WriteFile(filepath.Join(dir, configFileName), []byte(raw), 0o644); err != nil {
		t.Fatal(err)
	}

	c, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if c.SessionsDir != "/custom/session" {
		t.Errorf("SessionsDir = %q, want %q", c.SessionsDir, "/custom/session")
	}
	want := ToolPermissions{
		{Value: "/tmp", Policy: PolicyAllow},
		{Value: "/etc", Policy: PolicyDeny},
	}
	if got := c.GetToolPermissions("read_file"); !reflect.DeepEqual(got, want) {
		t.Errorf("GetToolPermissions(read_file) = %v, want %v", got, want)
	}
	if got := c.GetToolPermissions("bash"); len(got) != 0 {
		t.Errorf("GetToolPermissions(bash) = %v, want empty", got)
	}
}

func TestSaveLoadRoundTrip(t *testing.T) {
	home := withHome(t)

	c, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	c.SetToolPermissions("read_file", ToolPermissions{
		{Value: "/tmp", Policy: PolicyAllow},
		{Value: "/etc", Policy: PolicyDeny},
	})
	c.SetToolPermissions("bash", ToolPermissions{{Value: "*", Policy: PolicyAsk}})
	if err := c.Save(); err != nil {
		t.Fatalf("Save: %v", err)
	}

	// The file must exist and carry the expected JSON shape.
	data, err := os.ReadFile(filepath.Join(home, ".guac", configFileName))
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	var raw struct {
		SessionsDir string                  `json:"sessionsDir"`
		Permissions map[string][]Permission `json:"permissions"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("saved file is not JSON: %v", err)
	}
	if raw.SessionsDir == "" {
		t.Error("saved sessionsDir is empty, want the defaulted path")
	}
	if got, want := len(raw.Permissions["read_file"]), 2; got != want {
		t.Errorf("saved read_file rules = %d, want %d", got, want)
	}

	c2, err := Load()
	if err != nil {
		t.Fatalf("Load (second): %v", err)
	}
	if c2.SessionsDir != c.SessionsDir {
		t.Errorf("SessionsDir = %q, want %q", c2.SessionsDir, c.SessionsDir)
	}
	if got := c2.GetToolPermissions("bash"); !reflect.DeepEqual(got, ToolPermissions{{Value: "*", Policy: PolicyAsk}}) {
		t.Errorf("GetToolPermissions(bash) = %v, want one ask rule", got)
	}
}

func TestInitDefaultsKeepsSetValues(t *testing.T) {
	c := &GConfig{SessionsDir: "/custom/session"}
	c.InitDefaults()
	if c.SessionsDir != "/custom/session" {
		t.Errorf("SessionsDir = %q, want the existing value kept", c.SessionsDir)
	}
	if c.Permissions == nil {
		t.Error("Permissions = nil, want an initialized map")
	}
}

func TestSetToolPermissionsOnZeroConfig(t *testing.T) {
	c := &GConfig{}
	c.SetToolPermissions("ls", ToolPermissions{{Value: ".", Policy: PolicyAllow}})
	if c.Permissions == nil {
		t.Fatal("Permissions = nil after SetToolPermissions")
	}
	want := ToolPermissions{{Value: ".", Policy: PolicyAllow}}
	if got := c.GetToolPermissions("ls"); !reflect.DeepEqual(got, want) {
		t.Errorf("GetToolPermissions(ls) = %v, want %v", got, want)
	}
}
