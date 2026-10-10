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

// TestNormalizeFillsDefaultsByAPIType documents that NormalizeModelProvider
// fills in a name and base URL derived from the API type, so a provider only
// needs its API type (and usually its key) to be usable.
func TestNormalizeFillsDefaultsByAPIType(t *testing.T) {
	for _, tc := range []struct {
		typ     ModelProviderType
		name    string
		baseURL string
	}{
		{ModelProviderTypeOpenAI, "OpenAI", "https://api.openai.com/v1"},
		{ModelProviderTypeAnthropic, "Anthropic", "https://api.anthropic.com/v1"},
		{ModelProviderTypeOpenCode, "OpenCode", "https://opencode.ai/zen/v1"},
	} {
		got := NormalizeModelProvider(ModelProvider{APIType: tc.typ, APIKey: NewAPIKey("k")})
		if got.Name != tc.name {
			t.Errorf("%s: Name = %q, want %q", tc.typ, got.Name, tc.name)
		}
		if got.BaseURL != tc.baseURL {
			t.Errorf("%s: BaseURL = %q, want %q", tc.typ, got.BaseURL, tc.baseURL)
		}
		if got.APIType != tc.typ {
			t.Errorf("%s: APIType = %q, want %q", tc.typ, got.APIType, tc.typ)
		}
		if got.APIKey.String() != "k" {
			t.Errorf("%s: APIKey = %q, want %q", tc.typ, got.APIKey.String(), "k")
		}
	}
}

// TestNormalizeKeepsSuppliedValues documents that a provider with explicit
// values keeps them and is not given defaults.
func TestNormalizeKeepsSuppliedValues(t *testing.T) {
	got := NormalizeModelProvider(ModelProvider{
		APIType: ModelProviderTypeOpenAI,
		Name:    "My Local",
		BaseURL: "http://localhost:8080/v1",
		APIKey:  NewAPIKey("abc"),
	})
	want := ModelProvider{
		APIType: ModelProviderTypeOpenAI,
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
	c.AddModelProvider(ModelProvider{APIType: ModelProviderTypeAnthropic, Name: "Anthropic"})
	c.AddModelProvider(ModelProvider{APIType: ModelProviderTypeOpenAI, Name: "Mine", BaseURL: "http://x/v1"})

	if len(c.ModelProviders) != 2 {
		t.Fatalf("len(ModelProviders) = %d, want 2", len(c.ModelProviders))
	}
	if c.ModelProviders[0].Name != "Anthropic" || c.ModelProviders[0].APIType != ModelProviderTypeAnthropic {
		t.Errorf("added Anthropic provider = %+v", c.ModelProviders[0])
	}

	if !c.RemoveModelProvider("Anthropic") {
		t.Error("RemoveModelProvider(Anthropic) = false, want true")
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
	c.AddModelProvider(ModelProvider{APIType: ModelProviderTypeOpenAI, Name: "OpenAI", APIKey: NewAPIKey(secret)})
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
		APIType: ModelProviderTypeOpenAI,
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

// TestLoadMigratesExecutionShape checks that a config file written with the
// older execution/listing provider shape is migrated onto the current
// API type / base URL / API key fields.
func TestLoadMigratesExecutionShape(t *testing.T) {
	home := withHome(t)
	dir := filepath.Join(home, ".guac")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	raw := `{
		"modelProviders": [
			{
				"name": "nVidia",
				"executionType": "nVidia",
				"executionBaseUrl": "https://integrate.api.nvidia.com/v1",
				"executionApiKey": "nv-key",
				"listingType": "nVidia",
				"listingBaseUrl": "https://integrate.api.nvidia.com/v1",
				"models": ["meta/llama-3.1-8b-instruct"]
			},
			{
				"name": "OpenCode",
				"executionType": "OpenCode",
				"executionBaseUrl": "https://opencode.ai/v1",
				"listingType": "OpenCode",
				"listingBaseUrl": "https://opencode.ai"
			}
		]
	}`
	if err := os.WriteFile(filepath.Join(dir, configFileName), []byte(raw), 0o644); err != nil {
		t.Fatal(err)
	}

	c, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	nv, ok := c.GetModelProvider(0)
	if !ok {
		t.Fatal("nVidia provider missing after migration")
	}
	if nv.APIType != ModelProviderTypeOpenAI {
		t.Errorf("nVidia APIType = %q, want %q", nv.APIType, ModelProviderTypeOpenAI)
	}
	if nv.BaseURL != "https://integrate.api.nvidia.com/v1" {
		t.Errorf("nVidia BaseURL = %q, want the migrated execution base URL", nv.BaseURL)
	}
	if got := nv.APIKey.String(); got != "nv-key" {
		t.Errorf("nVidia APIKey = %q, want %q", got, "nv-key")
	}
	if len(nv.Models) != 1 || nv.Models[0] != "meta/llama-3.1-8b-instruct" {
		t.Errorf("nVidia Models = %v, want the migrated list", nv.Models)
	}

	oc, ok := c.GetModelProvider(1)
	if !ok {
		t.Fatal("OpenCode provider missing after migration")
	}
	if oc.APIType != ModelProviderTypeOpenCode {
		t.Errorf("OpenCode APIType = %q, want %q", oc.APIType, ModelProviderTypeOpenCode)
	}
}
