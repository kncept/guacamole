package restfulai

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/kncept/guacamole/ai"
	"github.com/kncept/guacamole/config"
	"github.com/openai/openai-go/v3"
	openaiOption "github.com/openai/openai-go/v3/option"
)

func testProvider() *restfulAI {
	p, err := NewRestfulAI(&config.ApiModelInterfaceDetails{
		BaseUrl:   "http://localhost:0",
		ApiKey:    "test",
		ModelName: "m1",
	})
	if err != nil {
		panic(err)
	}
	return p.(*restfulAI)
}

func TestParamsTools(t *testing.T) {
	p := testProvider().params(ai.Request{
		Prompt: "hi",
		Tools: []ai.Tool{{
			Name:        "read_file",
			Description: "Read a file.",
			Parameters:  map[string]any{"type": "object"},
		}},
	})

	if len(p.Tools) != 1 {
		t.Fatalf("Tools = %v, want 1 entry", p.Tools)
	}
	fn := p.Tools[0].OfFunction
	if fn == nil {
		t.Fatal("Tools[0].OfFunction is nil")
	}
	if fn.Name != "read_file" {
		t.Errorf("Name = %q, want %q", fn.Name, "read_file")
	}
	if !fn.Description.Valid() || fn.Description.Value != "Read a file." {
		t.Errorf("Description = %v, want %q", fn.Description, "Read a file.")
	}
	if fn.Parameters["type"] != "object" {
		t.Errorf("Parameters = %v, want the JSON schema", fn.Parameters)
	}
}

func TestParamsToolMessages(t *testing.T) {
	p := testProvider().params(ai.Request{
		Messages: []ai.Message{
			{Role: ai.RoleUser, Content: "go"},
			{Role: ai.RoleAssistant, Name: "read_file", ToolCallID: "call-1", ToolArguments: `{"path":"a.txt"}`},
			{Role: ai.RoleTool, Name: "read_file", ToolCallID: "call-1", Content: "contents"},
			{Role: ai.RoleAssistant, Content: "done"},
		},
	})

	items := p.Input.OfInputItemList
	if len(items) != 4 {
		t.Fatalf("input items = %d, want 4", len(items))
	}

	if items[0].OfMessage == nil || items[0].OfMessage.Role != "user" {
		t.Errorf("items[0] = %+v, want a user message", items[0])
	}

	call := items[1].OfFunctionCall
	if call == nil {
		t.Fatal("items[1].OfFunctionCall is nil")
	}
	if call.CallID != "call-1" || call.Name != "read_file" || call.Arguments != `{"path":"a.txt"}` {
		t.Errorf("function call = %+v", call)
	}

	output := items[2].OfFunctionCallOutput
	if output == nil {
		t.Fatal("items[2].OfFunctionCallOutput is nil")
	}
	if !output.CallID.Valid() || output.CallID.Value != "call-1" {
		t.Errorf("function call output CallID = %v, want %q", output.CallID, "call-1")
	}

	if items[3].OfMessage == nil || items[3].OfMessage.Role != "assistant" {
		t.Errorf("items[3] = %+v, want an assistant message", items[3])
	}
}

// TestListModelsOpenCodeStandardShape checks that the standard OpenAI listing
// shape returned by https://opencode.ai/zen/v1/models is parsed, and that
// "-free" models are marked free.
func TestListModelsOpenCodeStandardShape(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/models" {
			t.Errorf("path = %q, want %q", r.URL.Path, "/models")
		}
		if got := r.Header.Get("Authorization"); got != "Bearer secret" {
			t.Errorf("Authorization = %q, want %q", got, "Bearer secret")
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"object":"list","data":[
			{"id":"claude-opus-5","object":"model","created":1791634803,"owned_by":"opencode"},
			{"id":"ling-3.1-flash-free","object":"model","created":1791634803,"owned_by":"opencode"}
		]}`))
	}))
	defer srv.Close()

	infos, err := ListModelsOpenCode(srv.URL, "secret")
	if err != nil {
		t.Fatalf("ListModelsOpenCode: %v", err)
	}
	if len(infos) != 2 {
		t.Fatalf("got %d models, want 2: %+v", len(infos), infos)
	}
	if infos[0].ID != "claude-opus-5" || infos[0].IsFree {
		t.Errorf("infos[0] = %+v, want claude-opus-5 not free", infos[0])
	}
	if infos[1].ID != "ling-3.1-flash-free" || !infos[1].IsFree {
		t.Errorf("infos[1] = %+v, want ling-3.1-flash-free marked free", infos[1])
	}
}

// TestSendRateLimited checks that an HTTP 429 reply becomes an
// ai.RateLimitError carrying the Retry-After wait.
func TestSendRateLimited(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Retry-After", "30")
		w.WriteHeader(http.StatusTooManyRequests)
		_, _ = w.Write([]byte(`{"error":{"code":"rate_limit_exceeded","message":"slow down","param":"","type":"rate_limit_error"}}`))
	}))
	defer srv.Close()

	// Disable the client's own retries so one 429 reaches wrapErr directly.
	client := openai.NewClient(
		openaiOption.WithBaseURL(srv.URL),
		openaiOption.WithAPIKey("k"),
		openaiOption.WithMaxRetries(0),
	)
	p := &restfulAI{client: &client, baseURL: srv.URL, model: "m1"}

	_, err := p.Send(context.Background(), ai.Request{Prompt: "hi"})
	var rateLimited *ai.RateLimitError
	if !errors.As(err, &rateLimited) {
		t.Fatalf("Send error = %v, want a *ai.RateLimitError", err)
	}
	if rateLimited.RetryAfter != 30*time.Second {
		t.Errorf("RetryAfter = %v, want 30s", rateLimited.RetryAfter)
	}
}

// TestListModelsAppAttribution checks that app attribution headers are sent
// when enabled, and that only the standard headers are sent when disabled.
func TestListModelsAppAttribution(t *testing.T) {
	var got http.Header
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = r.Header.Clone()
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"object":"list","data":[]}`))
	}))
	defer srv.Close()

	if _, err := ListModels(srv.URL, "k", true); err != nil {
		t.Fatalf("ListModels (enabled): %v", err)
	}
	if v := got.Get("HTTP-Referer"); v != config.AppAttributionReferer {
		t.Errorf("HTTP-Referer = %q, want %q", v, config.AppAttributionReferer)
	}
	if v := got.Get("X-OpenRouter-Title"); v != config.AppAttributionTitle {
		t.Errorf("X-OpenRouter-Title = %q, want %q", v, config.AppAttributionTitle)
	}
	if v := got.Get("X-OpenRouter-Categories"); v != config.AppAttributionCategories {
		t.Errorf("X-OpenRouter-Categories = %q, want %q", v, config.AppAttributionCategories)
	}

	got = nil
	if _, err := ListModels(srv.URL, "k", false); err != nil {
		t.Fatalf("ListModels (disabled): %v", err)
	}
	if v := got.Get("X-OpenRouter-Title"); v != "" {
		t.Errorf("X-OpenRouter-Title = %q, want no attribution header when disabled", v)
	}
}

// TestListModelsOpenCodeLegacyShape checks the legacy {"models":[...]} shape
// is still accepted for older OpenCode deployments.
func TestListModelsOpenCodeLegacyShape(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"models":[{"id":"gpt-oss-120b","size":"120B","free":true,"description":"Big"}]}`))
	}))
	defer srv.Close()

	infos, err := ListModelsOpenCode(srv.URL, "")
	if err != nil {
		t.Fatalf("ListModelsOpenCode: %v", err)
	}
	if len(infos) != 1 {
		t.Fatalf("got %d models, want 1: %+v", len(infos), infos)
	}
	if infos[0].ID != "gpt-oss-120b" || infos[0].Size != "120B" || !infos[0].IsFree || infos[0].Description != "Big" {
		t.Errorf("infos[0] = %+v", infos[0])
	}
}
