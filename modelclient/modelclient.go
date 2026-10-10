// Package modelclient builds the ai.Provider for a configured model and lists
// the models a provider offers, dispatching on the provider's API type.
package modelclient

import (
	"github.com/kncept/guacamole/ai"
	"github.com/kncept/guacamole/anthropicai"
	"github.com/kncept/guacamole/config"
	"github.com/kncept/guacamole/restfulai"
)

// New builds the ai.Provider for conf, dispatching on its API type. OpenAI and
// OpenCode both infer through the OpenAI-compatible client; Anthropic uses the
// Messages API.
func New(conf *config.ApiModelInterfaceDetails) (ai.Provider, error) {
	switch conf.APIType {
	case config.ModelProviderTypeAnthropic:
		return anthropicai.NewAnthropicAI(conf)
	default:
		return restfulai.NewRestfulAI(conf)
	}
}

// ListModels fetches the models a provider offers, dispatching on its API
// type. OpenCode uses its own listing response; Anthropic uses the Anthropic
// Models API; everything else uses the standard OpenAI /v1/models endpoint.
func ListModels(apiType config.ModelProviderType, baseURL, apiKey string) ([]restfulai.ModelInfo, error) {
	switch apiType {
	case config.ModelProviderTypeOpenCode:
		return restfulai.ListModelsOpenCode(baseURL, apiKey)
	case config.ModelProviderTypeAnthropic:
		return anthropicai.ListModelsAnthropic(baseURL, apiKey)
	default:
		return restfulai.ListModelsWithInfo(baseURL, apiKey)
	}
}
