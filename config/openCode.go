package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
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

	// fmt.Printf("%+v\n", conf)
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
	Schema    string                               `json:"$schema"`
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
