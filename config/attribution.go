package config

// App attribution identifies Guacamole to OpenRouter so requests made through
// a provider show up under the app in OpenRouter's dashboards and
// leaderboards. See https://openrouter.ai/docs/app-attribution.
const (
	// AppAttributionReferer is the HTTP-Referer header value.
	AppAttributionReferer = "https://guacamole.kncept.com"
	// AppAttributionTitle is the X-OpenRouter-Title header value.
	AppAttributionTitle = "Guacamole"
	// AppAttributionCategories is the X-OpenRouter-Categories header value.
	AppAttributionCategories = "cli-agent,programming-app"
)

// AppAttributionHeaders returns the app attribution HTTP headers to send with
// a request, or nil when attribution is disabled.
func AppAttributionHeaders(enabled bool) map[string]string {
	if !enabled {
		return nil
	}
	return map[string]string{
		"HTTP-Referer":            AppAttributionReferer,
		"X-OpenRouter-Title":      AppAttributionTitle,
		"X-OpenRouter-Categories": AppAttributionCategories,
	}
}
