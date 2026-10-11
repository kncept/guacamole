// Package modelclient builds the ai.Provider for a configured model and lists
// the models a provider offers, dispatching on the provider's API type.
package modelclient

import (
	"github.com/kncept/guacamole/ai"
	"github.com/kncept/guacamole/anthropicai"
	"github.com/kncept/guacamole/config"
	"github.com/kncept/guacamole/gemini"
	"github.com/kncept/guacamole/mistralai"
	"github.com/kncept/guacamole/restfulai"
)

// New builds the ai.Provider for conf, dispatching on its API type. OpenAI and
// OpenCode both infer through the OpenAI-compatible client; Anthropic uses the
// Messages API; Mistral uses the Chat Completions API; Gemini uses the
// Generative Language API.
func New(conf *config.ApiModelInterfaceDetails) (ai.Provider, error) {
	switch conf.APIType {
	case config.ModelProviderTypeAnthropic:
		return anthropicai.NewAnthropicAI(conf)
	case config.ModelProviderTypeMistral:
		return mistralai.NewMistralAI(conf)
	case config.ModelProviderTypeGemini:
		return gemini.NewGemini(conf)
	default:
		return restfulai.NewRestfulAI(conf)
	}
}

// ListModels fetches the models a provider offers, dispatching on its API
// type. OpenCode uses its own listing response; Anthropic uses the Anthropic
// Models API; Mistral uses the Mistral Models API; Gemini uses the Models API;
// everything else uses the standard OpenAI /v1/models endpoint.
// appAttribution is only honored by the OpenAI-compatible path (OpenRouter).
func ListModels(apiType config.ModelProviderType, baseURL, apiKey string, appAttribution bool) ([]restfulai.ModelInfo, error) {
	switch apiType {
	case config.ModelProviderTypeOpenCode:
		return restfulai.ListModelsOpenCode(baseURL, apiKey)
	case config.ModelProviderTypeAnthropic:
		return anthropicai.ListModelsAnthropic(baseURL, apiKey)
	case config.ModelProviderTypeMistral:
		return mistralai.ListModelsMistral(baseURL, apiKey)
	case config.ModelProviderTypeGemini:
		return gemini.ListModelsGemini(baseURL, apiKey)
	default:
		return restfulai.ListModelsWithInfo(baseURL, apiKey, appAttribution)
	}
}
