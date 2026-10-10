package config

import "sort"

// ModelOption describes one selectable model and the provider connection
// details needed to call it. It is the unit the GUI's "Provider / Model"
// dropdown is built from.
type ModelOption struct {
	ProviderID        string
	Provider          string
	ModelID           string
	ModelName         string
	ExecutionBaseURL  string
	ExecutionAPIKey   string
	ListingBaseURL    string
	ListingAPIKey     string
	ProviderType      ModelProviderType
	ListingType       ModelListingType
	Size              string // e.g., "7B", "70B"
	IsFree            bool   // whether the model is free
	Description       string
}

// Label returns a human-readable identifier for the option: the model shown
// as "provider / model".
func (m ModelOption) Label() string {
	if m.ModelID == "" {
		return m.Provider
	}
	return m.Provider + " / " + m.ModelName
}

// AllModelOptionsFromProviders builds the selectable model list from the
// configured ModelProviders in gcfg. The dropdown is driven purely by the
// available models: a provider contributes one option per listed model, and
// a provider with no models contributes nothing.
func AllModelOptionsFromProviders(gcfg *GConfig) []ModelOption {
	var opts []ModelOption
	if gcfg == nil {
		return opts
	}
	for _, p := range gcfg.ModelProviders {
		for _, m := range p.Models {
			opts = append(opts, ModelOption{
				ProviderID:       p.Name,
				Provider:         p.Name,
				ModelID:          m,
				ModelName:        m,
				ExecutionBaseURL: p.ExecutionBaseURL,
				ExecutionAPIKey:  p.ExecutionAPIKey.String(),
				ListingBaseURL:   p.ListingBaseURL,
				ListingAPIKey:    p.ListingAPIKey.String(),
				ProviderType:     p.ExecutionType,
				ListingType:      p.ListingType,
			})
		}
	}
	sort.Slice(opts, func(i, j int) bool { return opts[i].Label() < opts[j].Label() })
	return opts
}
