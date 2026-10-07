package config

import (
	"fmt"
	"os"
	"path/filepath"
)

// GuacDir returns guacamole's state directory: ~/.guac. Sessions live
// under it.
func GuacDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".guac"), nil
}

type ApiModelInterfaceDetails struct {
	BaseUrl   string
	ApiKey    string
	ModelName string
}

// DefaultConfig resolves the AI backend guacamole should use. It currently
// reads the opencode configuration; the result carries the base URL, API
// key, and model name needed to call it directly.
func DefaultConfig() (*ApiModelInterfaceDetails, error) {
	conf, err := OpenCodeConfig()
	if err != nil {
		return nil, err
	}
	fmt.Printf("config: using model %s via %s (api key: %s)\n",
		conf.ModelName, conf.BaseUrl, apiKeyStatus(conf.ApiKey))
	return conf, nil
}

// apiKeyStatus says whether a key is present without printing it.
func apiKeyStatus(key string) string {
	if key == "" {
		return "unset"
	}
	return "set"
}
