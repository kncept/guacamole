// Package restfulai implements ai.Provider for OpenAI-compatible REST
// APIs, using the openai-go client's Responses API.
package restfulai

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/kncept/guacamole/ai"
	"github.com/kncept/guacamole/config"
	"github.com/openai/openai-go/v3"
	openaiOption "github.com/openai/openai-go/v3/option"
	"github.com/openai/openai-go/v3/responses"
)

// RestfulAI is an ai.Provider backed by an OpenAI-compatible REST API.
type RestfulAI interface {
	ai.Provider
}

type restfulAI struct {
	client         *openai.Client
	baseURL        string
	model          string
	appAttribution bool
}

// NewRestfulAI builds a Provider from API connection details. It fails fast
// on missing connection details: a client with an empty base URL would POST
// to a bare path and every request would die with "unsupported protocol
// scheme", far from the real cause.
func NewRestfulAI(conf *config.ApiModelInterfaceDetails) (RestfulAI, error) {
	if conf.BaseUrl == "" {
		return nil, fmt.Errorf("restfulai: no base URL for model %q; set base URL in provider configuration", conf.ModelName)
	}

	opts := []openaiOption.RequestOption{
		openaiOption.WithBaseURL(conf.BaseUrl),
		openaiOption.WithAPIKey(conf.ApiKey), // defaults to os.LookupEnv("OPENAI_API_KEY")
	}
	opts = append(opts, attributionOptions(conf.AppAttribution)...)
	client := openai.NewClient(opts...)
	this := &restfulAI{
		client:         &client,
		baseURL:        conf.BaseUrl,
		model:          conf.ModelName,
		appAttribution: conf.AppAttribution,
	}
	log.Printf("restfulai: ready: base URL %s, model %s, api key %s, app attribution %v",
		conf.BaseUrl, conf.ModelName, apiKeyStatus(conf.ApiKey), conf.AppAttribution)
	return this, nil
}

// attributionOptions returns the request options that add app attribution
// headers when enabled, and nothing when disabled.
func attributionOptions(enabled bool) []openaiOption.RequestOption {
	headers := config.AppAttributionHeaders(enabled)
	if len(headers) == 0 {
		return nil
	}
	opts := make([]openaiOption.RequestOption, 0, len(headers))
	for k, v := range headers {
		opts = append(opts, openaiOption.WithHeader(k, v))
	}
	return opts
}

// logRequest records what is about to be sent, so a failure can be traced
// back to the exact endpoint and request it came from.
func (this *restfulAI) logRequest(kind string, req ai.Request) {
	model := req.Model
	if model == "" {
		model = this.model
	}
	log.Printf("restfulai: %s: POST %s/responses model=%s messages=%d tools=%d",
		kind, this.baseURL, model, len(req.Conversation()), len(req.Tools))
}

// apiKeyStatus says whether a key is present without printing it.
func apiKeyStatus(key string) string {
	if key == "" {
		return "unset (falls back to $OPENAI_API_KEY)"
	}
	return "set"
}

// wrapErr attaches the request context to a provider error and marks HTTP
// 429 responses as a rate limit (with the provider's recommended wait, when
// it sent one) so the session can offer the user a retry decision. kind
// names the request stage, e.g. "" for a plain send and " (stream)" for a
// stream, matching the existing error messages.
func (this *restfulAI) wrapErr(kind string, err error) error {
	wrapped := fmt.Errorf("restfulai: POST %s/responses%s: %w", this.baseURL, kind, err)
	var apiErr *openai.Error
	if errors.As(err, &apiErr) && apiErr.StatusCode == http.StatusTooManyRequests {
		var retry time.Duration
		if apiErr.Response != nil {
			retry = retryAfter(apiErr.Response.Header)
		}
		return &ai.RateLimitError{Err: wrapped, RetryAfter: retry}
	}
	return wrapped
}

// retryAfter parses the Retry-After header: an integer number of seconds or
// an HTTP-date. Returns 0 when absent or unparseable.
func retryAfter(h http.Header) time.Duration {
	if v := strings.TrimSpace(h.Get("Retry-After")); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n >= 0 {
			return time.Duration(n) * time.Second
		}
		if t, err := http.ParseTime(v); err == nil {
			if d := time.Until(t); d > 0 {
				return d
			}
		}
	}
	return 0
}

// Send performs one non-streaming exchange.
func (this *restfulAI) Send(ctx context.Context, req ai.Request) (ai.Response, error) {
	this.logRequest("send", req)
	raw, err := this.client.Responses.New(ctx, this.params(req))
	if err != nil {
		return ai.Response{}, this.wrapErr("", err)
	}
	log.Printf("restfulai: response ok: %d input, %d output tokens",
		raw.Usage.InputTokens, raw.Usage.OutputTokens)
	return toResponse(raw), nil
}

// SendStream performs a streaming exchange, calling onChunk for every text
// delta as it arrives. A non-nil return from onChunk aborts the stream.
func (this *restfulAI) SendStream(ctx context.Context, req ai.Request, onChunk func(ai.Chunk) error) (ai.Response, error) {
	this.logRequest("stream", req)
	stream := this.client.Responses.NewStreaming(ctx, this.params(req))

	var (
		text     string
		final    *responses.Response
		finished bool
	)
	for stream.Next() {
		event := stream.Current()
		switch event.Type {
		case "response.output_text.delta":
			if onChunk != nil {
				if err := onChunk(ai.Chunk{Delta: event.Delta}); err != nil {
					stream.Close()
					return ai.Response{}, err
				}
			}
			text += event.Delta
		case "response.completed":
			final = &event.Response
			finished = true
		case "response.incomplete":
			// Cut short (e.g. token limit); the partial response is still useful.
			final = &event.Response
			finished = false
		}
	}
	if err := stream.Err(); err != nil {
		return ai.Response{}, this.wrapErr(" (stream)", err)
	}

	log.Printf("restfulai: stream done: %d chars, complete=%v", len(text), finished)

	resp := ai.Response{
		Text:     text,
		Finished: finished,
	}
	if final != nil {
		resp.Messages = outputMessages(final)
		resp.Usage = ai.Usage{
			InputTokens:  int(final.Usage.InputTokens),
			OutputTokens: int(final.Usage.OutputTokens),
		}
		resp.Raw = final
		log.Printf("restfulai: usage: %d input, %d output tokens",
			final.Usage.InputTokens, final.Usage.OutputTokens)
	}
	return resp, nil
}

// ListModels fetches available model IDs from an OpenAI-compatible endpoint
// at baseURL using apiKey for authentication (falls back to
// $OPENAI_API_KEY when empty). When appAttribution is set, app attribution
// headers are sent with the request.
func ListModels(baseURL, apiKey string, appAttribution bool) ([]string, error) {
	if baseURL == "" {
		return nil, fmt.Errorf("restfulai: no base URL to list models")
	}

	opts := []openaiOption.RequestOption{
		openaiOption.WithBaseURL(baseURL),
	}
	if apiKey != "" {
		opts = append(opts, openaiOption.WithAPIKey(apiKey))
	}
	opts = append(opts, attributionOptions(appAttribution)...)

	client := openai.NewClient(opts...)
	var models []string
	pager := client.Models.ListAutoPaging(context.Background())
	for pager.Next() {
		m := pager.Current()
		if m.ID != "" {
			models = append(models, m.ID)
		}
	}
	if err := pager.Err(); err != nil {
		return nil, fmt.Errorf("restfulai: list models from %s: %w", baseURL, err)
	}
	return models, nil
}

// ModelInfo represents a model with additional metadata.
type ModelInfo struct {
	ID          string
	Size        string // e.g., "7B", "70B"
	IsFree      bool   // whether the model is free
	Description string
}

// ListModelsWithInfo fetches available models with additional metadata from
// an OpenAI-compatible endpoint at baseURL using apiKey for authentication.
// This is a generic implementation that only returns model IDs; specific
// providers may override this with richer data. When appAttribution is set,
// app attribution headers are sent with the request.
func ListModelsWithInfo(baseURL, apiKey string, appAttribution bool) ([]ModelInfo, error) {
	ids, err := ListModels(baseURL, apiKey, appAttribution)
	if err != nil {
		return nil, err
	}
	var infos []ModelInfo
	for _, id := range ids {
		infos = append(infos, ModelInfo{ID: id})
	}
	return infos, nil
}

// ListModelsOpenCode fetches models from the OpenCode API. baseURL is the
// OpenCode base URL (for example https://opencode.ai/zen/v1); the listing
// endpoint is baseURL/models.
//
// Despite being a provider-specific path, the endpoint returns the standard
// OpenAI listing shape:
//
//	{"object":"list","data":[{"id":"...","object":"model","owned_by":"opencode"}]}
//
// Free models are named with a "-free" suffix. The legacy {"models":[...]}
// shape is still accepted for older deployments.
func ListModelsOpenCode(baseURL, apiKey string) ([]ModelInfo, error) {
	if baseURL == "" {
		baseURL = "https://opencode.ai/zen/v1"
	}
	// The OpenCode models endpoint
	url := strings.TrimSuffix(baseURL, "/") + "/models"

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("opencode: create request: %w", err)
	}
	if apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+apiKey)
	}

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("opencode: fetch models: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("opencode: fetch models: status %d: %s", resp.StatusCode, string(body))
	}

	var result struct {
		// Standard OpenAI listing shape.
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
		// Legacy OpenCode listing shape.
		Models []struct {
			ID          string `json:"id"`
			Size        string `json:"size"`
			Description string `json:"description"`
			Free        bool   `json:"free"`
		} `json:"models"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("opencode: decode response: %w", err)
	}

	var infos []ModelInfo
	if len(result.Data) > 0 {
		for _, m := range result.Data {
			if m.ID == "" {
				continue
			}
			infos = append(infos, ModelInfo{
				ID:     m.ID,
				IsFree: strings.HasSuffix(m.ID, "-free"),
			})
		}
		return infos, nil
	}

	for _, m := range result.Models {
		infos = append(infos, ModelInfo{
			ID:          m.ID,
			Size:        m.Size,
			IsFree:      m.Free,
			Description: m.Description,
		})
	}
	return infos, nil
}

// params translates a normalized ai.Request into openai-go request params.
func (this *restfulAI) params(req ai.Request) responses.ResponseNewParams {
	model := req.Model
	if model == "" {
		model = this.model
	}

	messages := req.Conversation()
	p := responses.ResponseNewParams{
		Model: model,
	}

	// A lone user message goes on the wire as a plain string; anything
	// richer (multi-turn, tool messages) goes as an item list.
	if len(messages) == 1 && messages[0].Role == ai.RoleUser {
		p.Input = responses.ResponseNewParamsInputUnion{
			OfString: openai.String(messages[0].Content),
		}
	} else {
		items := make(responses.ResponseInputParam, 0, len(messages))
		for _, m := range messages {
			items = append(items, inputItem(m))
		}
		p.Input = responses.ResponseNewParamsInputUnion{
			OfInputItemList: items,
		}
	}

	if len(req.Tools) > 0 {
		tools := make([]responses.ToolUnionParam, 0, len(req.Tools))
		for _, t := range req.Tools {
			tool := responses.ToolParamOfFunction(t.Name, t.Parameters, false)
			tool.OfFunction.Description = openai.String(t.Description)
			tools = append(tools, tool)
		}
		p.Tools = tools
	}

	if req.SystemPrompt != "" {
		p.Instructions = openai.String(req.SystemPrompt)
	}
	if req.Temperature != nil {
		p.Temperature = openai.Float(*req.Temperature)
	}
	if req.MaxTokens != nil {
		p.MaxOutputTokens = openai.Int(int64(*req.MaxTokens))
	}
	return p
}

// inputItem translates one history message into a Responses API input
// item: a plain message, a function call, or a function call output.
func inputItem(m ai.Message) responses.ResponseInputItemUnionParam {
	switch {
	case m.Role == ai.RoleAssistant && m.Name != "":
		return responses.ResponseInputItemParamOfFunctionCall(m.ToolArguments, m.ToolCallID, m.Name)
	case m.Role == ai.RoleTool:
		item := responses.ResponseInputItemParamOfFunctionCallOutput(m.Content)
		item.OfFunctionCallOutput.CallID = openai.String(m.ToolCallID)
		return item
	default:
		return responses.ResponseInputItemUnionParam{
			OfMessage: &responses.EasyInputMessageParam{
				Content: responses.EasyInputMessageContentUnionParam{
					OfString: openai.String(m.Content),
				},
				Role: responses.EasyInputMessageRole(m.Role),
				Type: responses.EasyInputMessageType("message"),
			},
		}
	}
}

// outputMessages extracts structured output items beyond the plain text —
// currently function calls — from a raw response.
func outputMessages(raw *responses.Response) []ai.Message {
	var messages []ai.Message
	for _, item := range raw.Output {
		if item.Type == "function_call" {
			messages = append(messages, ai.Message{
				Role:          ai.RoleAssistant,
				Name:          item.Name,
				ToolCallID:    item.CallID,
				ToolArguments: item.Arguments.OfString,
			})
		}
	}
	return messages
}

// toResponse normalizes a raw API response into an ai.Response.
func toResponse(raw *responses.Response) ai.Response {
	return ai.Response{
		Text:     raw.OutputText(),
		Messages: outputMessages(raw),
		Finished: true,
		Usage: ai.Usage{
			InputTokens:  int(raw.Usage.InputTokens),
			OutputTokens: int(raw.Usage.OutputTokens),
		},
		Raw: raw,
	}
}
