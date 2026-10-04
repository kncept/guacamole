// Package ai implements the request/processing/response loop that AI tools
// use to talk to model providers.
//
// Every AI tool, from a single-prompt REPL to a multi-agent system, runs the
// same three-phase cycle:
//
//  1. Request    — a Request is built and handed to a Provider.
//  2. Processing — the Provider streams or returns output and a Processor
//     post-processes the result (validation, extraction, transformation).
//  3. Response   — the finished Response is returned to the caller.
//
// Loop owns the cycle end to end: it sends the request, processes the
// response, and wraps the whole thing in retries with exponential backoff.
// Provider-specific code lives behind the Provider interface; loop policy
// lives in Loop and Processor. Session builds stateful multi-turn
// conversations on top of a Loop, keeping the history and sending it with
// every exchange.
//
// Typical use:
//
//	provider := restfulai.NewRestfulAI(conf)
//	loop := ai.NewLoop(provider)
//	loop.Stream = true
//	loop.OnChunk = func(c ai.Chunk) error { fmt.Print(c.Delta); return nil }
//
//	resp, err := loop.Run(ctx, ai.Request{Prompt: "hello"})
//	if err != nil {
//		// handle
//	}
//	fmt.Println(resp.Text)
package ai

// Role identifies the author of a Message.
type Role string

const (
	RoleSystem    Role = "system"
	RoleUser      Role = "user"
	RoleAssistant Role = "assistant"
	RoleTool      Role = "tool"
)

// Message is one entry in a conversation.
type Message struct {
	Role    Role   `json:"role"`
	Content string `json:"content"`
	// Name identifies the sender for tool messages.
	Name string `json:"name,omitempty"`
}

// Request is everything a Provider needs to call a model once.
type Request struct {
	// Model names the model to use (e.g. "gpt-4o"). An empty Model lets the
	// provider fall back to its own default.
	Model string

	// Prompt is a convenience for a single user message. If set and Messages
	// is empty, it becomes the request input.
	Prompt string

	// Messages is the full conversation. Takes precedence over Prompt.
	Messages []Message

	// SystemPrompt is sent as the provider's system/developer instructions,
	// kept separate from the conversation.
	SystemPrompt string

	// Temperature, when set, is forwarded to the provider.
	Temperature *float64

	// MaxTokens, when set, is forwarded to the provider as the maximum
	// number of output tokens.
	MaxTokens *int

	// Tag is free-form caller metadata carried through the loop. Hooks and
	// processors can use it to correlate logs.
	Tag string
}

// Conversation returns the effective conversation for the request: Messages
// if set, otherwise a single user message built from Prompt.
func (r Request) Conversation() []Message {
	if len(r.Messages) > 0 {
		return r.Messages
	}
	if r.Prompt != "" {
		return []Message{{Role: RoleUser, Content: r.Prompt}}
	}
	return nil
}

// Chunk is a piece of a streaming response.
type Chunk struct {
	// Delta is the new text in this chunk. Empty for metadata-only chunks.
	Delta string

	// Finished marks the final chunk of the stream.
	Finished bool
}

// Usage reports token consumption for a completed exchange.
type Usage struct {
	InputTokens  int
	OutputTokens int
}

// Response is the outcome of one request/processing/response cycle.
type Response struct {
	// Text is the assistant's final text output.
	Text string

	// Messages holds structured output items beyond the plain text, such as
	// tool calls the model requested.
	Messages []Message

	// Usage is the token accounting reported by the provider.
	Usage Usage

	// Finished reports whether the model believes the exchange is complete,
	// as opposed to cut short (e.g. by a token limit).
	Finished bool

	// Raw holds provider-specific data, typically the raw API response, for
	// callers that need more than the normalized fields.
	Raw any
}

// Empty reports whether the response carries no output at all.
func (r Response) Empty() bool {
	return len(r.Text) == 0 && len(r.Messages) == 0
}
