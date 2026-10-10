package config

import (
	"bytes"
	"encoding/json"
	"fmt"
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

// TestNormalizePreconfiguredProvider documents that a preconfigured provider
// (OpenAI / nVidia / OpenCode) is filled in with its fixed name and base URL,
// so only the API key needs to be supplied.
func TestNormalizePreconfiguredProvider(t *testing.T) {
	for _, tc := range []struct {
		typ     ModelProviderType
		name    string
		baseURL string
	}{
		{ModelProviderTypeOpenAI, "OpenAI", "https://api.openai.com/v1"},
		{ModelProviderTypeNvidia, "nVidia", "https://integrate.api.nvidia.com/v1"},
		{ModelProviderTypeOpenCode, "OpenCode", "https://opencode.ai/v1"},
	} {
		got := NormalizeModelProvider(ModelProvider{Type: tc.typ, APIKey: NewAPIKey("k")})
		if got.Name != tc.name {
			t.Errorf("%s: Name = %q, want %q", tc.typ, got.Name, tc.name)
		}
		if got.BaseURL != tc.baseURL {
			t.Errorf("%s: BaseURL = %q, want %q", tc.typ, got.BaseURL, tc.baseURL)
		}
		if got.APIKey.String() != "k" {
			t.Errorf("%s: APIKey = %q, want %q", tc.typ, got.APIKey.String(), "k")
		}
	}
}

// TestNormalizeOpenAICompatibleUntouched documents that a custom OpenAI
// Compatible provider keeps the user-supplied name, base URL and key and is
// not given any defaults.
func TestNormalizeOpenAICompatibleUntouched(t *testing.T) {
	got := NormalizeModelProvider(ModelProvider{
		Type:    ModelProviderTypeOpenAICompatible,
		Name:    "My Local",
		BaseURL: "http://localhost:8080/v1",
		APIKey:  NewAPIKey("abc"),
	})
	want := ModelProvider{
		Type:    ModelProviderTypeOpenAICompatible,
		Name:    "My Local",
		BaseURL: "http://localhost:8080/v1",
		APIKey:  NewAPIKey("abc"),
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Normalize = %+v, want %+v", got, want)
	}
}

// TestAddRemoveModelProvider checks that providers can be added (normalized)
// and removed by name.
func TestAddRemoveModelProvider(t *testing.T) {
	c := &GConfig{}
	c.AddModelProvider(ModelProvider{Type: ModelProviderTypeNvidia, APIKey: NewAPIKey("nv")})
	c.AddModelProvider(ModelProvider{Type: ModelProviderTypeOpenAICompatible, Name: "Mine", BaseURL: "http://x/v1"})

	if len(c.ModelProviders) != 2 {
		t.Fatalf("len(ModelProviders) = %d, want 2", len(c.ModelProviders))
	}
	if c.ModelProviders[0].Name != "nVidia" || c.ModelProviders[0].BaseURL != "https://integrate.api.nvidia.com/v1" {
		t.Errorf("normalized nVidia provider = %+v", c.ModelProviders[0])
	}

	if !c.RemoveModelProvider("nVidia") {
		t.Error("RemoveModelProvider(nVidia) = false, want true")
	}
	if len(c.ModelProviders) != 1 || c.ModelProviders[0].Name != "Mine" {
		t.Errorf("after removal ModelProviders = %+v, want just Mine", c.ModelProviders)
	}
	if c.RemoveModelProvider("nope") {
		t.Error("RemoveModelProvider(nope) = true, want false")
	}
}

// TestSaveObfuscatesAPIKey checks that an API key is written to the config
// file in obfuscated form: the file must not contain the plain key, and a
// fresh load must recover it via String().
func TestSaveObfuscatesAPIKey(t *testing.T) {
	home := withHome(t)

	c, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	const secret = "sk-super-secret-key"
	c.AddModelProvider(ModelProvider{Type: ModelProviderTypeOpenAI, APIKey: NewAPIKey(secret)})
	if err := c.Save(); err != nil {
		t.Fatalf("Save: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(home, ".guac", configFileName))
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if bytes.Contains(data, []byte(secret)) {
		t.Error("config file contains the plain API key, want it obfuscated")
	}

	c2, err := Load()
	if err != nil {
		t.Fatalf("Load (second): %v", err)
	}
	p, ok := c2.GetModelProvider(0)
	if !ok {
		t.Fatal("provider missing after reload")
	}
	if got := p.APIKey.String(); got != secret {
		t.Errorf("APIKey = %q, want %q", got, secret)
	}
}

// TestSaveNullAPIKey documents that a provider without an API key is written
// as an explicit null rather than a plain empty string.
func TestSaveNullAPIKey(t *testing.T) {
	home := withHome(t)

	c, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	c.AddModelProvider(ModelProvider{
		Type:    ModelProviderTypeOpenAICompatible,
		Name:    "Mine",
		BaseURL: "http://x/v1",
	})
	if err := c.Save(); err != nil {
		t.Fatalf("Save: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(home, ".guac", configFileName))
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if !bytes.Contains(data, []byte(`"apiKey": null`)) {
		t.Errorf("saved file = %s, want an explicit null apiKey", data)
	}
}

// TestLoadUpgradesLegacyPlaintextAPIKey checks that a config file written by
// an older version (plain-string apiKey) still loads, and that the key is
// stored obfuscated again on the next save.
func TestLoadUpgradesLegacyPlaintextAPIKey(t *testing.T) {
	home := withHome(t)
	dir := filepath.Join(home, ".guac")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	const legacy = "nvapi-legacy-plain-key"
	raw := fmt.Sprintf(`{
		"modelProviders": [
			{"name": "nVidia", "type": "nVidia", "baseUrl": "https://integrate.api.nvidia.com/v1", "apiKey": %q}
		]
	}`, legacy)
	if err := os.WriteFile(filepath.Join(dir, configFileName), []byte(raw), 0o644); err != nil {
		t.Fatal(err)
	}

	c, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	p, ok := c.GetModelProvider(0)
	if !ok {
		t.Fatal("provider missing after legacy load")
	}
	if got := p.APIKey.String(); got != legacy {
		t.Errorf("APIKey = %q, want %q", got, legacy)
	}
	if err := c.Save(); err != nil {
		t.Fatalf("Save: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(dir, configFileName))
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if bytes.Contains(data, []byte(legacy)) {
		t.Error("config file still contains the plain API key after save")
	}
}
