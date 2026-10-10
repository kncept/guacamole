// Package anthropicai implements ai.Provider for the Anthropic Messages
// API, including streaming and tool use.
package anthropicai

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/kncept/guacamole/ai"
	"github.com/kncept/guacamole/config"
	"github.com/kncept/guacamole/restfulai"
)

// anthropicVersion is the API version sent on every request; the Messages API
// requires it.
const anthropicVersion = "2023-06-01"

// defaultMaxTokens is used when a request does not set MaxTokens: the
// Messages API requires the field.
const defaultMaxTokens = 4096

// AnthropicAI is an ai.Provider backed by the Anthropic Messages API.
type AnthropicAI interface {
	ai.Provider
}

type anthropicAI struct {
	client  *http.Client
	baseURL string
	apiKey  string
	model   string
}

// NewAnthropicAI builds a Provider from API connection details. It fails fast
// on a missing base URL, the same way restfulai.NewRestfulAI does.
func NewAnthropicAI(conf *config.ApiModelInterfaceDetails) (AnthropicAI, error) {
	if conf.BaseUrl == "" {
		return nil, fmt.Errorf("anthropicai: no base URL for model %q; set base URL in provider configuration", conf.ModelName)
	}
	return &anthropicAI{
		client:  &http.Client{Timeout: 10 * time.Minute},
		baseURL: strings.TrimSuffix(conf.BaseUrl, "/"),
		apiKey:  conf.ApiKey,
		model:   conf.ModelName,
	}, nil
}

// modelName returns the model for req, falling back to the provider default.
func (a *anthropicAI) modelName(req ai.Request) string {
	if req.Model != "" {
		return req.Model
	}
	return a.model
}

// endpoint builds a Messages API URL from the base URL.
func (a *anthropicAI) endpoint(path string) string {
	return a.baseURL + path
}

// setHeaders applies the Anthropic authentication and version headers.
func (a *anthropicAI) setHeaders(req *http.Request) {
	if a.apiKey != "" {
		req.Header.Set("x-api-key", a.apiKey)
	}
	req.Header.Set("anthropic-version", anthropicVersion)
}

// apiRequest is the Messages API request body.
type apiRequest struct {
	Model       string       `json:"model"`
	MaxTokens   int          `json:"max_tokens"`
	System      string       `json:"system,omitempty"`
	Messages    []apiMessage `json:"messages"`
	Tools       []apiTool    `json:"tools,omitempty"`
	Temperature *float64     `json:"temperature,omitempty"`
	Stream      bool         `json:"stream,omitempty"`
}

// apiMessage is one conversation turn: a list of content blocks.
type apiMessage struct {
	Role    string     `json:"role"`
	Content []apiBlock `json:"content"`
}

// apiBlock is one content block. Only the fields relevant to its Type are set.
type apiBlock struct {
	Type string `json:"type"`
	// text
	Text string `json:"text,omitempty"`
	// tool_use
	ID    string          `json:"id,omitempty"`
	Name  string          `json:"name,omitempty"`
	Input json.RawMessage `json:"input,omitempty"`
	// tool_result
	ToolUseID string `json:"tool_use_id,omitempty"`
	Content   string `json:"content,omitempty"`
	IsError   bool   `json:"is_error,omitempty"`
}

// apiTool is one tool definition the model may call.
type apiTool struct {
	Name        string         `json:"name"`
	Description string         `json:"description,omitempty"`
	InputSchema map[string]any `json:"input_schema"`
}

// params translates a normalized ai.Request into Messages API params.
func (a *anthropicAI) params(req ai.Request) apiRequest {
	maxTokens := defaultMaxTokens
	if req.MaxTokens != nil && *req.MaxTokens > 0 {
		maxTokens = *req.MaxTokens
	}

	p := apiRequest{
		Model:       a.modelName(req),
		MaxTokens:   maxTokens,
		System:      req.SystemPrompt,
		Messages:    toMessages(req.Conversation()),
		Temperature: req.Temperature,
	}
	for _, t := range req.Tools {
		schema := t.Parameters
		if schema == nil {
			schema = map[string]any{"type": "object"}
		}
		p.Tools = append(p.Tools, apiTool{
			Name:        t.Name,
			Description: t.Description,
			InputSchema: schema,
		})
	}
	return p
}

// toMessages converts the normalized conversation into Anthropic messages.
// Consecutive turns with the same role are merged into one message, since the
// Messages API requires strict user/assistant alternation: several tool
// results become one user message, and an assistant's text plus its tool
// calls become one assistant message.
func toMessages(msgs []ai.Message) []apiMessage {
	var out []apiMessage
	add := func(role string, block apiBlock) {
		if n := len(out); n > 0 && out[n-1].Role == role {
			out[n-1].Content = append(out[n-1].Content, block)
			return
		}
		out = append(out, apiMessage{Role: role, Content: []apiBlock{block}})
	}

	for _, m := range msgs {
		switch {
		case m.Role == ai.RoleAssistant && m.Name != "":
			input := json.RawMessage(m.ToolArguments)
			if len(input) == 0 || !json.Valid(input) {
				input = json.RawMessage("{}")
			}
			add("assistant", apiBlock{Type: "tool_use", ID: m.ToolCallID, Name: m.Name, Input: input})
		case m.Role == ai.RoleAssistant:
			if m.Content == "" {
				continue
			}
			add("assistant", apiBlock{Type: "text", Text: m.Content})
		case m.Role == ai.RoleTool:
			content := m.Content
			if content == "" {
				content = "(no output)"
			}
			add("user", apiBlock{Type: "tool_result", ToolUseID: m.ToolCallID, Content: content})
		default: // user and any unexpected role
			if m.Content == "" {
				continue
			}
			add("user", apiBlock{Type: "text", Text: m.Content})
		}
	}
	return out
}

// apiResponse is the Messages API response body.
type apiResponse struct {
	ID      string `json:"id"`
	Content []struct {
		Type  string          `json:"type"`
		Text  string          `json:"text"`
		ID    string          `json:"id"`
		Name  string          `json:"name"`
		Input json.RawMessage `json:"input"`
	} `json:"content"`
	StopReason string `json:"stop_reason"`
	Usage      struct {
		InputTokens  int `json:"input_tokens"`
		OutputTokens int `json:"output_tokens"`
	} `json:"usage"`
}

// Send performs one non-streaming exchange.
func (a *anthropicAI) Send(ctx context.Context, req ai.Request) (ai.Response, error) {
	log.Printf("anthropicai: send: POST %s/messages model=%s messages=%d tools=%d",
		a.baseURL, a.modelName(req), len(req.Conversation()), len(req.Tools))

	body, err := json.Marshal(a.params(req))
	if err != nil {
		return ai.Response{}, fmt.Errorf("anthropicai: marshal request: %w", err)
	}

	raw, err := a.do(ctx, body)
	if err != nil {
		return ai.Response{}, err
	}
	return toResponse(raw), nil
}

// do posts body to the Messages API and decodes the response.
func (a *anthropicAI) do(ctx context.Context, body []byte) (*apiResponse, error) {
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, a.endpoint("/messages"), bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("anthropicai: build request: %w", err)
	}
	a.setHeaders(httpReq)
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := a.client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("anthropicai: POST %s/messages: %w", a.baseURL, err)
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("anthropicai: read response: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("anthropicai: POST %s/messages: status %d: %s",
			a.baseURL, resp.StatusCode, strings.TrimSpace(string(data)))
	}

	var raw apiResponse
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("anthropicai: decode response: %w", err)
	}
	return &raw, nil
}

// toResponse normalizes a raw response into an ai.Response.
func toResponse(raw *apiResponse) ai.Response {
	var text strings.Builder
	var messages []ai.Message
	for _, block := range raw.Content {
		switch block.Type {
		case "text":
			text.WriteString(block.Text)
		case "tool_use":
			args := block.Input
			if len(args) == 0 {
				args = json.RawMessage("{}")
			}
			messages = append(messages, ai.Message{
				Role:          ai.RoleAssistant,
				Name:          block.Name,
				ToolCallID:    block.ID,
				ToolArguments: string(args),
			})
		}
	}
	return ai.Response{
		Text:     text.String(),
		Messages: messages,
		Finished: raw.StopReason != "max_tokens",
		Usage: ai.Usage{
			InputTokens:  raw.Usage.InputTokens,
			OutputTokens: raw.Usage.OutputTokens,
		},
		Raw: raw,
	}
}

// SendStream performs a streaming exchange, calling onChunk for every text
// delta. A non-nil return from onChunk aborts the stream and is returned
// unwrapped so the loop can recognize a deliberate stop.
func (a *anthropicAI) SendStream(ctx context.Context, req ai.Request, onChunk func(ai.Chunk) error) (ai.Response, error) {
	log.Printf("anthropicai: stream: POST %s/messages model=%s messages=%d tools=%d",
		a.baseURL, a.modelName(req), len(req.Conversation()), len(req.Tools))

	params := a.params(req)
	params.Stream = true
	body, err := json.Marshal(params)
	if err != nil {
		return ai.Response{}, fmt.Errorf("anthropicai: marshal request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, a.endpoint("/messages"), bytes.NewReader(body))
	if err != nil {
		return ai.Response{}, fmt.Errorf("anthropicai: build request: %w", err)
	}
	a.setHeaders(httpReq)
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Accept", "text/event-stream")

	resp, err := a.client.Do(httpReq)
	if err != nil {
		return ai.Response{}, fmt.Errorf("anthropicai: POST %s/messages (stream): %w", a.baseURL, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		data, _ := io.ReadAll(resp.Body)
		return ai.Response{}, fmt.Errorf("anthropicai: POST %s/messages (stream): status %d: %s",
			a.baseURL, resp.StatusCode, strings.TrimSpace(string(data)))
	}

	return a.consumeStream(resp.Body, onChunk)
}

// toolAccumulator collects a streamed tool_use block: its id and name arrive
// in content_block_start, and its JSON arguments in input_json_delta pieces.
type toolAccumulator struct {
	id   string
	name string
	args strings.Builder
}

// streamEvent is the subset of a Messages API SSE event this client reads.
type streamEvent struct {
	Type         string `json:"type"`
	Index        int    `json:"index"`
	ContentBlock struct {
		Type string `json:"type"`
		ID   string `json:"id"`
		Name string `json:"name"`
	} `json:"content_block"`
	Delta struct {
		Type        string `json:"type"`
		Text        string `json:"text"`
		PartialJSON string `json:"partial_json"`
		StopReason  string `json:"stop_reason"`
	} `json:"delta"`
	Message struct {
		Usage struct {
			InputTokens  int `json:"input_tokens"`
			OutputTokens int `json:"output_tokens"`
		} `json:"usage"`
	} `json:"message"`
	Usage struct {
		OutputTokens int `json:"output_tokens"`
	} `json:"usage"`
	Error struct {
		Type    string `json:"type"`
		Message string `json:"message"`
	} `json:"error"`
}

// consumeStream reads the SSE stream, forwarding text deltas to onChunk and
// accumulating tool calls.
func (a *anthropicAI) consumeStream(r io.Reader, onChunk func(ai.Chunk) error) (ai.Response, error) {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	var (
		text         strings.Builder
		inputTokens  int
		outputTokens int
		stopReason   string
		toolOrder    []int
		toolByIndex  = map[int]*toolAccumulator{}
	)

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		data := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if data == "" {
			continue
		}

		var ev streamEvent
		if err := json.Unmarshal([]byte(data), &ev); err != nil {
			return ai.Response{}, fmt.Errorf("anthropicai: decode stream event: %w", err)
		}

		switch ev.Type {
		case "message_start":
			inputTokens = ev.Message.Usage.InputTokens
		case "content_block_start":
			if ev.ContentBlock.Type == "tool_use" {
				toolByIndex[ev.Index] = &toolAccumulator{id: ev.ContentBlock.ID, name: ev.ContentBlock.Name}
				toolOrder = append(toolOrder, ev.Index)
			}
		case "content_block_delta":
			switch ev.Delta.Type {
			case "text_delta":
				text.WriteString(ev.Delta.Text)
				if onChunk != nil {
					if err := onChunk(ai.Chunk{Delta: ev.Delta.Text}); err != nil {
						return ai.Response{}, err
					}
				}
			case "input_json_delta":
				if acc := toolByIndex[ev.Index]; acc != nil {
					acc.args.WriteString(ev.Delta.PartialJSON)
				}
			}
		case "message_delta":
			if ev.Delta.StopReason != "" {
				stopReason = ev.Delta.StopReason
			}
			if ev.Usage.OutputTokens > 0 {
				outputTokens = ev.Usage.OutputTokens
			}
		case "error":
			return ai.Response{}, fmt.Errorf("anthropicai: stream error: %s: %s", ev.Error.Type, ev.Error.Message)
		}
	}
	if err := scanner.Err(); err != nil {
		return ai.Response{}, fmt.Errorf("anthropicai: read stream: %w", err)
	}

	var messages []ai.Message
	for _, idx := range toolOrder {
		acc := toolByIndex[idx]
		args := acc.args.String()
		if args == "" || !json.Valid([]byte(args)) {
			args = "{}"
		}
		messages = append(messages, ai.Message{
			Role:          ai.RoleAssistant,
			Name:          acc.name,
			ToolCallID:    acc.id,
			ToolArguments: args,
		})
	}

	log.Printf("anthropicai: stream done: %d chars, %d tool call(s)", text.Len(), len(messages))

	return ai.Response{
		Text:     text.String(),
		Messages: messages,
		Finished: stopReason != "max_tokens",
		Usage: ai.Usage{
			InputTokens:  inputTokens,
			OutputTokens: outputTokens,
		},
	}, nil
}

// ListModelsAnthropic fetches available models from the Anthropic Models API.
// baseURL is the Anthropic base URL (for example https://api.anthropic.com/v1).
func ListModelsAnthropic(baseURL, apiKey string) ([]restfulai.ModelInfo, error) {
	if baseURL == "" {
		baseURL = "https://api.anthropic.com/v1"
	}
	url := strings.TrimSuffix(baseURL, "/") + "/models"

	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("anthropic: create request: %w", err)
	}
	if apiKey != "" {
		req.Header.Set("x-api-key", apiKey)
	}
	req.Header.Set("anthropic-version", anthropicVersion)

	resp, err := (&http.Client{Timeout: 60 * time.Second}).Do(req)
	if err != nil {
		return nil, fmt.Errorf("anthropic: fetch models: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("anthropic: fetch models: status %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	var result struct {
		Data []struct {
			ID          string `json:"id"`
			DisplayName string `json:"display_name"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("anthropic: decode models: %w", err)
	}

	infos := make([]restfulai.ModelInfo, 0, len(result.Data))
	for _, m := range result.Data {
		infos = append(infos, restfulai.ModelInfo{ID: m.ID, Description: m.DisplayName})
	}
	return infos, nil
}
