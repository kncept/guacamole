package ai

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"strconv"
	"time"
)

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
// If the run fails the history is left unchanged, so a failed turn can be
// retried (or abandoned) without corrupting the conversation.
func (this *Session) Say(ctx context.Context, prompt string) (Response, error) {
	history := make([]Message, 0, len(this.messages)+2)
	history = append(history, this.messages...)
	history = append(history, Message{Role: RoleUser, Content: prompt})

	resp, err := this.Loop.Run(ctx, Request{
		Model:        this.Model,
		Messages:     history,
		SystemPrompt: this.SystemPrompt,
		Temperature:  this.Temperature,
		MaxTokens:    this.MaxTokens,
	})
	if err != nil {
		return Response{}, err
	}

	if resp.Text != "" || len(resp.Messages) == 0 {
		history = append(history, Message{Role: RoleAssistant, Content: resp.Text})
	}
	// Structured output items (e.g. tool calls) are part of the reply too.
	this.messages = append(history, resp.Messages...)
	return resp, nil
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
