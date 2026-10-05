package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// writeConfig writes raw to a temp opencode config file and returns its path.
func writeConfig(t *testing.T, raw string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "opencode.json")
	if err := os.WriteFile(path, []byte(raw), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestParseFileTopLevelModel(t *testing.T) {
	path := writeConfig(t, `{
		"model": "local/qwen3.8-27b",
		"provider": {
			"local": {
				"options": {"baseURL": "http://localhost:8080/v1", "apiKey": "lockdown"},
				"models": {"qwen3.8-27b": {"name": "Qwen 3.8"}}
			}
		}
	}`)

	conf, err := openCodeParseFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if conf.BaseUrl != "http://localhost:8080/v1" {
		t.Errorf("BaseUrl = %q, want %q", conf.BaseUrl, "http://localhost:8080/v1")
	}
	if conf.ApiKey != "lockdown" {
		t.Errorf("ApiKey = %q, want %q", conf.ApiKey, "lockdown")
	}
	if conf.ModelName != "qwen3.8-27b" {
		t.Errorf("ModelName = %q, want %q", conf.ModelName, "qwen3.8-27b")
	}
}

func TestParseFileTopLevelModelProviderMismatch(t *testing.T) {
	// The model reference names a provider key that is not in the config;
	// the provider that actually offers the model must be picked up.
	path := writeConfig(t, `{
		"model": "localai/qwen3.8-27b",
		"provider": {
			"local": {
				"options": {"baseURL": "http://localhost:8080/v1", "apiKey": "lockdown"},
				"models": {"qwen3.8-27b": {"name": "Qwen 3.8"}}
			}
		}
	}`)

	conf, err := openCodeParseFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if conf.BaseUrl != "http://localhost:8080/v1" {
		t.Errorf("BaseUrl = %q, want %q", conf.BaseUrl, "http://localhost:8080/v1")
	}
	if conf.ModelName != "qwen3.8-27b" {
		t.Errorf("ModelName = %q, want %q", conf.ModelName, "qwen3.8-27b")
	}
}

func TestParseFileFallbackProviderModels(t *testing.T) {
	// No top-level model: the first provider (in key order) with a base
	// URL wins; a provider without one is skipped.
	path := writeConfig(t, `{
		"provider": {
			"alpha": {
				"models": {"a1": {}}
			},
			"beta": {
				"options": {"baseURL": "http://beta.local/v1", "apiKey": "k"},
				"models": {"b1": {"name": "B One"}, "b0": {"name": "B Zero"}}
			}
		}
	}`)

	conf, err := openCodeParseFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if conf.BaseUrl != "http://beta.local/v1" {
		t.Errorf("BaseUrl = %q, want %q", conf.BaseUrl, "http://beta.local/v1")
	}
	if conf.ModelName != "b0" {
		t.Errorf("ModelName = %q, want first model key %q", conf.ModelName, "b0")
	}
}

func TestParseFileProviderWithoutBaseUrl(t *testing.T) {
	path := writeConfig(t, `{
		"model": "local/qwen3.8-27b",
		"provider": {
			"local": {
				"models": {"qwen3.8-27b": {}}
			}
		}
	}`)

	_, err := openCodeParseFile(path)
	if err == nil {
		t.Fatal("expected an error for a provider without options.baseURL")
	}
	if !strings.Contains(err.Error(), "baseURL") {
		t.Errorf("error = %v, want it to mention baseURL", err)
	}
}

func TestParseFileUnknownModel(t *testing.T) {
	path := writeConfig(t, `{
		"model": "nosuch/m1",
		"provider": {
			"local": {
				"options": {"baseURL": "http://localhost:8080/v1"},
				"models": {"m2": {}}
			}
		}
	}`)

	if _, err := openCodeParseFile(path); err == nil {
		t.Fatal("expected an error for a model that matches no provider")
	}
}
