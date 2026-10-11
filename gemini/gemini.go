// Package gemini implements ai.Provider for the Google Gemini (Generative
// Language) API, including streaming and function calling.
//
// The Gemini wire API is not OpenAI-compatible: inference goes to
// models/{model}:generateContent (and :streamGenerateContent for streaming),
// messages are carried as "contents" of role user/model with "parts", the
// system prompt is a separate system_instruction, and function calls travel in
// part.functionCalls / part.functionResponse.
package gemini

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/kncept/guacamole/ai"
	"github.com/kncept/guacamole/config"
	"github.com/kncept/guacamole/restfulai"
)

// Gemini is an ai.Provider backed by the Google Gemini (Generative Language)
// API.
type Gemini interface {
	ai.Provider
}

type geminiProvider struct {
	client  *http.Client
	baseURL string
	apiKey  string
	model   string
}

// NewGemini builds a Provider from API connection details. It fails fast on a
// missing base URL, the same way the other providers do.
func NewGemini(conf *config.ApiModelInterfaceDetails) (Gemini, error) {
	if conf.BaseUrl == "" {
		return nil, fmt.Errorf("gemini: no base URL for model %q; set base URL in provider configuration", conf.ModelName)
	}
	return &geminiProvider{
		client:  &http.Client{Timeout: 10 * time.Minute},
		baseURL: strings.TrimSuffix(conf.BaseUrl, "/"),
		apiKey:  conf.ApiKey,
		model:   conf.ModelName,
	}, nil
}

// modelName returns the model for req, falling back to the provider default.
func (g *geminiProvider) modelName(req ai.Request) string {
	if req.Model != "" {
		return req.Model
	}
	return g.model
}

// modelPath returns the Gemini model resource name (models/{model}), adding
// the models/ prefix if the caller did not supply it.
func (g *geminiProvider) modelPath(req ai.Request) string {
	m := strings.TrimPrefix(g.modelName(req), "models/")
	return "models/" + m
}

// generateURL builds the generate/streamGenerate endpoint for the request.
func (g *geminiProvider) generateURL(req ai.Request, stream bool) string {
	if stream {
		return fmt.Sprintf("%s/%s:streamGenerateContent?alt=sse", g.baseURL, g.modelPath(req))
	}
	return fmt.Sprintf("%s/%s:generateContent", g.baseURL, g.modelPath(req))
}

// setHeaders applies the Gemini authentication and content-type headers.
func (g *geminiProvider) setHeaders(req *http.Request) {
	if g.apiKey != "" {
		req.Header.Set("x-goog-api-key", g.apiKey)
	}
	req.Header.Set("Content-Type", "application/json")
}

// genContentRequest is the generateContent request body.
type genContentRequest struct {
	Contents          []apiContent   `json:"contents"`
	SystemInstruction *apiContent    `json:"system_instruction,omitempty"`
	Tools             []apiTool      `json:"tools,omitempty"`
	ToolConfig        *apiToolConfig `json:"tool_config,omitempty"`
	GenerationConfig  *genConfig     `json:"generation_config,omitempty"`
}

// apiContent is one turn of the conversation.
type apiContent struct {
	Role  string    `json:"role,omitempty"`
	Parts []apiPart `json:"parts"`
}

// apiPart is one piece of a turn: text, a function call, or a function result.
type apiPart struct {
	Text             string        `json:"text,omitempty"`
	FunctionCalls    []funcCall    `json:"functionCalls,omitempty"`
	FunctionResponse *funcResponse `json:"functionResponse,omitempty"`
}

// funcCall is a function the model wants to call (request and response).
type funcCall struct {
	Name string         `json:"name"`
	Args map[string]any `json:"args"`
}

// funcResponse is the result of a function call fed back to the model.
type funcResponse struct {
	Name     string         `json:"name"`
	Response map[string]any `json:"response"`
}

// apiTool groups the function declarations the model may use.
type apiTool struct {
	FunctionDeclarations []funcDecl `json:"function_declarations"`
}

// funcDecl declares one function for the model.
type funcDecl struct {
	Name        string         `json:"name"`
	Description string         `json:"description,omitempty"`
	Parameters  map[string]any `json:"parameters,omitempty"`
}

// apiToolConfig sets the function-calling mode.
type apiToolConfig struct {
	FunctionCallingConfig funcCallConfig `json:"function_calling_config"`
}

// funcCallConfig names the function-calling mode (auto, any, none).
type funcCallConfig struct {
	Mode string `json:"mode"`
}

// genConfig holds generation tuning options.
type genConfig struct {
	Temperature     *float64 `json:"temperature,omitempty"`
	MaxOutputTokens *int64   `json:"max_output_tokens,omitempty"`
}

// params translates a normalized ai.Request into a generateContent request.
func (g *geminiProvider) params(req ai.Request) genContentRequest {
	p := genContentRequest{Contents: toContents(req.Conversation())}

	if req.SystemPrompt != "" {
		p.SystemInstruction = &apiContent{Parts: []apiPart{{Text: req.SystemPrompt}}}
	}
	if len(req.Tools) > 0 {
		decls := make([]funcDecl, 0, len(req.Tools))
		for _, t := range req.Tools {
			decls = append(decls, funcDecl{
				Name:        t.Name,
				Description: t.Description,
				Parameters:  t.Parameters,
			})
		}
		p.Tools = []apiTool{{FunctionDeclarations: decls}}
		p.ToolConfig = &apiToolConfig{FunctionCallingConfig: funcCallConfig{Mode: "auto"}}
	}
	if req.Temperature != nil || (req.MaxTokens != nil && *req.MaxTokens > 0) {
		p.GenerationConfig = &genConfig{}
		if req.Temperature != nil {
			p.GenerationConfig.Temperature = req.Temperature
		}
		if req.MaxTokens != nil && *req.MaxTokens > 0 {
			mt := int64(*req.MaxTokens)
			p.GenerationConfig.MaxOutputTokens = &mt
		}
	}
	return p
}

// toContents converts the normalized conversation into Gemini contents,
// merging consecutive turns of the same role so the user/model alternation the
// API expects is preserved (several tool results become one user content, an
// assistant's text plus its calls become one model content).
func toContents(msgs []ai.Message) []apiContent {
	var out []apiContent
	add := func(role string, part apiPart) {
		if n := len(out); n > 0 && out[n-1].Role == role {
			out[n-1].Parts = append(out[n-1].Parts, part)
			return
		}
		out = append(out, apiContent{Role: role, Parts: []apiPart{part}})
	}

	for _, m := range msgs {
		switch {
		case m.Role == ai.RoleAssistant && m.Name != "":
			// A tool call the model requested.
			add("model", apiPart{FunctionCalls: []funcCall{{
				Name: m.Name,
				Args: parseArgs(m.ToolArguments),
			}}})
		case m.Role == ai.RoleAssistant:
			if m.Content == "" {
				continue
			}
			add("model", apiPart{Text: m.Content})
		case m.Role == ai.RoleTool:
			content := m.Content
			if content == "" {
				content = "(no output)"
			}
			add("user", apiPart{FunctionResponse: &funcResponse{
				Name:     m.Name,
				Response: toolResponse(content),
			}})
		default: // user and any unexpected role
			if m.Content == "" {
				continue
			}
			add("user", apiPart{Text: m.Content})
		}
	}
	return out
}

// parseArgs turns a tool call's JSON arguments string into the object Gemini
// expects. An empty or invalid value becomes an empty object.
func parseArgs(s string) map[string]any {
	if s == "" {
		return map[string]any{}
	}
	var m map[string]any
	if err := json.Unmarshal([]byte(s), &m); err != nil {
		return map[string]any{}
	}
	return m
}

// toolResponse wraps a tool's string result into the object Gemini's
// functionResponse.response requires. A result that is already a JSON object is
// passed through unchanged.
func toolResponse(s string) map[string]any {
	if s != "" {
		var m map[string]any
		if err := json.Unmarshal([]byte(s), &m); err == nil {
			return m
		}
	}
	return map[string]any{"output": s}
}

// genContentResponse is the generateContent response body (also the shape of
// each streaming chunk).
type genContentResponse struct {
	Candidates []candidate `json:"candidates"`
	UsageMetadata struct {
		PromptTokenCount     int `json:"promptTokenCount"`
		CandidatesTokenCount int `json:"candidatesTokenCount"`
		TotalTokenCount      int `json:"totalTokenCount"`
	} `json:"usageMetadata"`
}

// candidate is one model output turn.
type candidate struct {
	Index        int        `json:"index"`
	Content      apiContent `json:"content"`
	FinishReason string     `json:"finishReason"`
}

// Send performs one non-streaming exchange.
func (g *geminiProvider) Send(ctx context.Context, req ai.Request) (ai.Response, error) {
	log.Printf("gemini: send: POST %s model=%s messages=%d tools=%d",
		g.generateURL(req, false), g.modelName(req), len(req.Conversation()), len(req.Tools))

	body, err := json.Marshal(g.params(req))
	if err != nil {
		return ai.Response{}, fmt.Errorf("gemini: marshal request: %w", err)
	}

	raw, err := g.do(ctx, g.generateURL(req, false), body)
	if err != nil {
		return ai.Response{}, err
	}
	return toResponse(raw), nil
}

// do posts body to a Gemini endpoint and decodes the response.
func (g *geminiProvider) do(ctx context.Context, url string, body []byte) (*genContentResponse, error) {
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("gemini: build request: %w", err)
	}
	g.setHeaders(httpReq)

	resp, err := g.client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("gemini: POST %s: %w", url, err)
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("gemini: read response: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		err := fmt.Errorf("gemini: POST %s: status %d: %s",
			url, resp.StatusCode, strings.TrimSpace(string(data)))
		if resp.StatusCode == http.StatusTooManyRequests {
			return nil, &ai.RateLimitError{Err: err, RetryAfter: retryAfter(resp.Header)}
		}
		return nil, err
	}

	var raw genContentResponse
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("gemini: decode response: %w", err)
	}
	return &raw, nil
}

// toResponse normalizes a raw response into an ai.Response.
func toResponse(raw *genContentResponse) ai.Response {
	if len(raw.Candidates) == 0 {
		return ai.Response{
			Finished: true,
			Usage: ai.Usage{
				InputTokens:  raw.UsageMetadata.PromptTokenCount,
				OutputTokens: raw.UsageMetadata.CandidatesTokenCount,
			},
			Raw: raw,
		}
	}

	c := raw.Candidates[0]
	var text strings.Builder
	var messages []ai.Message
	callSeq := 0
	for _, part := range c.Content.Parts {
		if part.Text != "" {
			text.WriteString(part.Text)
		}
		for _, fc := range part.FunctionCalls {
			args := "{}"
			if fc.Args != nil {
				if b, err := json.Marshal(fc.Args); err == nil {
					args = string(b)
				}
			}
			messages = append(messages, ai.Message{
				Role:          ai.RoleAssistant,
				Name:          fc.Name,
				ToolCallID:    fmt.Sprintf("gemini-call-%d", callSeq),
				ToolArguments: args,
			})
			callSeq++
		}
	}

	return ai.Response{
		Text:     text.String(),
		Messages: messages,
		Finished: c.FinishReason != "MAX_TOKENS",
		Usage: ai.Usage{
			InputTokens:  raw.UsageMetadata.PromptTokenCount,
			OutputTokens: raw.UsageMetadata.CandidatesTokenCount,
		},
		Raw: raw,
	}
}

// SendStream performs a streaming exchange, calling onChunk for every text
// delta. A non-nil return from onChunk aborts the stream and is returned
// unwrapped so the loop can recognize a deliberate stop.
func (g *geminiProvider) SendStream(ctx context.Context, req ai.Request, onChunk func(ai.Chunk) error) (ai.Response, error) {
	url := g.generateURL(req, true)
	log.Printf("gemini: stream: POST %s model=%s messages=%d tools=%d",
		url, g.modelName(req), len(req.Conversation()), len(req.Tools))

	params := g.params(req)
	body, err := json.Marshal(params)
	if err != nil {
		return ai.Response{}, fmt.Errorf("gemini: marshal request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return ai.Response{}, fmt.Errorf("gemini: build request: %w", err)
	}
	g.setHeaders(httpReq)
	httpReq.Header.Set("Accept", "text/event-stream")

	resp, err := g.client.Do(httpReq)
	if err != nil {
		return ai.Response{}, fmt.Errorf("gemini: POST %s (stream): %w", url, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		data, _ := io.ReadAll(resp.Body)
		err := fmt.Errorf("gemini: POST %s (stream): status %d: %s",
			url, resp.StatusCode, strings.TrimSpace(string(data)))
		if resp.StatusCode == http.StatusTooManyRequests {
			return ai.Response{}, &ai.RateLimitError{Err: err, RetryAfter: retryAfter(resp.Header)}
		}
		return ai.Response{}, err
	}

	return g.consumeStream(resp.Body, onChunk)
}

// rawFuncCall captures a streamed function call, keeping the arguments as raw
// JSON so split arguments can be reassembled.
type rawFuncCall struct {
	Name string          `json:"name"`
	Args json.RawMessage `json:"args"`
}

// streamPart is one streamed part.
type streamPart struct {
	Text          string        `json:"text"`
	FunctionCalls []rawFuncCall `json:"functionCalls"`
}

// streamFuncAcc collects a streamed function call across chunks.
type streamFuncAcc struct {
	name string
	args strings.Builder
}

// consumeStream reads the SSE stream, forwarding text deltas to onChunk and
// accumulating function calls.
func (g *geminiProvider) consumeStream(r io.Reader, onChunk func(ai.Chunk) error) (ai.Response, error) {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	var (
		text         strings.Builder
		inputTokens  int
		outputTokens int
		finishReason string
		funcAccs     = map[string]*streamFuncAcc{}
		funcOrder    []string
	)

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		data := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if data == "" || data == "[DONE]" {
			continue
		}

		var chunk genContentResponse
		if err := json.Unmarshal([]byte(data), &chunk); err != nil {
			return ai.Response{}, fmt.Errorf("gemini: decode stream event: %w", err)
		}

		if len(chunk.Candidates) == 0 {
			continue
		}
		c := chunk.Candidates[0]
		if c.FinishReason != "" {
			finishReason = c.FinishReason
		}
		if chunk.UsageMetadata.PromptTokenCount > 0 {
			inputTokens = chunk.UsageMetadata.PromptTokenCount
		}
		if chunk.UsageMetadata.CandidatesTokenCount > 0 {
			outputTokens = chunk.UsageMetadata.CandidatesTokenCount
		}

		for pi, part := range c.Content.Parts {
			if part.Text != "" {
				text.WriteString(part.Text)
				if onChunk != nil {
					if err := onChunk(ai.Chunk{Delta: part.Text}); err != nil {
						return ai.Response{}, err
					}
				}
			}
			for fi, fc := range part.FunctionCalls {
				key := fmt.Sprintf("%d-%d", pi, fi)
				acc := funcAccs[key]
				if acc == nil {
					acc = &streamFuncAcc{}
					funcAccs[key] = acc
					funcOrder = append(funcOrder, key)
				}
				if fc.Name != "" && acc.name == "" {
					acc.name = fc.Name
				}
				if len(fc.Args) > 0 {
					if b, err := json.Marshal(fc.Args); err == nil {
						acc.args.Write(b)
					}
				}
			}
		}
	}
	if err := scanner.Err(); err != nil {
		return ai.Response{}, fmt.Errorf("gemini: read stream: %w", err)
	}

	var messages []ai.Message
	for i, key := range funcOrder {
		acc := funcAccs[key]
		args := acc.args.String()
		if args == "" || !json.Valid([]byte(args)) {
			args = "{}"
		}
		messages = append(messages, ai.Message{
			Role:          ai.RoleAssistant,
			Name:          acc.name,
			ToolCallID:    fmt.Sprintf("gemini-call-%d", i),
			ToolArguments: args,
		})
	}

	log.Printf("gemini: stream done: %d chars, %d tool call(s)", text.Len(), len(messages))

	return ai.Response{
		Text:     text.String(),
		Messages: messages,
		Finished: finishReason != "MAX_TOKENS",
		Usage: ai.Usage{
			InputTokens:  inputTokens,
			OutputTokens: outputTokens,
		},
	}, nil
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

// ListModelsGemini fetches available models from the Gemini Models API
// (GET {baseURL}/models). The listing returns models named "models/{id}"; the
// models/ prefix is stripped so stored model names are the bare ID.
func ListModelsGemini(baseURL, apiKey string) ([]restfulai.ModelInfo, error) {
	if baseURL == "" {
		baseURL = "https://generativelanguage.googleapis.com/v1beta"
	}
	url := strings.TrimSuffix(baseURL, "/") + "/models"

	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("gemini: create request: %w", err)
	}
	if apiKey != "" {
		req.Header.Set("x-goog-api-key", apiKey)
	}

	client := &http.Client{Timeout: 60 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("gemini: fetch models: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("gemini: fetch models: status %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	var result struct {
		Models []struct {
			Name                     string   `json:"name"`
			DisplayName              string   `json:"displayName"`
			Description              string   `json:"description"`
			SupportedGenerationMethods []string `json:"supportedGenerationMethods"`
		} `json:"models"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("gemini: decode models: %w", err)
	}

	infos := make([]restfulai.ModelInfo, 0, len(result.Models))
	for _, m := range result.Models {
		id := strings.TrimPrefix(m.Name, "models/")
		if id == "" {
			continue
		}
		// Only list chat-capable models when the capability is reported.
		if len(m.SupportedGenerationMethods) > 0 && !contains(m.SupportedGenerationMethods, "generateContent") {
			continue
		}
		desc := m.DisplayName
		if desc == "" {
			desc = m.Description
		}
		infos = append(infos, restfulai.ModelInfo{ID: id, Description: desc})
	}
	return infos, nil
}

// contains reports whether s is in list.
func contains(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}