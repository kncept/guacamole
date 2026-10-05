package restfulai

import (
	"testing"

	"github.com/kncept/guacamole/ai"
	"github.com/kncept/guacamole/config"
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
