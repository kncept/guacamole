package config

import "testing"

// TestAllModelOptionsFromProvidersWithModels checks that a provider listing
// models contributes one dropdown option per model, carrying the provider's
// connection details.
func TestAllModelOptionsFromProvidersWithModels(t *testing.T) {
	gcfg := &GConfig{
		ModelProviders: []ModelProvider{
			{
				Name:    "OpenAI",
				Type:    ModelProviderTypeOpenAI,
				BaseURL: "https://api.openai.com/v1",
				APIKey:  "k",
				Models:  []string{"gpt-4o", "gpt-4o-mini"},
			},
		},
	}

	got := AllModelOptionsFromProviders(gcfg)
	if len(got) != 2 {
		t.Fatalf("len = %d, want 2 (one per model)", len(got))
	}
	// Sorted by label: "OpenAI / gpt-4o" < "OpenAI / gpt-4o-mini".
	if got[0].Label() != "OpenAI / gpt-4o" {
		t.Errorf("first label = %q, want %q", got[0].Label(), "OpenAI / gpt-4o")
	}
	if got[0].BaseURL != "https://api.openai.com/v1" || got[0].APIKey != "k" {
		t.Errorf("connection details = %q / %q, want the provider's", got[0].BaseURL, got[0].APIKey)
	}
	if got[0].ModelID != "gpt-4o" {
		t.Errorf("ModelID = %q, want %q", got[0].ModelID, "gpt-4o")
	}
}

// TestAllModelOptionsFromProviderWithoutModels checks that a provider with no
// models still appears in the dropdown as a single provider-level option
// (empty model ID, so the provider's own default applies).
func TestAllModelOptionsFromProviderWithoutModels(t *testing.T) {
	gcfg := &GConfig{
		ModelProviders: []ModelProvider{
			{Name: "nVidia", Type: ModelProviderTypeNvidia, BaseURL: "https://integrate.api.nvidia.com/v1", APIKey: "nv"},
		},
	}

	got := AllModelOptionsFromProviders(gcfg)
	if len(got) != 1 {
		t.Fatalf("len = %d, want 1 (the provider itself)", len(got))
	}
	if got[0].Label() != "nVidia" {
		t.Errorf("label = %q, want %q", got[0].Label(), "nVidia")
	}
	if got[0].ModelID != "" {
		t.Errorf("ModelID = %q, want empty for a provider-level option", got[0].ModelID)
	}
	if got[0].ProviderType != ModelProviderTypeNvidia {
		t.Errorf("ProviderType = %q, want %q", got[0].ProviderType, ModelProviderTypeNvidia)
	}
}

// TestAllModelOptionsFromProvidersEmpty checks the empty and nil cases.
func TestAllModelOptionsFromProvidersEmpty(t *testing.T) {
	if got := AllModelOptionsFromProviders(nil); len(got) != 0 {
		t.Errorf("nil config: len = %d, want 0", len(got))
	}
	if got := AllModelOptionsFromProviders(&GConfig{}); len(got) != 0 {
		t.Errorf("no providers: len = %d, want 0", len(got))
	}
}
