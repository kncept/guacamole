package config

import "testing"

// saveProviderWithSelection writes a single-provider config whose SelectedModel
// is set to selection, and returns after saving.
func saveProviderWithSelection(t *testing.T, selection string) {
	t.Helper()

	c, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	c.AddModelProvider(ModelProvider{
		APIType: ModelProviderTypeOpenAI,
		Name:    "OpenAI",
		BaseURL: "https://api.openai.com/v1",
		APIKey:  NewAPIKey("k"),
		Models:  []string{"gpt-4o", "gpt-4o-mini"},
	})
	c.SetSelectedModel(selection)
	if err := c.Save(); err != nil {
		t.Fatalf("Save: %v", err)
	}
}

// TestDefaultConfigUsesSelectedModel checks that an explicitly selected model
// wins over the provider's first model.
func TestDefaultConfigUsesSelectedModel(t *testing.T) {
	withHome(t)
	saveProviderWithSelection(t, "OpenAI / gpt-4o-mini")

	conf, err := DefaultConfig()
	if err != nil {
		t.Fatalf("DefaultConfig: %v", err)
	}
	if conf.ModelName != "gpt-4o-mini" {
		t.Errorf("ModelName = %q, want %q", conf.ModelName, "gpt-4o-mini")
	}
	if conf.ApiKey != "k" {
		t.Errorf("ApiKey = %q, want %q", conf.ApiKey, "k")
	}
	if conf.BaseUrl != "https://api.openai.com/v1" {
		t.Errorf("BaseUrl = %q, want the provider's", conf.BaseUrl)
	}
}

// TestDefaultConfigFallsBackWhenSelectedModelMissing checks that an unset (or
// no longer available) selection is not an error: the provider's first model
// is used instead.
func TestDefaultConfigFallsBackWhenSelectedModelMissing(t *testing.T) {
	for _, tc := range []struct {
		name      string
		selection string
	}{
		{"unset", ""},
		{"stale", "OpenAI / removed-model"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			withHome(t)
			saveProviderWithSelection(t, tc.selection)

			conf, err := DefaultConfig()
			if err != nil {
				t.Fatalf("DefaultConfig: %v", err)
			}
			if conf.ModelName != "gpt-4o" {
				t.Errorf("ModelName = %q, want the first model %q", conf.ModelName, "gpt-4o")
			}
		})
	}
}
