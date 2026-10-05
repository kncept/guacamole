package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
)

func OpenCodeConfig() (*ApiModelInterfaceDetails, error) {
	for _, path := range openCodeConfigLocations() {
		_, err := os.Stat(path)
		if os.IsNotExist(err) {
			continue
		}
		fmt.Printf("examine %v\n", path)
		conf, err := openCodeParseFile(path)
		if conf != nil {
			return conf, nil
		}
	}

	return nil, errors.ErrUnsupported
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

func openCodeParseFile(path string) (*ApiModelInterfaceDetails, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		panic(err)
	}

	var conf json_OpenCode_V1
	if err := json.Unmarshal(data, &conf); err != nil {
		panic(err)
	}

	// Check top-level model field first
	if conf.Model != "" {
		return &ApiModelInterfaceDetails{
			ModelName: conf.Model,
		}, nil
	}

	// Fall back to provider models
	for _, provider := range conf.Providers {
		if provider.Options != nil {
			for modelId, _ := range provider.Models {
				return &ApiModelInterfaceDetails{
					BaseUrl:   provider.Options.BaseUrl,
					ApiKey:    provider.Options.ApiKey,
					ModelName: modelId,
				}, nil
			}
		}

	}
	return nil, nil
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
