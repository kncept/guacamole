package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// OpenCodeConfig reads the opencode configuration and resolves the provider
// and model it points at, along with the connection details (base URL, API
// key) needed to call them directly.
func OpenCodeConfig() (*ApiModelInterfaceDetails, error) {
	for _, path := range openCodeConfigLocations() {
		if _, err := os.Stat(path); os.IsNotExist(err) {
			continue
		}
		fmt.Printf("config: examining %s\n", path)
		conf, err := openCodeParseFile(path)
		if conf != nil {
			return conf, nil
		}
		if err != nil {
			return nil, err
		}
	}
	return nil, errors.New("config: no opencode config file found in " +
		strings.Join(openCodeConfigLocations(), ", "))
}

func openCodeConfigLocations() []string {
	locations := make([]string, 0)

	homeDir, err := os.UserHomeDir()
	if err != nil {
		panic(err)
	}
	if homeDir != "" {
		locations = append(locations, filepath.Join(homeDir, ".config", "opencode", "opencode.json"))
		// locations = append(locations, filepath.Join(homeDir, ".config", "opencode", "opencode.jsonc"))
	}
	return locations
}

// openCodeParseFile resolves one opencode config file to connection details.
//
// The top-level "model" field is an opencode "provider/model" reference: its
// connection details live on the provider entry it names, not on the model
// itself, so the two must be joined. Without a top-level model, the first
// provider (in key order) with a base URL and at least one model is used.
func openCodeParseFile(path string) (*ApiModelInterfaceDetails, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("config: read %s: %w", path, err)
	}

	var conf json_OpenCode_V1
	if err := json.Unmarshal(data, &conf); err != nil {
		return nil, fmt.Errorf("config: parse %s: %w", path, err)
	}

	// Check top-level model field first
	if ref := conf.Model; ref != "" {
		providerID, modelID := splitModelRef(ref)
		p, matched := conf.Providers[providerID]
		if !matched {
			// The reference may name a provider key that is not in the
			// config (or no provider at all); find the provider that
			// actually offers the model.
			for _, id := range sortedProviderIDs(conf) {
				if _, ok := conf.Providers[id].Models[modelID]; ok {
					fmt.Printf("config: %s: model %q references provider %q, which is not in the config; using provider %q instead\n",
						path, ref, providerID, id)
					p, matched = conf.Providers[id], true
				}
			}
		}
		if !matched {
			return nil, fmt.Errorf("config: %s: model %q does not match any configured provider", path, ref)
		}
		details, err := providerDetails(p, modelID)
		if err != nil {
			return nil, fmt.Errorf("config: %s: provider for model %q: %w", path, ref, err)
		}
		return details, nil
	}

	// Fall back to provider models: first provider (in key order) that has
	// a base URL and at least one model.
	for _, id := range sortedProviderIDs(conf) {
		if details, err := providerDetails(conf.Providers[id], ""); err == nil {
			return details, nil
		}
	}
	return nil, errors.New("config: no provider with a base URL and at least one model")
}

// providerDetails builds connection details for modelID of provider p. An
// empty modelID selects the provider's first model (in key order).
func providerDetails(p json_OpenCode_V1_provider, modelID string) (*ApiModelInterfaceDetails, error) {
	if modelID == "" {
		modelID = firstModelID(p.Models)
	}
	if modelID == "" {
		return nil, errors.New("has no models")
	}
	if p.Options == nil || p.Options.BaseUrl == "" {
		return nil, errors.New("has no options.baseURL")
	}
	return &ApiModelInterfaceDetails{
		BaseUrl:   p.Options.BaseUrl,
		ApiKey:    p.Options.ApiKey,
		ModelName: modelID,
	}, nil
}

// splitModelRef splits an opencode model reference "provider/model" into its
// parts. A reference without a slash is a bare model name.
func splitModelRef(ref string) (providerID, modelID string) {
	if i := strings.Index(ref, "/"); i >= 0 {
		return ref[:i], ref[i+1:]
	}
	return "", ref
}

// sortedProviderIDs lists the provider keys in stable sorted order.
func sortedProviderIDs(conf json_OpenCode_V1) []string {
	ids := make([]string, 0, len(conf.Providers))
	for id := range conf.Providers {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids
}

// firstModelID returns the lexicographically first key of models.
func firstModelID(models map[string]json_OpenCode_V1_model) string {
	if len(models) == 0 {
		return ""
	}
	ids := make([]string, 0, len(models))
	for id := range models {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids[0]
}

type json_OpenCode_options struct {
	BaseUrl string `json:"baseURL"`
	ApiKey  string `json:"apiKey"`
}
type json_OpenCode_V1 struct {
	Model    string                               `json:"model"`
	Schema   string                               `json:"$schema"`
	Providers map[string]json_OpenCode_V1_provider `json:"provider"`
}
type json_OpenCode_V1_provider struct {
	Name    string
	Options *json_OpenCode_options            `json:"options,omitempty"`
	Models  map[string]json_OpenCode_V1_model `json:"models"`
}
type json_OpenCode_V1_model struct {
	DisplayName string `json:"name"`
}

// ModelOption describes one model usable through the opencode config,
// including the provider connection details needed to call it.
type ModelOption struct {
	ProviderID  string
	Provider    string
	ModelID     string
	ModelName   string
	BaseURL     string
	APIKey      string
}

// Label returns a human-readable "provider / model" identifier.
func (m ModelOption) Label() string {
	return m.Provider + " / " + m.ModelName
}

// AllModelOptions lists every model of every configured provider.
func AllModelOptions() ([]ModelOption, error) {
	for _, path := range openCodeConfigLocations() {
		if _, err := os.Stat(path); os.IsNotExist(err) {
			continue
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, err
		}
		var conf json_OpenCode_V1
		if err := json.Unmarshal(data, &conf); err != nil {
			return nil, err
		}
		var opts []ModelOption
		for pid, p := range conf.Providers {
			var baseURL, apiKey string
			if p.Options != nil {
				baseURL = p.Options.BaseUrl
				apiKey = p.Options.ApiKey
			}
			for mid, m := range p.Models {
				name := m.DisplayName
				if name == "" {
					name = mid
				}
				opts = append(opts, ModelOption{
					ProviderID: pid,
					Provider:   p.Name,
					ModelID:    mid,
					ModelName:  name,
					BaseURL:    baseURL,
					APIKey:     apiKey,
				})
			}
		}
		sort.Slice(opts, func(i, j int) bool { return opts[i].Label() < opts[j].Label() })
		return opts, nil
	}
	return nil, errors.ErrUnsupported
}
