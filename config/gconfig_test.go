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
	if c.Permissions.Shell.Policy != PolicyAsk {
		t.Errorf("Shell policy = %q, want %q", c.Permissions.Shell.Policy, PolicyAsk)
	}
	if c.Permissions.Filesystem.AllowAll {
		t.Error("Filesystem.AllowAll = true, want false")
	}
	if len(c.Permissions.Filesystem.Directories) != 0 {
		t.Errorf("Filesystem directories = %v, want empty", c.Permissions.Filesystem.Directories)
	}
	if c.Permissions.Web.AllowAll {
		t.Error("Web.AllowAll = true, want false")
	}
	if len(c.Permissions.Web.Domains) != 0 {
		t.Errorf("Web domains = %v, want empty", c.Permissions.Web.Domains)
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
			"filesystem": {
				"allowAll": false,
				"directories": [
					{"directory": "/tmp", "read": "allow", "write": "allow"},
					{"directory": "/etc", "read": "deny", "write": "deny"}
				]
			},
			"shell": {"policy": "allow"},
			"web": {
				"allowAll": true,
				"domains": [{"domain": "example.com", "policy": "deny"}]
			}
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
	wantDirs := []DirectoryPermission{
		{Directory: "/tmp", Read: PolicyAllow, Write: PolicyAllow},
		{Directory: "/etc", Read: PolicyDeny, Write: PolicyDeny},
	}
	if got := c.Permissions.Filesystem.Directories; !reflect.DeepEqual(got, wantDirs) {
		t.Errorf("Filesystem directories = %v, want %v", got, wantDirs)
	}
	if c.Permissions.Shell.Policy != PolicyAllow {
		t.Errorf("Shell policy = %q, want %q", c.Permissions.Shell.Policy, PolicyAllow)
	}
	if !c.Permissions.Web.AllowAll {
		t.Error("Web.AllowAll = false, want true")
	}
	wantDomains := []DomainPermission{{Domain: "example.com", Policy: PolicyDeny}}
	if got := c.Permissions.Web.Domains; !reflect.DeepEqual(got, wantDomains) {
		t.Errorf("Web domains = %v, want %v", got, wantDomains)
	}
}

// TestLoadIgnoresLegacyPerToolPermissions documents the deliberate start
// fresh decision: the old per-tool permission format is not migrated, it
// simply loads as an empty category config.
func TestLoadIgnoresLegacyPerToolPermissions(t *testing.T) {
	home := withHome(t)
	dir := filepath.Join(home, ".guac")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	raw := `{
		"sessionsDir": "/custom/session",
		"permissions": {
			"read_file": [{"value": "/tmp", "read": "allow", "write": "allow"}],
			"bash": [{"value": "*", "read": "ask", "write": "ask"}]
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
	if len(c.Permissions.Filesystem.Directories) != 0 {
		t.Errorf("Filesystem directories = %v, want empty (legacy rules are not migrated)", c.Permissions.Filesystem.Directories)
	}
	if c.Permissions.Shell.Policy != PolicyAsk {
		t.Errorf("Shell policy = %q, want %q", c.Permissions.Shell.Policy, PolicyAsk)
	}
}

func TestSaveLoadRoundTrip(t *testing.T) {
	home := withHome(t)

	c, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	c.Permissions.Filesystem.Directories = []DirectoryPermission{
		{Directory: "/tmp", Read: PolicyAllow, Write: PolicyAllow},
		{Directory: "/etc", Read: PolicyDeny, Write: PolicyDeny},
	}
	c.Permissions.Shell.Policy = PolicyDeny
	c.Permissions.Web.Domains = []DomainPermission{{Domain: "example.com", Policy: PolicyAsk}}
	if err := c.Save(); err != nil {
		t.Fatalf("Save: %v", err)
	}

	// The file must exist and carry the expected JSON shape.
	data, err := os.ReadFile(filepath.Join(home, ".guac", configFileName))
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	var raw struct {
		SessionsDir string `json:"sessionsDir"`
		Permissions struct {
			Filesystem struct {
				AllowAll    bool                  `json:"allowAll"`
				Directories []DirectoryPermission `json:"directories"`
			} `json:"filesystem"`
			Shell struct {
				Policy Policy `json:"policy"`
			} `json:"shell"`
			Web struct {
				AllowAll bool               `json:"allowAll"`
				Domains  []DomainPermission `json:"domains"`
			} `json:"web"`
		} `json:"permissions"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("saved file is not JSON: %v", err)
	}
	if raw.SessionsDir == "" {
		t.Error("saved sessionsDir is empty, want the defaulted path")
	}
	if got, want := len(raw.Permissions.Filesystem.Directories), 2; got != want {
		t.Errorf("saved directory rules = %d, want %d", got, want)
	}
	if got := raw.Permissions.Shell.Policy; got != PolicyDeny {
		t.Errorf("saved shell policy = %q, want %q", got, PolicyDeny)
	}

	c2, err := Load()
	if err != nil {
		t.Fatalf("Load (second): %v", err)
	}
	if c2.SessionsDir != c.SessionsDir {
		t.Errorf("SessionsDir = %q, want %q", c2.SessionsDir, c.SessionsDir)
	}
	if !reflect.DeepEqual(c2.Permissions, c.Permissions) {
		t.Errorf("Permissions = %v, want %v", c2.Permissions, c.Permissions)
	}
}

func TestInitDefaultsKeepsSetValues(t *testing.T) {
	c := &GConfig{
		SessionsDir: "/custom/session",
		Permissions: Permissions{Shell: ShellPermissions{Policy: PolicyAllow}},
	}
	c.InitDefaults()
	if c.SessionsDir != "/custom/session" {
		t.Errorf("SessionsDir = %q, want the existing value kept", c.SessionsDir)
	}
	if c.Permissions.Shell.Policy != PolicyAllow {
		t.Errorf("Shell policy = %q, want the existing value kept", c.Permissions.Shell.Policy)
	}
}

func TestInitDefaultsShellPolicy(t *testing.T) {
	c := &GConfig{}
	c.InitDefaults()
	if c.Permissions.Shell.Policy != PolicyAsk {
		t.Errorf("Shell policy = %q, want the %q default", c.Permissions.Shell.Policy, PolicyAsk)
	}
}
