package anthropicai

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/kncept/guacamole/ai"
	"github.com/kncept/guacamole/config"
)

// TestToMessagesGroupsToolRounds checks that the normalized history maps onto
// the Messages API's strict user/assistant alternation: an assistant's text
// and tool calls become one message, and several tool results become one
// user message.
func TestToMessagesGroupsToolRounds(t *testing.T) {
	got := toMessages([]ai.Message{
		{Role: ai.RoleUser, Content: "go"},
		{Role: ai.RoleAssistant, Content: "ok"},
		{Role: ai.RoleAssistant, Name: "read_file", ToolCallID: "c1", ToolArguments: `{"path":"a.txt"}`},
		{Role: ai.RoleTool, Name: "read_file", ToolCallID: "c1", Content: "contents"},
		{Role: ai.RoleAssistant, Content: "done"},
	})

	if len(got) != 4 {
		t.Fatalf("messages = %d, want 4 (user, assistant, user, assistant): %+v", len(got), got)
	}
	if got[1].Role != "assistant" || len(got[1].Content) != 2 {
		t.Fatalf("assistant message = %+v, want text + tool_use blocks", got[1])
	}
	if got[1].Content[0].Type != "text" || got[1].Content[1].Type != "tool_use" {
		t.Errorf("assistant blocks = %+v, want [text tool_use]", got[1].Content)
	}
	if got[2].Role != "user" || got[2].Content[0].Type != "tool_result" {
		t.Errorf("tool result message = %+v, want one user tool_result", got[2])
	}
	if got[2].Content[0].ToolUseID != "c1" {
		t.Errorf("tool_result id = %q, want %q", got[2].Content[0].ToolUseID, "c1")
	}
}

// TestParamsTools checks tool definitions and a bad tool-call argument string
// are handled.
func TestParamsTools(t *testing.T) {
	a := &anthropicAI{model: "claude"}
	p := a.params(ai.Request{
		Prompt: "hi",
		Tools: []ai.Tool{{
			Name:        "read_file",
			Description: "Read a file.",
			Parameters:  map[string]any{"type": "object"},
		}},
	})

	if p.Model != "claude" {
		t.Errorf("model = %q, want claude", p.Model)
	}
	if p.MaxTokens != defaultMaxTokens {
		t.Errorf("max_tokens = %d, want the default %d", p.MaxTokens, defaultMaxTokens)
	}
	if len(p.Tools) != 1 || p.Tools[0].Name != "read_file" {
		t.Fatalf("tools = %+v, want read_file", p.Tools)
	}
	if p.Tools[0].InputSchema["type"] != "object" {
		t.Errorf("input_schema = %v, want the tool schema", p.Tools[0].InputSchema)
	}
}

// TestSend checks a non-streaming exchange: text, tool use, usage and headers.
func TestSend(t *testing.T) {
	var gotKey, gotVersion string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotKey = r.Header.Get("x-api-key")
		gotVersion = r.Header.Get("anthropic-version")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"id": "msg_1",
			"content": [
				{"type": "text", "text": "Hello"},
				{"type": "tool_use", "id": "toolu_1", "name": "read_file", "input": {"path": "a.txt"}}
			],
			"stop_reason": "tool_use",
			"usage": {"input_tokens": 5, "output_tokens": 7}
		}`))
	}))
	defer srv.Close()

	p, err := NewAnthropicAI(&config.ApiModelInterfaceDetails{BaseUrl: srv.URL, ApiKey: "k", ModelName: "claude"})
	if err != nil {
		t.Fatalf("NewAnthropicAI: %v", err)
	}
	resp, err := p.Send(context.Background(), ai.Request{Prompt: "hi"})
	if err != nil {
		t.Fatalf("Send: %v", err)
	}

	if gotKey != "k" {
		t.Errorf("x-api-key = %q, want k", gotKey)
	}
	if gotVersion != anthropicVersion {
		t.Errorf("anthropic-version = %q, want %q", gotVersion, anthropicVersion)
	}
	if resp.Text != "Hello" {
		t.Errorf("Text = %q, want Hello", resp.Text)
	}
	if resp.Usage.InputTokens != 5 || resp.Usage.OutputTokens != 7 {
		t.Errorf("Usage = %+v, want 5/7", resp.Usage)
	}
	if len(resp.Messages) != 1 || resp.Messages[0].Name != "read_file" || resp.Messages[0].ToolCallID != "toolu_1" {
		t.Fatalf("Messages = %+v, want one read_file call", resp.Messages)
	}
	if resp.Messages[0].ToolArguments != `{"path": "a.txt"}` {
		t.Errorf("ToolArguments = %q", resp.Messages[0].ToolArguments)
	}
}

// TestSendStream checks streaming: text deltas are forwarded, and a tool call
// split across input_json_delta events is reassembled.
func TestSendStream(t *testing.T) {
	const sse = `event: message_start
data: {"type":"message_start","message":{"usage":{"input_tokens":5,"output_tokens":0}}}

event: content_block_start
data: {"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}

event: content_block_delta
data: {"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"Hel"}}

event: content_block_delta
data: {"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"lo"}}

event: content_block_stop
data: {"type":"content_block_stop","index":0}

event: content_block_start
data: {"type":"content_block_start","index":1,"content_block":{"type":"tool_use","id":"toolu_1","name":"read_file"}}

event: content_block_delta
data: {"type":"content_block_delta","index":1,"delta":{"type":"input_json_delta","partial_json":"{\"path\":"}}

event: content_block_delta
data: {"type":"content_block_delta","index":1,"delta":{"type":"input_json_delta","partial_json":"\"a.txt\"}"}}

event: content_block_stop
data: {"type":"content_block_stop","index":1}

event: message_delta
data: {"type":"message_delta","delta":{"stop_reason":"tool_use"},"usage":{"output_tokens":7}}

event: message_stop
data: {"type":"message_stop"}
`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte(sse))
	}))
	defer srv.Close()

	p, err := NewAnthropicAI(&config.ApiModelInterfaceDetails{BaseUrl: srv.URL, ApiKey: "k", ModelName: "claude"})
	if err != nil {
		t.Fatalf("NewAnthropicAI: %v", err)
	}

	var streamed strings.Builder
	resp, err := p.SendStream(context.Background(), ai.Request{Prompt: "hi"}, func(c ai.Chunk) error {
		streamed.WriteString(c.Delta)
		return nil
	})
	if err != nil {
		t.Fatalf("SendStream: %v", err)
	}

	if streamed.String() != "Hello" || resp.Text != "Hello" {
		t.Errorf("streamed = %q, resp.Text = %q, want Hello", streamed.String(), resp.Text)
	}
	if resp.Usage.InputTokens != 5 || resp.Usage.OutputTokens != 7 {
		t.Errorf("Usage = %+v, want 5/7", resp.Usage)
	}
	if len(resp.Messages) != 1 {
		t.Fatalf("Messages = %+v, want one tool call", resp.Messages)
	}
	m := resp.Messages[0]
	if m.Name != "read_file" || m.ToolCallID != "toolu_1" {
		t.Errorf("tool call = %+v, want read_file/toolu_1", m)
	}
	if !json.Valid([]byte(m.ToolArguments)) || m.ToolArguments != `{"path":"a.txt"}` {
		t.Errorf("ToolArguments = %q, want the reassembled JSON", m.ToolArguments)
	}
}

// TestSendStreamAborts checks that an error from onChunk stops the stream and
// is returned so the loop sees a deliberate abort.
func TestSendStreamAborts(t *testing.T) {
	const sse = `event: content_block_delta
data: {"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"x"}}
`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(sse))
	}))
	defer srv.Close()

	p, _ := NewAnthropicAI(&config.ApiModelInterfaceDetails{BaseUrl: srv.URL, ApiKey: "k", ModelName: "claude"})
	want := context.Canceled
	_, err := p.SendStream(context.Background(), ai.Request{Prompt: "hi"}, func(c ai.Chunk) error {
		return want
	})
	if err != want {
		t.Errorf("SendStream error = %v, want the onChunk error %v", err, want)
	}
}

// TestListModelsAnthropic checks the Models API listing and headers.
func TestListModelsAnthropic(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("anthropic-version") != anthropicVersion {
			t.Errorf("anthropic-version = %q, want %q", r.Header.Get("anthropic-version"), anthropicVersion)
		}
		_, _ = w.Write([]byte(`{"data":[{"id":"claude-3-5-sonnet","display_name":"Claude 3.5 Sonnet"}]}`))
	}))
	defer srv.Close()

	infos, err := ListModelsAnthropic(srv.URL, "k")
	if err != nil {
		t.Fatalf("ListModelsAnthropic: %v", err)
	}
	if len(infos) != 1 || infos[0].ID != "claude-3-5-sonnet" {
		t.Fatalf("infos = %+v, want claude-3-5-sonnet", infos)
	}
	if infos[0].Description != "Claude 3.5 Sonnet" {
		t.Errorf("Description = %q", infos[0].Description)
	}
}
