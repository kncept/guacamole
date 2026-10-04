// Package restfulai implements ai.Provider for OpenAI-compatible REST
// APIs, using the openai-go client's Responses API.
package restfulai

import (
	"context"

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
	client *openai.Client
	model  string
}

// NewRestfulAI builds a Provider from API connection details.
func NewRestfulAI(conf *config.ApiModelInterfaceDetails) RestfulAI {
	client := openai.NewClient(
		openaiOption.WithBaseURL(conf.BaseUrl),
		openaiOption.WithAPIKey(conf.ApiKey), // defaults to os.LookupEnv("OPENAI_API_KEY")
	)

	return &restfulAI{
		client: &client,
		model:  conf.ModelName,
	}
}

// Send performs one non-streaming exchange.
func (this *restfulAI) Send(ctx context.Context, req ai.Request) (ai.Response, error) {
	raw, err := this.client.Responses.New(ctx, this.params(req))
	if err != nil {
		return ai.Response{}, err
	}
	return toResponse(raw), nil
}

// SendStream performs a streaming exchange, calling onChunk for every text
// delta as it arrives. A non-nil return from onChunk aborts the stream.
func (this *restfulAI) SendStream(ctx context.Context, req ai.Request, onChunk func(ai.Chunk) error) (ai.Response, error) {
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
		return ai.Response{}, err
	}

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
	}
	return resp, nil
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
