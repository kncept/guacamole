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
	APIType   ModelProviderType
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

	// A model the user explicitly selected wins, provided it still resolves to
	// a configured provider and model. An unset or stale setting simply falls
	// through to the provider defaults below.
	if conf := selectedModelDetails(gcfg); conf != nil {
		fmt.Printf("config: using selected model %s via %s (api key: %s)\n",
			conf.ModelName, conf.BaseUrl, apiKeyStatus(conf.ApiKey))
		return conf, nil
	}

	var fallback *ModelProvider
	for i := range gcfg.ModelProviders {
		p := &gcfg.ModelProviders[i]
		if len(p.Models) > 0 {
			conf := &ApiModelInterfaceDetails{
				APIType:   p.APIType,
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
		APIType: fallback.APIType,
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

// selectedModelDetails resolves the config's SelectedModel label to the
// provider and model it names. It returns nil when the setting is unset or no
// longer matches a configured provider and model, so callers can fall back to
// their own default.
func selectedModelDetails(gcfg *GConfig) *ApiModelInterfaceDetails {
	if gcfg.SelectedModel == "" {
		return nil
	}
	for i := range gcfg.ModelProviders {
		p := &gcfg.ModelProviders[i]
		for _, model := range p.Models {
			option := ModelOption{Provider: p.Name, ModelID: model, ModelName: model}
			if option.Label() != gcfg.SelectedModel {
				continue
			}
			return &ApiModelInterfaceDetails{
				APIType:   p.APIType,
				BaseUrl:   p.BaseURL,
				ApiKey:    p.APIKey.String(),
				ModelName: model,
			}
		}
	}
	return nil
}
