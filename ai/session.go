package ai

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/kncept/guacamole/userinput"
)

// DefaultMaxToolRounds is the default bound on tool-call rounds in one Say.
const DefaultMaxToolRounds = 10

// Session is a stateful, multi-turn conversation built on top of a Loop.
//
// Where a Loop runs one request/processing/response cycle, a Session owns
// the history of a whole conversation: every Say appends the user's message
// to the history, sends the full conversation through the loop, and records
// the assistant's reply, so the model can see everything said so far.
//
// Every session has a stable ID, so it can be saved and resumed later.
//
// A Session is not safe for concurrent use: a conversation is ordered, so
// calls to Say must come one at a time.
type Session struct {
	// ID identifies the session. NewSession generates a random one.
	ID string

	// Loop runs each exchange. Required.
	Loop *Loop

	// Model names the model to use; an empty Model lets the provider fall
	// back to its own default. Copied onto every request.
	Model string

	// SystemPrompt is sent as the provider's system/developer instructions
	// on every request, kept separate from the conversation history.
	SystemPrompt string

	// Temperature, when set, is forwarded to the provider on every request.
	Temperature *float64

	// MaxTokens, when set, is forwarded to the provider on every request.
	MaxTokens *int

	// Tools lists the functions available to the model. Copied onto every
	// request.
	Tools []Tool

	// MaxToolRounds bounds how many rounds of tool calls one Say executes
	// before giving up. Zero uses DefaultMaxToolRounds.
	MaxToolRounds int

	// UserInput asks the user for a decision when a Say reaches
	// MaxToolRounds and the model still requests tool calls: it is prompted
	// to keep going or abort the turn. When nil, Say always stops with the
	// round-limit error.
	UserInput userinput.RequestForegroundUserInput

	// Magi, when set, switches the session to Magi Mode: each turn runs the
	// Magi panel's two-phase turn (inference, voting, collation) instead of
	// this session's single Loop, and only the highest-scoring response is
	// kept and executed. Loop is still used to report the winning response
	// to observers (its OnChunk and OnResponse hooks fire), so a UI keeps
	// working unchanged.
	Magi *Magi

	messages []Message
}

// NewSession starts a Session with a fresh random ID that runs each
// exchange through loop. Any history messages seed the conversation, as if
// those turns had already happened — use them to resume a saved session.
func NewSession(loop *Loop, history ...Message) *Session {
	messages := make([]Message, len(history))
	copy(messages, history)
	return &Session{
		ID:       newSessionID(),
		Loop:     loop,
		messages: messages,

		SystemPrompt:  DevAssistant,
		MaxToolRounds: 30, // default is 10
	}
}

// newSessionID returns 16 random hex characters.
func newSessionID() string {
	var b [8]byte
	if _, err := rand.Read(b[:]); err != nil {
		// crypto/rand should never fail; a timestamp still makes a usable ID.
		return strconv.FormatInt(time.Now().UnixNano(), 36)
	}
	return hex.EncodeToString(b[:])
}

// Say adds prompt to the history as a user message, runs the whole
// conversation through the loop, and records the assistant's reply.
//
// If the reply requests tool calls, Say executes them, appends the calls
// and their results to the history, and runs the conversation again —
// repeating until the model answers without tool calls, up to MaxToolRounds
// rounds. When that limit is hit, UserInput (if set) lets the user decide
// whether to keep going or stop; without it Say aborts with an error.
//
// When the provider rate limits the request (HTTP 429), UserInput (if set)
// is asked whether to retry; a retry runs the request again with a fresh
// retry budget, in the same way a "Continue" answer resets the tool-round
// count. Without a UserInput, or if the user aborts, the rate-limit error
// is returned.
//
// If a run fails, the history keeps everything up to the last successful
// round, so a failed turn can be retried (or abandoned) without corrupting
// the conversation.
func (this *Session) Say(ctx context.Context, prompt string) (Response, error) {
	history := make([]Message, 0, len(this.messages)+2)
	history = append(history, this.messages...)
	history = append(history, Message{Role: RoleUser, Content: prompt})

	maxRounds := this.MaxToolRounds
	if maxRounds <= 0 {
		maxRounds = DefaultMaxToolRounds
	}

	for round := 0; ; round++ {
		resp, err := this.run(ctx, Request{
			Model:        this.Model,
			Messages:     history,
			SystemPrompt: this.SystemPrompt,
			Temperature:  this.Temperature,
			MaxTokens:    this.MaxTokens,
			Tools:        this.Tools,
		})
		if err != nil {
			return Response{}, err
		}

		if resp.Text != "" || len(resp.Messages) == 0 {
			history = append(history, Message{Role: RoleAssistant, Content: resp.Text})
		}
		// Structured output items (e.g. tool calls) are part of the reply.
		history = append(history, resp.Messages...)
		this.messages = history

		calls := toolCalls(resp.Messages)
		if len(calls) == 0 {
			return resp, nil
		}
		if round >= maxRounds {
			// The limit was reached but the model still wants tools. By
			// default Say gives up; UserInput hands the decision to the
			// user instead. A "Continue" answer resets the round count and
			// keeps going, any other answer (or a prompt error) aborts
			// with the usual round-limit error.
			if this.UserInput == nil {
				return resp, fmt.Errorf("ai: stopped after %d tool rounds", maxRounds)
			}
			answer, err := this.UserInput.Prompt(
				fmt.Sprintf("The model has used %d rounds of tool calls and is requesting more", round),
				false,
				[]string{"Continue", "Abort"},
			)
			if err != nil || answer != "Continue" {
				return resp, fmt.Errorf("ai: stopped after %d tool rounds", maxRounds)
			}
			round = -1 // continue: the loop's round++ makes the next round 0
		}
		for _, call := range calls {
			history = append(history, this.execute(ctx, call))
		}
		this.messages = history
	}
}

// run sends one request through the session's turn runner — the Magi panel
// in Magi Mode, otherwise the single Loop. If the provider rate limits the
// request (HTTP 429) and a UserInput is configured, the user is asked whether
// to retry: "Retry" sends the request again with a fresh retry budget (the
// loop's own retries were already spent), "Abort" (or a nil UserInput, or a
// failed prompt) returns the rate-limit error.
func (this *Session) run(ctx context.Context, req Request) (Response, error) {
	for {
		var (
			resp Response
			err  error
		)
		if this.Magi != nil {
			resp, err = this.Magi.Run(ctx, req)
		} else {
			resp, err = this.Loop.Run(ctx, req)
		}
		if err == nil {
			if this.Magi != nil {
				this.emitMagi(resp)
			}
			return resp, nil
		}
		var rateLimited *RateLimitError
		if !errors.As(err, &rateLimited) || this.UserInput == nil {
			return resp, err
		}

		text := "The request was rate limited (HTTP 429). Retry?"
		if rateLimited.RetryAfter > 0 {
			text = fmt.Sprintf("The request was rate limited (HTTP 429). Wait %s, then retry?", rateLimited.RetryAfter)
		}
		answer, perr := this.UserInput.Prompt(text, false, []string{"Retry", "Abort"})
		if perr != nil || answer != "Retry" {
			return resp, err
		}
		if rateLimited.RetryAfter > 0 {
			select {
			case <-ctx.Done():
				return resp, ctx.Err()
			case <-time.After(rateLimited.RetryAfter):
			}
		}
	}
}

// emitMagi reports a Magi turn's winning response to the session's observers
// through the Loop's hooks. The magi do not stream, so the winning text is
// delivered as a single chunk; it is emitted before OnResponse, matching the
// order a streaming provider uses.
func (this *Session) emitMagi(resp Response) {
	if this.Loop == nil {
		return
	}
	if this.Loop.OnChunk != nil && resp.Text != "" {
		_ = this.Loop.OnChunk(Chunk{Delta: resp.Text, Finished: true})
	}
	if this.Loop.OnResponse != nil {
		this.Loop.OnResponse(resp)
	}
}

// toolCalls extracts the tool calls requested in messages.
func toolCalls(messages []Message) []Message {
	var calls []Message
	for _, m := range messages {
		if m.Role == RoleAssistant && m.Name != "" {
			calls = append(calls, m)
		}
	}
	return calls
}

// execute runs one tool call and builds the tool result message. Handler
// errors become the result content, so the model can see and recover from
// them instead of the turn failing.
func (this *Session) execute(ctx context.Context, call Message) Message {
	result := Message{Role: RoleTool, Name: call.Name, ToolCallID: call.ToolCallID}

	var tool *Tool
	for i := range this.Tools {
		if this.Tools[i].Name == call.Name {
			tool = &this.Tools[i]
			break
		}
	}
	if tool == nil {
		result.Content = fmt.Sprintf("error: unknown tool %q", call.Name)
		return result
	}

	out, err := tool.Handler(ctx, json.RawMessage(call.ToolArguments))
	if err != nil {
		result.Content = "error: " + err.Error()
	} else {
		result.Content = out
	}
	return result
}

// Messages returns a copy of the conversation so far, oldest first.
func (this *Session) Messages() []Message {
	messages := make([]Message, len(this.messages))
	copy(messages, this.messages)
	return messages
}

// Clear discards the history, returning the session to a fresh state. The
// session keeps its ID.
func (this *Session) Clear() {
	this.messages = nil
}
