package config

import (
	"errors"
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

// DefaultConfig resolves the AI backend guacamole should use from the
// configured model providers in the guac config. A provider that lists models
// is preferred (its first model is used); otherwise the first configured
// provider is used with an empty model so the provider's own default applies.
func DefaultConfig() (*ApiModelInterfaceDetails, error) {
	gcfg, err := Load()
	if err != nil {
		return nil, err
	}
	if gcfg == nil || len(gcfg.ModelProviders) == 0 {
		return nil, errors.New("config: no model providers configured; add one via the GUI (Preferences → Providers)")
	}

	var fallback *ModelProvider
	for i := range gcfg.ModelProviders {
		p := &gcfg.ModelProviders[i]
		if len(p.Models) > 0 {
			conf := &ApiModelInterfaceDetails{
				BaseUrl:   p.BaseURL,
				ApiKey:    p.APIKey.String(),
				ModelName: p.Models[0],
			}
			fmt.Printf("config: using model %s via %s (api key: %s)\n",
				conf.ModelName, conf.BaseUrl, apiKeyStatus(conf.ApiKey))
			return conf, nil
		}
		if fallback == nil {
			fallback = p
		}
	}

	conf := &ApiModelInterfaceDetails{
		BaseUrl: fallback.BaseURL,
		ApiKey:  fallback.APIKey.String(),
	}
	fmt.Printf("config: using provider %s via %s (api key: %s)\n",
		fallback.Name, conf.BaseUrl, apiKeyStatus(conf.ApiKey))
	return conf, nil
}

// apiKeyStatus says whether a key is present without printing it.
func apiKeyStatus(key string) string {
	if key == "" {
		return "unset"
	}
	return "set"
}
