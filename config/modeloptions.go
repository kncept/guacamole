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
	Size         string // e.g., "7B", "70B"
	IsFree       bool   // whether the model is free
	Description  string
	// AppAttribution carries the provider's opt-in app attribution setting.
	AppAttribution bool
}

// Label returns a human-readable identifier for the option: the model shown
// as "provider / model".
func (m ModelOption) Label() string {
	if m.IsMagi() {
		return MagiModel
	}
	if m.ModelID == "" {
		return m.Provider
	}
	return m.Provider + " / " + m.ModelName
}

// IsMagi reports whether the option is the virtual Magi model rather than a
// real provider model.
func (m ModelOption) IsMagi() bool {
	return m.ModelID == MagiModel
}

// MagiModelOption returns the virtual "Magi" model. It is not a real model:
// selecting it switches a session to Magi Mode. It leads the list of
// available models (see AllSelectableModelOptions).
func MagiModelOption() ModelOption {
	return ModelOption{
		Provider:  MagiModel,
		ModelID:   MagiModel,
		ModelName: MagiModel,
	}
}

// AllSelectableModelOptions builds the full list of models a session may
// choose from: the virtual "Magi" model first, then every configured
// provider model.
func AllSelectableModelOptions(gcfg *GConfig) []ModelOption {
	return append([]ModelOption{MagiModelOption()}, AllModelOptionsFromProviders(gcfg)...)
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
				ProviderID:     p.Name,
				Provider:       p.Name,
				ModelID:        m,
				ModelName:      m,
				BaseURL:        p.BaseURL,
				APIKey:         p.APIKey.String(),
				ProviderType:   p.APIType,
				AppAttribution: p.AppAttribution,
			})
		}
	}
	sort.Slice(opts, func(i, j int) bool { return opts[i].Label() < opts[j].Label() })
	return opts
}
