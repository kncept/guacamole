// Package mistralai implements ai.Provider for the Mistral AI Chat
// Completions API, including streaming and tool use.
package mistralai

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

// MistralAI is an ai.Provider backed by the Mistral AI Chat Completions API.
type MistralAI interface {
	ai.Provider
}

type mistralAI struct {
	client  *http.Client
	baseURL string
	apiKey  string
	model   string
}

// NewMistralAI builds a Provider from API connection details. It fails fast
// on a missing base URL.
func NewMistralAI(conf *config.ApiModelInterfaceDetails) (MistralAI, error) {
	if conf.BaseUrl == "" {
		return nil, fmt.Errorf("mistralai: no base URL for model %q; set base URL in provider configuration", conf.ModelName)
	}
	return &mistralAI{
		client:  &http.Client{Timeout: 10 * time.Minute},
		baseURL: strings.TrimSuffix(conf.BaseUrl, "/"),
		apiKey:  conf.ApiKey,
		model:   conf.ModelName,
	}, nil
}

// modelName returns the model for req, falling back to the provider default.
func (m *mistralAI) modelName(req ai.Request) string {
	if req.Model != "" {
		return req.Model
	}
	return m.model
}

// endpoint builds a Chat Completions API URL from the base URL.
func (m *mistralAI) endpoint(path string) string {
	return m.baseURL + path
}

// setHeaders applies the Mistral authentication headers.
func (m *mistralAI) setHeaders(req *http.Request) {
	if m.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+m.apiKey)
	}
	req.Header.Set("Content-Type", "application/json")
}

// apiRequest is the Chat Completions API request body.
type apiRequest struct {
	Model            string          `json:"model"`
	Messages         []apiMessage    `json:"messages"`
	Temperature      *float64        `json:"temperature,omitempty"`
	TopP             *float64        `json:"top_p,omitempty"`
	MaxTokens        int             `json:"max_tokens,omitempty"`
	Stream           bool            `json:"stream,omitempty"`
	Tools            []apiTool       `json:"tools,omitempty"`
	ToolChoice       any             `json:"tool_choice,omitempty"`
	ResponseFormat   *apiResponseFormat `json:"response_format,omitempty"`
	SafePrompt       *bool           `json:"safe_prompt,omitempty"`
	RandomSeed       *int            `json:"random_seed,omitempty"`
	Stop             any             `json:"stop,omitempty"`
	FrequencyPenalty *float64        `json:"frequency_penalty,omitempty"`
	PresencePenalty  *float64        `json:"presence_penalty,omitempty"`
}

// apiMessage represents a message in the conversation.
type apiMessage struct {
	Role         string        `json:"role"`
	Content      string        `json:"content,omitempty"`
	ToolCallID   string        `json:"tool_call_id,omitempty"`
	ToolCalls    []apiToolCall `json:"tool_calls,omitempty"`
	Name         string        `json:"name,omitempty"`
}

// apiToolCall represents a tool call in an assistant message.
type apiToolCall struct {
	ID       string       `json:"id"`
	Type     string       `json:"type"`
	Function apiFunction  `json:"function"`
}

// apiFunction represents the function part of a tool call.
type apiFunction struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

// apiTool represents a tool definition.
type apiTool struct {
	Type     string         `json:"type"`
	Function apiToolFunction `json:"function"`
}

// apiToolFunction describes a function for the tool.
type apiToolFunction struct {
	Name        string         `json:"name"`
	Description string         `json:"description,omitempty"`
	Parameters  map[string]any `json:"parameters"`
}

// apiResponseFormat specifies the response format.
type apiResponseFormat struct {
	Type       string                 `json:"type"`
	JSONSchema map[string]any         `json:"json_schema,omitempty"`
}

// params translates a normalized ai.Request into Chat Completions API params.
func (m *mistralAI) params(req ai.Request) apiRequest {
	p := apiRequest{
		Model:    m.modelName(req),
		Messages: toMessages(req.Conversation()),
	}
	if req.Temperature != nil {
		p.Temperature = req.Temperature
	}
	if req.MaxTokens != nil && *req.MaxTokens > 0 {
		p.MaxTokens = *req.MaxTokens
	}
	if req.SystemPrompt != "" {
		// Mistral doesn't have a separate system parameter; prepend as system message
		p.Messages = append([]apiMessage{{Role: "system", Content: req.SystemPrompt}}, p.Messages...)
	}
	if len(req.Tools) > 0 {
		tools := make([]apiTool, 0, len(req.Tools))
		for _, t := range req.Tools {
			schema := t.Parameters
			if schema == nil {
				schema = map[string]any{"type": "object"}
			}
			tools = append(tools, apiTool{
				Type: "function",
				Function: apiToolFunction{
					Name:        t.Name,
					Description: t.Description,
					Parameters:  schema,
				},
			})
		}
		p.Tools = tools
		p.ToolChoice = "auto"
	}
	return p
}

// toMessages converts the normalized conversation into Chat Completions messages.
func toMessages(msgs []ai.Message) []apiMessage {
	var out []apiMessage
	for _, msg := range msgs {
		switch {
		case msg.Role == ai.RoleAssistant && msg.Name != "":
			// Tool call from assistant
			out = append(out, apiMessage{
				Role: "assistant",
				ToolCalls: []apiToolCall{{
					ID:   msg.ToolCallID,
					Type: "function",
					Function: apiFunction{
						Name:      msg.Name,
						Arguments: msg.ToolArguments,
					},
				}},
			})
		case msg.Role == ai.RoleAssistant:
			// Regular assistant message
			if msg.Content != "" {
				out = append(out, apiMessage{
					Role:    "assistant",
					Content: msg.Content,
				})
			}
		case msg.Role == ai.RoleTool:
			// Tool result
			content := msg.Content
			if content == "" {
				content = "(no output)"
			}
			out = append(out, apiMessage{
				Role:       "tool",
				Content:    content,
				ToolCallID: msg.ToolCallID,
			})
		default: // user, system, and any unexpected role
			if msg.Content != "" {
				out = append(out, apiMessage{
					Role:    string(msg.Role),
					Content: msg.Content,
				})
			}
		}
	}
	return out
}

// apiResponse is the Chat Completions API response body.
type apiResponse struct {
	ID      string `json:"id"`
	Object  string `json:"object"`
	Created int64  `json:"created"`
	Model   string `json:"model"`
	Choices []struct {
		Index        int `json:"index"`
		Message      struct {
			Role      string        `json:"role"`
			Content   string        `json:"content"`
			ToolCalls []apiToolCall `json:"tool_calls"`
		} `json:"message"`
		FinishReason string `json:"finish_reason"`
	} `json:"choices"`
	Usage struct {
		PromptTokens     int `json:"prompt_tokens"`
		CompletionTokens int `json:"completion_tokens"`
		TotalTokens      int `json:"total_tokens"`
	} `json:"usage"`
}

// Send performs one non-streaming exchange.
func (m *mistralAI) Send(ctx context.Context, req ai.Request) (ai.Response, error) {
	log.Printf("mistralai: send: POST %s/chat/completions model=%s messages=%d tools=%d",
		m.baseURL, m.modelName(req), len(req.Conversation()), len(req.Tools))

	body, err := json.Marshal(m.params(req))
	if err != nil {
		return ai.Response{}, fmt.Errorf("mistralai: marshal request: %w", err)
	}

	raw, err := m.do(ctx, body)
	if err != nil {
		return ai.Response{}, err
	}
	return toResponse(raw), nil
}

// do posts body to the Chat Completions API and decodes the response.
func (m *mistralAI) do(ctx context.Context, body []byte) (*apiResponse, error) {
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, m.endpoint("/chat/completions"), bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("mistralai: build request: %w", err)
	}
	m.setHeaders(httpReq)

	resp, err := m.client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("mistralai: POST %s/chat/completions: %w", m.baseURL, err)
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("mistralai: read response: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		err := fmt.Errorf("mistralai: POST %s/chat/completions: status %d: %s",
			m.baseURL, resp.StatusCode, strings.TrimSpace(string(data)))
		if resp.StatusCode == http.StatusTooManyRequests {
			return nil, &ai.RateLimitError{Err: err, RetryAfter: retryAfter(resp.Header)}
		}
		return nil, err
	}

	var raw apiResponse
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("mistralai: decode response: %w", err)
	}
	return &raw, nil
}

// toResponse normalizes a raw response into an ai.Response.
func toResponse(raw *apiResponse) ai.Response {
	if len(raw.Choices) == 0 {
		return ai.Response{
			Finished: true,
			Usage: ai.Usage{
				InputTokens:  raw.Usage.PromptTokens,
				OutputTokens: raw.Usage.CompletionTokens,
			},
			Raw: raw,
		}
	}

	choice := raw.Choices[0]
	var text string
	var messages []ai.Message

	if choice.Message.Content != "" {
		text = choice.Message.Content
	}
	for _, tc := range choice.Message.ToolCalls {
		messages = append(messages, ai.Message{
			Role:          ai.RoleAssistant,
			Name:          tc.Function.Name,
			ToolCallID:    tc.ID,
			ToolArguments: tc.Function.Arguments,
		})
	}

	finished := choice.FinishReason != "length"

	return ai.Response{
		Text:     text,
		Messages: messages,
		Finished: finished,
		Usage: ai.Usage{
			InputTokens:  raw.Usage.PromptTokens,
			OutputTokens: raw.Usage.CompletionTokens,
		},
		Raw: raw,
	}
}

// SendStream performs a streaming exchange, calling onChunk for every text
// delta. A non-nil return from onChunk aborts the stream and is returned
// unwrapped so the loop can recognize a deliberate stop.
func (m *mistralAI) SendStream(ctx context.Context, req ai.Request, onChunk func(ai.Chunk) error) (ai.Response, error) {
	log.Printf("mistralai: stream: POST %s/chat/completions model=%s messages=%d tools=%d",
		m.baseURL, m.modelName(req), len(req.Conversation()), len(req.Tools))

	params := m.params(req)
	params.Stream = true
	body, err := json.Marshal(params)
	if err != nil {
		return ai.Response{}, fmt.Errorf("mistralai: marshal request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, m.endpoint("/chat/completions"), bytes.NewReader(body))
	if err != nil {
		return ai.Response{}, fmt.Errorf("mistralai: build request: %w", err)
	}
	m.setHeaders(httpReq)
	httpReq.Header.Set("Accept", "text/event-stream")

	resp, err := m.client.Do(httpReq)
	if err != nil {
		return ai.Response{}, fmt.Errorf("mistralai: POST %s/chat/completions (stream): %w", m.baseURL, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		data, _ := io.ReadAll(resp.Body)
		err := fmt.Errorf("mistralai: POST %s/chat/completions (stream): status %d: %s",
			m.baseURL, resp.StatusCode, strings.TrimSpace(string(data)))
		if resp.StatusCode == http.StatusTooManyRequests {
			return ai.Response{}, &ai.RateLimitError{Err: err, RetryAfter: retryAfter(resp.Header)}
		}
		return ai.Response{}, err
	}

	return m.consumeStream(resp.Body, onChunk)
}

// streamToolAccumulator collects a streamed tool call.
type streamToolAccumulator struct {
	id        string
	name      string
	arguments strings.Builder
}

// streamEvent is the subset of a Chat Completions SSE event this client reads.
type streamEvent struct {
	ID      string `json:"id"`
	Object  string `json:"object"`
	Created int64  `json:"created"`
	Model   string `json:"model"`
	Choices []struct {
		Index int `json:"index"`
		Delta struct {
			Role      string            `json:"role"`
			Content   string            `json:"content"`
			ToolCalls []streamToolCall  `json:"tool_calls"`
		} `json:"delta"`
		FinishReason string `json:"finish_reason"`
	} `json:"choices"`
	Usage struct {
		PromptTokens     int `json:"prompt_tokens"`
		CompletionTokens int `json:"completion_tokens"`
		TotalTokens      int `json:"total_tokens"`
	} `json:"usage"`
	Error struct {
		Message string `json:"message"`
		Type    string `json:"type"`
	} `json:"error"`
}

type streamToolCall struct {
	Index    int    `json:"index"`
	ID       string `json:"id"`
	Type     string `json:"type"`
	Function struct {
		Name      string `json:"name"`
		Arguments string `json:"arguments"`
	} `json:"function"`
}

// consumeStream reads the SSE stream, forwarding text deltas to onChunk and
// accumulating tool calls.
func (m *mistralAI) consumeStream(r io.Reader, onChunk func(ai.Chunk) error) (ai.Response, error) {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	var (
		text         strings.Builder
		inputTokens  int
		outputTokens int
		finishReason string
		toolOrder    []int
		toolByIndex  = map[int]*streamToolAccumulator{}
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

		var ev streamEvent
		if err := json.Unmarshal([]byte(data), &ev); err != nil {
			return ai.Response{}, fmt.Errorf("mistralai: decode stream event: %w", err)
		}

		if ev.Error.Message != "" {
			return ai.Response{}, fmt.Errorf("mistralai: stream error: %s: %s", ev.Error.Type, ev.Error.Message)
		}

		if len(ev.Choices) == 0 {
			continue
		}

		choice := ev.Choices[0]

		if choice.Delta.Content != "" {
			text.WriteString(choice.Delta.Content)
			if onChunk != nil {
				if err := onChunk(ai.Chunk{Delta: choice.Delta.Content}); err != nil {
					return ai.Response{}, err
				}
			}
		}

		for _, tc := range choice.Delta.ToolCalls {
			idx := tc.Index
			if acc := toolByIndex[idx]; acc == nil {
				toolByIndex[idx] = &streamToolAccumulator{
					id:   tc.ID,
					name: tc.Function.Name,
				}
				toolOrder = append(toolOrder, idx)
			}
			if acc := toolByIndex[idx]; acc != nil && tc.Function.Arguments != "" {
				acc.arguments.WriteString(tc.Function.Arguments)
			}
		}

		if choice.FinishReason != "" {
			finishReason = choice.FinishReason
		}
		if ev.Usage.CompletionTokens > 0 {
			outputTokens = ev.Usage.CompletionTokens
		}
		if ev.Usage.PromptTokens > 0 {
			inputTokens = ev.Usage.PromptTokens
		}
	}
	if err := scanner.Err(); err != nil {
		return ai.Response{}, fmt.Errorf("mistralai: read stream: %w", err)
	}

	var messages []ai.Message
	for _, idx := range toolOrder {
		acc := toolByIndex[idx]
		args := acc.arguments.String()
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

	log.Printf("mistralai: stream done: %d chars, %d tool call(s)", text.Len(), len(messages))

	return ai.Response{
		Text:     text.String(),
		Messages: messages,
		Finished: finishReason != "length",
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

// ListModelsMistral fetches available models from the Mistral AI Models API.
// Mistral uses OpenAI-compatible /v1/models endpoint but returns richer metadata.
func ListModelsMistral(baseURL, apiKey string) ([]restfulai.ModelInfo, error) {
	if baseURL == "" {
		baseURL = "https://api.mistral.ai/v1"
	}
	url := strings.TrimSuffix(baseURL, "/") + "/models"

	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("mistral: create request: %w", err)
	}
	if apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+apiKey)
	}

	client := &http.Client{Timeout: 60 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("mistral: fetch models: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("mistral: fetch models: status %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	// Mistral returns an array directly, not wrapped in {data: []}
	var result []struct {
		ID                  string `json:"id"`
		Object              string `json:"object"`
		Created             int64  `json:"created"`
		OwnedBy             string `json:"owned_by"`
		MaxContextLength    int    `json:"max_context_length"`
		Capabilities        struct {
			CompletionChat    bool `json:"completion_chat"`
			CompletionFIM     bool `json:"completion_fim"`
			FunctionCalling   bool `json:"function_calling"`
			FineTuning        bool `json:"fine_tuning"`
			Vision            bool `json:"vision"`
			Classification    bool `json:"classification"`
		} `json:"capabilities"`
		Root       string   `json:"root"`
		Aliases    []string `json:"aliases"`
		Type       string   `json:"type"`
		Archived   bool     `json:"archived"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("mistral: decode models: %w", err)
	}

	infos := make([]restfulai.ModelInfo, 0, len(result))
	for _, m := range result {
		if m.Archived {
			continue
		}
		desc := m.ID
		if m.Root != "" {
			desc = fmt.Sprintf("%s (root: %s, ctx: %dk)", m.ID, m.Root, m.MaxContextLength/1024)
		}
		infos = append(infos, restfulai.ModelInfo{
			ID:          m.ID,
			Description: desc,
		})
	}
	return infos, nil
}