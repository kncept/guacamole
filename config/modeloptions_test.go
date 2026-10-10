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
				APIType: ModelProviderTypeOpenAI,
				BaseURL: "https://api.openai.com/v1",
				APIKey:  NewAPIKey("k"),
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
// models contributes nothing to the dropdown: the list is driven purely by
// the available models.
func TestAllModelOptionsFromProviderWithoutModels(t *testing.T) {
	gcfg := &GConfig{
		ModelProviders: []ModelProvider{
			{Name: "nVidia", APIType: ModelProviderTypeOpenAI, BaseURL: "https://integrate.api.nvidia.com/v1", APIKey: NewAPIKey("nv")},
		},
	}

	got := AllModelOptionsFromProviders(gcfg)
	if len(got) != 0 {
		t.Fatalf("len = %d, want 0 (no models, nothing to show)", len(got))
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
