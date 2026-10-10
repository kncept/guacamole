package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/kncept/guacamole/utils/encryption"
)

// configFileName is GConfig's file name, stored in GuacDir().
const configFileName = "config.json"

// apiKeyObfuscationKey is the XOR key API keys are stored with in
// config.json. It is not a secret: the obfuscation only keeps keys from
// being readable straight out of the file, not from someone who also has
// this source.
const apiKeyObfuscationKey = "guacamole-api-key-obfuscation"

// Policy is what guacamole does with a tool call that matches a permission
// rule.
type Policy string

const (
	PolicyAllow Policy = "allow"
	PolicyDeny  Policy = "deny"
	PolicyAsk   Policy = "ask"
)

// Permissions holds every permission rule, grouped by category: filesystem,
// shell and web.
type Permissions struct {
	Filesystem FilesystemPermissions `json:"filesystem"`
	Shell      ShellPermissions      `json:"shell"`
	Web        WebPermissions        `json:"web"`
}

// FilesystemPermissions tracks read and write access per directory.
// AllowAll is an override: when set, every path is allowed without asking
// and the directory rules are ignored.
type FilesystemPermissions struct {
	AllowAll    bool                  `json:"allowAll"`
	Directories []DirectoryPermission `json:"directories"`
}

// DirectoryPermission is one rule: reads and writes under Directory (and
// everything below it) follow the given policies.
type DirectoryPermission struct {
	Directory string `json:"directory"`
	Read      Policy `json:"read"`
	Write     Policy `json:"write"`
}

// ShellPermissions is a single allow/ask/deny policy shared by every shell
// operation; unlike the other categories it has no per-value rules.
type ShellPermissions struct {
	Policy Policy `json:"policy"`
}

// WebPermissions tracks access per domain. AllowAll is an override: when
// set, every domain is allowed without asking and the domain rules are
// ignored.
type WebPermissions struct {
	AllowAll bool               `json:"allowAll"`
	Domains  []DomainPermission `json:"domains"`
}

// DomainPermission is one rule: requests to Domain (and its subdomains)
// follow Policy.
type DomainPermission struct {
	Domain string `json:"domain"`
	Policy Policy `json:"policy"`
}

// ModelProviderType represents the type of model provider.
type ModelProviderType string

const (
	// ModelProviderTypeOpenAICompatible is a user-defined OpenAI-compatible
	// endpoint: the name and base URL are entered, and the API key is optional.
	ModelProviderTypeOpenAICompatible ModelProviderType = "OpenAI Compatible"
	// ModelProviderTypeOpenAI is the preconfigured OpenAI provider.
	ModelProviderTypeOpenAI ModelProviderType = "OpenAI"
	// ModelProviderTypeNvidia is the preconfigured nVidia (NIM) provider.
	ModelProviderTypeNvidia ModelProviderType = "nVidia"
	// ModelProviderTypeOpenCode is the preconfigured OpenCode provider.
	ModelProviderTypeOpenCode ModelProviderType = "OpenCode"
)

// providerDefaults holds the fixed name and base URL for the providers that
// ship preconfigured: for these, only the API key is user-supplied.
var providerDefaults = map[ModelProviderType]struct {
	Name    string
	BaseURL string
}{
	ModelProviderTypeOpenAI:   {Name: "OpenAI", BaseURL: "https://api.openai.com/v1"},
	ModelProviderTypeNvidia:   {Name: "nVidia", BaseURL: "https://integrate.api.nvidia.com/v1"},
	ModelProviderTypeOpenCode: {Name: "OpenCode", BaseURL: "https://opencode.ai/v1"},
}

// IsPreconfigured reports whether t is a provider that ships with a fixed
// name and base URL, so the user only supplies the API key.
func (t ModelProviderType) IsPreconfigured() bool {
	_, ok := providerDefaults[t]
	return ok
}

// PreconfiguredName returns the default display name for a preconfigured
// provider, or an empty string when t is not preconfigured.
func (t ModelProviderType) PreconfiguredName() string {
	if d, ok := providerDefaults[t]; ok {
		return d.Name
	}
	return ""
}

// PreconfiguredBaseURL returns the fixed base URL for a preconfigured
// provider, or an empty string when t is not preconfigured.
func (t ModelProviderType) PreconfiguredBaseURL() string {
	if d, ok := providerDefaults[t]; ok {
		return d.BaseURL
	}
	return ""
}

// ModelProvider represents a configured model provider.
type ModelProvider struct {
	// Name is the user-friendly name of the provider (e.g., "OpenAI",
	// "nVidia", "OpenCode", or a custom name for OpenAI Compatible).
	Name string `json:"name"`
	// Type is the type of provider.
	Type ModelProviderType `json:"type"`
	// BaseURL is the base URL for the provider's API.
	BaseURL string `json:"baseUrl,omitempty"`
	// APIKey is the API key for the provider (optional for OpenAI
	// Compatible), stored obfuscated so it is not readable straight out of
	// the config file.
	APIKey encryption.ObfuscatedValue `json:"apiKey"`
	// Models is the list of available model names.
	Models []string `json:"models,omitempty"`
}

// GConfig is the basis of the ~/.guac/config.json file: the directories
// guacamole uses, the category-based permission rules, and model providers.
type GConfig struct {
	// SessionsDir is where saved sessions live. Left empty it defaults to
	// ~/.guac/session via InitDefaults.
	SessionsDir string `json:"sessionsDir"`
	// Permissions holds the permission rules of each category.
	Permissions Permissions `json:"permissions"`
	// ModelProviders holds the configured model providers.
	ModelProviders []ModelProvider `json:"modelProviders"`
}

// NewAPIKey returns an API key ready to store in a ModelProvider: XOR-
// obfuscated so it is not readable straight out of the config file.
func NewAPIKey(key string) encryption.ObfuscatedValue {
	return encryption.NewObfuscatedValue(key, apiKeyObfuscationKey)
}

// Load reads GConfig from GuacDir()/config.json — an empty config when the
// file does not exist yet — and applies InitDefaults.
func Load() (*GConfig, error) {
	dir, err := GuacDir()
	if err != nil {
		return nil, err
	}

	c := &GConfig{}
	data, err := os.ReadFile(filepath.Join(dir, configFileName))
	if err != nil {
		if !os.IsNotExist(err) {
			return nil, err
		}
		// First run: no config file yet.
	} else if err := json.Unmarshal(data, c); err != nil {
		return nil, fmt.Errorf("config: parse %s: %w", configFileName, err)
	}

	c.InitDefaults()
	return c, nil
}

// InitDefaults populates any required members that are not set, like
// SessionsDir. Values that are already present are never overwritten.
func (this *GConfig) InitDefaults() {
	if this.SessionsDir == "" {
		if dir, err := GuacDir(); err == nil {
			this.SessionsDir = filepath.Join(dir, "session")
		}
	}
	if this.Permissions.Shell.Policy == "" {
		// An unset shell policy means "ask before every shell command".
		this.Permissions.Shell.Policy = PolicyAsk
	}
	if this.ModelProviders == nil {
		this.ModelProviders = []ModelProvider{}
	}
	// Upgrade API keys to obfuscated form. This re-wraps legacy values that
	// still sit in the file as plain text and is a no-op for values already
	// stored obfuscated, so it is safe to run on every load.
	for i := range this.ModelProviders {
		this.ModelProviders[i].APIKey = this.ModelProviders[i].APIKey.Reobfuscated(apiKeyObfuscationKey)
	}
}

// AddModelProvider adds a new model provider to the config, filling in the
// preconfigured name and base URL for providers that ship with fixed
// connection details.
func (this *GConfig) AddModelProvider(provider ModelProvider) {
	if this.ModelProviders == nil {
		this.ModelProviders = []ModelProvider{}
	}
	provider = NormalizeModelProvider(provider)
	this.ModelProviders = append(this.ModelProviders, provider)
}

// RemoveModelProvider removes the first provider with the given name. It
// reports whether a provider was removed.
func (this *GConfig) RemoveModelProvider(name string) bool {
	for i, p := range this.ModelProviders {
		if p.Name == name {
			this.ModelProviders = append(this.ModelProviders[:i], this.ModelProviders[i+1:]...)
			return true
		}
	}
	return false
}

// UpdateModelProvider replaces the provider at index i with the new provider.
func (this *GConfig) UpdateModelProvider(i int, provider ModelProvider) {
	if this.ModelProviders == nil || i < 0 || i >= len(this.ModelProviders) {
		return
	}
	provider = NormalizeModelProvider(provider)
	this.ModelProviders[i] = provider
}

// GetModelProvider returns the provider at index i (or zero value if out of range).
func (this *GConfig) GetModelProvider(i int) (ModelProvider, bool) {
	if this.ModelProviders == nil || i < 0 || i >= len(this.ModelProviders) {
		return ModelProvider{}, false
	}
	return this.ModelProviders[i], true
}

// NormalizeModelProvider fills in the preconfigured name and base URL for a
// provider that ships with fixed connection details, leaving user-supplied
// values untouched.
func NormalizeModelProvider(provider ModelProvider) ModelProvider {
	if provider.Type.IsPreconfigured() {
		if provider.Name == "" {
			provider.Name = provider.Type.PreconfiguredName()
		}
		if provider.BaseURL == "" {
			provider.BaseURL = provider.Type.PreconfiguredBaseURL()
		}
	}
	return provider
}

// Save writes the config back to GuacDir()/config.json, creating the
// directory if needed.
func (this *GConfig) Save() error {
	dir, err := GuacDir()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(this, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, configFileName), data, 0o644)
}
