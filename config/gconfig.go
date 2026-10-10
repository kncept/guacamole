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

// ModelProviderType is the wire API a provider speaks, used for both
// inference and model listing.
type ModelProviderType string

const (
	// ModelProviderTypeOpenAI is the standard OpenAI (and OpenAI-compatible)
	// API: Responses-style inference and the /v1/models listing endpoint.
	ModelProviderTypeOpenAI ModelProviderType = "OpenAI"
	// ModelProviderTypeAnthropic is the Anthropic Messages API.
	ModelProviderTypeAnthropic ModelProviderType = "Anthropic"
	// ModelProviderTypeOpenCode is the OpenCode override: it infers through
	// the OpenAI-compatible API but lists models from OpenCode's own
	// (non-standard) endpoint.
	ModelProviderTypeOpenCode ModelProviderType = "OpenCode"
)

// SelectableAPITypes lists the API types a custom provider may choose.
// OpenCode is deliberately absent: it is applied automatically by the
// OpenCode preset and is not user-selectable.
func SelectableAPITypes() []ModelProviderType {
	return []ModelProviderType{ModelProviderTypeOpenAI, ModelProviderTypeAnthropic}
}

// DefaultBaseURL returns the default base URL for the API type, or an empty
// string when the type has none.
func (t ModelProviderType) DefaultBaseURL() string {
	switch t {
	case ModelProviderTypeOpenAI:
		return "https://api.openai.com/v1"
	case ModelProviderTypeAnthropic:
		return "https://api.anthropic.com/v1"
	case ModelProviderTypeOpenCode:
		return "https://opencode.ai/zen/v1"
	default:
		return ""
	}
}

// DefaultName returns the default display name for the API type, or an empty
// string when the type has none.
func (t ModelProviderType) DefaultName() string {
	switch t {
	case ModelProviderTypeOpenAI:
		return "OpenAI"
	case ModelProviderTypeAnthropic:
		return "Anthropic"
	case ModelProviderTypeOpenCode:
		return "OpenCode"
	default:
		return ""
	}
}

// ProviderPreset names one of the built-in providers offered when adding a
// provider. Every preset except Custom has fixed connection details, so only
// its name and API key need be supplied.
type ProviderPreset string

const (
	// ProviderPresetOpenCode is the OpenCode provider, using the OpenCode
	// API type.
	ProviderPresetOpenCode ProviderPreset = "OpenCode"
	// ProviderPresetOpenRouter is the OpenRouter provider.
	ProviderPresetOpenRouter ProviderPreset = "openrouter"
	// ProviderPresetNvidia is the nVidia (NIM) provider.
	ProviderPresetNvidia ProviderPreset = "nVidia"
	// ProviderPresetHuggingFace is the HuggingFace provider.
	ProviderPresetHuggingFace ProviderPreset = "HuggingFace"
	// ProviderPresetCustom is a user-defined provider: every field is entered
	// by hand.
	ProviderPresetCustom ProviderPreset = "Custom"
)

// ProviderPresets lists the presets in display order.
func ProviderPresets() []ProviderPreset {
	return []ProviderPreset{
		ProviderPresetOpenCode,
		ProviderPresetOpenRouter,
		ProviderPresetNvidia,
		ProviderPresetHuggingFace,
		ProviderPresetCustom,
	}
}

// PresetDetails holds the fixed connection details of a provider preset.
type PresetDetails struct {
	// Name is the default display name of the preset.
	Name string
	// BaseURL is the fixed API base URL of the preset.
	BaseURL string
	// APIType is the fixed API type of the preset.
	APIType ModelProviderType
}

// presetDetails are the fixed connection details of each non-custom preset.
var presetDetails = map[ProviderPreset]PresetDetails{
	ProviderPresetOpenCode:    {Name: "OpenCode", BaseURL: "https://opencode.ai/zen/v1", APIType: ModelProviderTypeOpenCode},
	ProviderPresetOpenRouter:  {Name: "openrouter", BaseURL: "https://openrouter.ai/api/v1", APIType: ModelProviderTypeOpenAI},
	ProviderPresetNvidia:      {Name: "nVidia", BaseURL: "https://integrate.api.nvidia.com/v1", APIType: ModelProviderTypeOpenAI},
	ProviderPresetHuggingFace: {Name: "HuggingFace", BaseURL: "https://router.huggingface.co/v1", APIType: ModelProviderTypeOpenAI},
}

// Details returns the fixed connection details for preset p. Custom has none,
// so ok is false.
func (p ProviderPreset) Details() (PresetDetails, bool) {
	d, ok := presetDetails[p]
	return d, ok
}

// PresetForProvider returns the preset whose fixed details match provider, or
// ProviderPresetCustom when none match.
func PresetForProvider(provider ModelProvider) ProviderPreset {
	for _, p := range ProviderPresets() {
		if d, ok := p.Details(); ok && d.BaseURL == provider.BaseURL && d.APIType == provider.APIType {
			return p
		}
	}
	return ProviderPresetCustom
}

// ModelProvider is one configured model provider. A provider has a single
// base URL, API type and API key, used for both inference and model listing.
type ModelProvider struct {
	// Name is the display name. For presets it defaults to the preset name
	// and remains editable.
	Name string `json:"name"`
	// APIType is the wire API used for inference and model listing.
	APIType ModelProviderType `json:"apiType"`
	// BaseURL is the base URL of the provider's API.
	BaseURL string `json:"baseUrl"`
	// APIKey is the API key (optional for OpenAI-compatible endpoints),
	// stored obfuscated so it is not readable straight out of the config
	// file.
	APIKey encryption.ObfuscatedValue `json:"apiKey"`
	// Models is the list of available model names.
	Models []string `json:"models,omitempty"`
}

// legacyModelProvider is the on-disk shape written by earlier versions. It is
// only read: UnmarshalJSON migrates these fields into the current
// ModelProvider shape so existing config files keep working.
type legacyModelProvider struct {
	Name    string                     `json:"name"`
	APIType ModelProviderType          `json:"apiType"`
	BaseURL string                     `json:"baseUrl"`
	APIKey  encryption.ObfuscatedValue `json:"apiKey"`
	Models  []string                   `json:"models,omitempty"`

	// "execution / listing" shape.
	ExecutionType    ModelProviderType          `json:"executionType"`
	ExecutionBaseURL string                     `json:"executionBaseUrl"`
	ExecutionAPIKey  encryption.ObfuscatedValue `json:"executionApiKey"`
	ListingType      string                     `json:"listingType"`
	ListingBaseURL   string                     `json:"listingBaseUrl"`
	ListingAPIKey    encryption.ObfuscatedValue `json:"listingApiKey"`

	// Oldest shape.
	Type ModelProviderType `json:"type"`
}

// UnmarshalJSON accepts both the current ModelProvider shape and the older
// execution/listing and type/baseUrl/apiKey shapes, migrating the latter into
// the current fields. Unknown API types migrate to OpenAI.
func (p *ModelProvider) UnmarshalJSON(data []byte) error {
	var raw legacyModelProvider
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}

	p.Name = raw.Name
	p.APIType = migrateAPIType(raw.APIType)
	p.BaseURL = raw.BaseURL
	p.APIKey = raw.APIKey
	p.Models = raw.Models

	// Migrate the "execution / listing" shape.
	if p.APIType == "" && raw.ExecutionType != "" {
		p.APIType = migrateAPIType(raw.ExecutionType)
		if raw.ListingType == string(ModelProviderTypeOpenCode) {
			p.APIType = ModelProviderTypeOpenCode
		}
		if p.BaseURL == "" {
			p.BaseURL = raw.ExecutionBaseURL
		}
		if p.BaseURL == "" {
			p.BaseURL = raw.ListingBaseURL
		}
		if p.APIKey.IsEmpty() {
			p.APIKey = raw.ExecutionAPIKey
		}
		if p.APIKey.IsEmpty() {
			p.APIKey = raw.ListingAPIKey
		}
	}
	// Migrate the oldest "type" shape.
	if p.APIType == "" && raw.Type != "" {
		p.APIType = migrateAPIType(raw.Type)
	}
	return nil
}

// migrateAPIType maps any historical or current API type to a current one.
// It returns an empty string only for an empty input, so callers can tell
// "not set" apart from "migrated".
func migrateAPIType(t ModelProviderType) ModelProviderType {
	switch t {
	case "":
		return ""
	case ModelProviderTypeAnthropic:
		return ModelProviderTypeAnthropic
	case ModelProviderTypeOpenCode:
		return ModelProviderTypeOpenCode
	default:
		return ModelProviderTypeOpenAI
	}
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

// NormalizeModelProvider fills in defaults for a provider missing connection
// details: an API type (OpenAI), a name and a base URL derived from the API
// type. Values the caller supplied are never overwritten.
func NormalizeModelProvider(provider ModelProvider) ModelProvider {
	if provider.APIType == "" {
		provider.APIType = ModelProviderTypeOpenAI
	}
	if provider.Name == "" {
		provider.Name = provider.APIType.DefaultName()
	}
	if provider.BaseURL == "" {
		provider.BaseURL = provider.APIType.DefaultBaseURL()
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
