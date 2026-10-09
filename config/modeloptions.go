package config

import "sort"

// ModelOption describes one selectable model and the provider connection
// details needed to call it. It is the unit the GUI's "Provider / Model"
// dropdown is built from.
type ModelOption struct {
	ProviderID   string
	Provider     string
	ModelID      string
	ModelName    string
	BaseURL      string
	APIKey       string
	ProviderType ModelProviderType
}

// Label returns a human-readable identifier for the option. A model that
// belongs to a provider is shown as "provider / model"; a provider offered on
// its own (no specific model selected) is shown by name only.
func (m ModelOption) Label() string {
	if m.ModelID == "" {
		return m.Provider
	}
	return m.Provider + " / " + m.ModelName
}

// AllModelOptionsFromProviders builds the selectable model list from the
// configured ModelProviders in gcfg. A provider that lists models contributes
// one option per model; a provider with no models still contributes a single
// provider-level option so that every configured provider is represented in
// the dropdown.
func AllModelOptionsFromProviders(gcfg *GConfig) []ModelOption {
	var opts []ModelOption
	if gcfg == nil {
		return opts
	}
	for _, p := range gcfg.ModelProviders {
		if len(p.Models) == 0 {
			opts = append(opts, ModelOption{
				ProviderID:   p.Name,
				Provider:     p.Name,
				ModelID:      "",
				ModelName:    p.Name,
				BaseURL:      p.BaseURL,
				APIKey:       p.APIKey,
				ProviderType: p.Type,
			})
			continue
		}
		for _, m := range p.Models {
			opts = append(opts, ModelOption{
				ProviderID:   p.Name,
				Provider:     p.Name,
				ModelID:      m,
				ModelName:    m,
				BaseURL:      p.BaseURL,
				APIKey:       p.APIKey,
				ProviderType: p.Type,
			})
		}
	}
	sort.Slice(opts, func(i, j int) bool { return opts[i].Label() < opts[j].Label() })
	return opts
}
