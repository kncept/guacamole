package ai

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/kncept/guacamole/userinput"
)

// recordingProvider captures every request it receives and replies with a
// per-call counter, so tests can see exactly what a Session sent.
type recordingProvider struct {
	reqs  []Request
	calls int
	fail  error
}

func (r *recordingProvider) Send(ctx context.Context, req Request) (Response, error) {
	r.calls++
	r.reqs = append(r.reqs, req)
	if r.fail != nil {
		return Response{}, r.fail
	}
	return Response{Text: fmt.Sprintf("reply %d", r.calls), Finished: true}, nil
}

func (r *recordingProvider) SendStream(ctx context.Context, req Request, onChunk func(Chunk) error) (Response, error) {
	resp, err := r.Send(ctx, req)
	if err != nil {
		return Response{}, err
	}
	if onChunk != nil {
		if err := onChunk(Chunk{Delta: resp.Text, Finished: true}); err != nil {
			return Response{}, err
		}
	}
	return resp, nil
}

func TestSessionAccumulatesHistory(t *testing.T) {
	provider := &recordingProvider{}
	session := NewSession(NewLoop(provider))
	session.Model = "m1"
	session.SystemPrompt = "be brief"

	if _, err := session.Say(context.Background(), "one"); err != nil {
		t.Fatalf("Say: %v", err)
	}
	resp, err := session.Say(context.Background(), "two")
	if err != nil {
		t.Fatalf("Say: %v", err)
	}
	if resp.Text != "reply 2" {
		t.Errorf("Text = %q, want %q", resp.Text, "reply 2")
	}

	want := []Message{
		{Role: RoleUser, Content: "one"},
		{Role: RoleAssistant, Content: "reply 1"},
		{Role: RoleUser, Content: "two"},
		{Role: RoleAssistant, Content: "reply 2"},
	}
	got := session.Messages()
	if len(got) != len(want) {
		t.Fatalf("Messages() = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("Messages()[%d] = %+v, want %+v", i, got[i], want[i])
		}
	}

	// The second request must carry the whole conversation up to that
	// point, plus the session's model and system prompt.
	if len(provider.reqs) != 2 {
		t.Fatalf("provider calls = %d, want 2", len(provider.reqs))
	}
	sent := provider.reqs[1]
	if len(sent.Messages) != 3 {
		t.Fatalf("request Messages = %v, want 3 entries", sent.Messages)
	}
	for i, m := range want[:3] {
		if sent.Messages[i] != m {
			t.Errorf("request Messages[%d] = %+v, want %+v", i, sent.Messages[i], m)
		}
	}
	if sent.Model != "m1" || sent.SystemPrompt != "be brief" {
		t.Errorf("Model = %q, SystemPrompt = %q; want %q, %q", sent.Model, sent.SystemPrompt, "m1", "be brief")
	}
}

func TestSessionFailedSayKeepsHistory(t *testing.T) {
	provider := &recordingProvider{fail: errors.New("boom")}
	loop := NewLoop(provider)
	loop.Retries = 0
	session := NewSession(loop)

	if _, err := session.Say(context.Background(), "one"); err == nil {
		t.Fatal("Say: expected error, got nil")
	}
	if got := session.Messages(); len(got) != 0 {
		t.Errorf("Messages() = %v, want empty after a failed Say", got)
	}

	provider.fail = nil
	if _, err := session.Say(context.Background(), "two"); err != nil {
		t.Fatalf("Say: %v", err)
	}
	// The failed turn left no trace: the new request is a lone message.
	sent := provider.reqs[len(provider.reqs)-1]
	if len(sent.Messages) != 1 || sent.Messages[0].Content != "two" {
		t.Errorf("request Messages = %v, want just %q", sent.Messages, "two")
	}
}

func TestSessionClear(t *testing.T) {
	provider := &recordingProvider{}
	session := NewSession(NewLoop(provider))

	if _, err := session.Say(context.Background(), "one"); err != nil {
		t.Fatalf("Say: %v", err)
	}
	session.Clear()
	if got := session.Messages(); len(got) != 0 {
		t.Errorf("Messages() = %v, want empty after Clear", got)
	}

	if _, err := session.Say(context.Background(), "two"); err != nil {
		t.Fatalf("Say: %v", err)
	}
	sent := provider.reqs[len(provider.reqs)-1]
	if len(sent.Messages) != 1 || sent.Messages[0].Content != "two" {
		t.Errorf("request Messages = %v, want just %q", sent.Messages, "two")
	}
}

// scriptedProvider plays back a fixed list of responses, recording the
// requests it receives.
type scriptedProvider struct {
	responses []Response
	reqs      []Request
}

func (s *scriptedProvider) Send(ctx context.Context, req Request) (Response, error) {
	s.reqs = append(s.reqs, req)
	if len(s.responses) == 0 {
		return Response{}, errors.New("scriptedProvider: no more responses")
	}
	resp := s.responses[0]
	s.responses = s.responses[1:]
	return resp, nil
}

func (s *scriptedProvider) SendStream(ctx context.Context, req Request, onChunk func(Chunk) error) (Response, error) {
	resp, err := s.Send(ctx, req)
	if err != nil {
		return Response{}, err
	}
	if onChunk != nil && resp.Text != "" {
		if err := onChunk(Chunk{Delta: resp.Text, Finished: true}); err != nil {
			return Response{}, err
		}
	}
	return resp, nil
}

func TestSessionToolUse(t *testing.T) {
	provider := &scriptedProvider{responses: []Response{
		{Messages: []Message{
			{Role: RoleAssistant, Name: "echo", ToolCallID: "call-1", ToolArguments: `{"text":"hi"}`},
		}, Finished: true},
		{Text: "done", Finished: true},
	}}

	var gotArgs string
	echo := Tool{
		Name: "echo",
		Handler: func(ctx context.Context, args json.RawMessage) (string, error) {
			gotArgs = string(args)
			return "tool output", nil
		},
	}

	session := NewSession(NewLoop(provider))
	session.Tools = []Tool{echo}

	resp, err := session.Say(context.Background(), "go")
	if err != nil {
		t.Fatalf("Say: %v", err)
	}
	if resp.Text != "done" {
		t.Errorf("Text = %q, want %q", resp.Text, "done")
	}
	if gotArgs != `{"text":"hi"}` {
		t.Errorf("handler args = %q, want %q", gotArgs, `{"text":"hi"}`)
	}

	want := []Message{
		{Role: RoleUser, Content: "go"},
		{Role: RoleAssistant, Name: "echo", ToolCallID: "call-1", ToolArguments: `{"text":"hi"}`},
		{Role: RoleTool, Name: "echo", ToolCallID: "call-1", Content: "tool output"},
		{Role: RoleAssistant, Content: "done"},
	}
	got := session.Messages()
	if len(got) != len(want) {
		t.Fatalf("Messages() = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("Messages()[%d] = %+v, want %+v", i, got[i], want[i])
		}
	}

	// Round two must carry the whole exchange, plus the session's tools.
	if len(provider.reqs) != 2 {
		t.Fatalf("provider calls = %d, want 2", len(provider.reqs))
	}
	sent := provider.reqs[1]
	if len(sent.Messages) != 3 {
		t.Fatalf("round 2 Messages = %v, want 3 entries", sent.Messages)
	}
	for i, m := range want[:3] {
		if sent.Messages[i] != m {
			t.Errorf("round 2 Messages[%d] = %+v, want %+v", i, sent.Messages[i], m)
		}
	}
	if len(sent.Tools) != 1 || sent.Tools[0].Name != "echo" {
		t.Errorf("round 2 Tools = %v, want the echo tool", sent.Tools)
	}
}

func TestSessionUnknownTool(t *testing.T) {
	provider := &scriptedProvider{responses: []Response{
		{Messages: []Message{
			{Role: RoleAssistant, Name: "nosuch", ToolCallID: "call-1", ToolArguments: `{}`},
		}, Finished: true},
		{Text: "recovered", Finished: true},
	}}

	session := NewSession(NewLoop(provider))
	resp, err := session.Say(context.Background(), "go")
	if err != nil {
		t.Fatalf("Say: %v", err)
	}
	if resp.Text != "recovered" {
		t.Errorf("Text = %q, want %q", resp.Text, "recovered")
	}
	got := session.Messages()
	if len(got) != 4 || got[2].Role != RoleTool || got[2].Content != `error: unknown tool "nosuch"` {
		t.Errorf("Messages() = %v, want the unknown-tool error as the tool result", got)
	}
}

func TestSessionToolRoundLimit(t *testing.T) {
	call := Response{Messages: []Message{
		{Role: RoleAssistant, Name: "echo", ToolCallID: "call-1", ToolArguments: `{}`},
	}, Finished: true}
	provider := &scriptedProvider{responses: []Response{call, call, call, call}}

	echo := Tool{
		Name:    "echo",
		Handler: func(ctx context.Context, args json.RawMessage) (string, error) { return "out", nil },
	}

	session := NewSession(NewLoop(provider))
	session.Tools = []Tool{echo}
	session.MaxToolRounds = 2

	if _, err := session.Say(context.Background(), "go"); err == nil {
		t.Fatal("Say: expected a tool round limit error, got nil")
	}
	// Every completed round is still recorded, including the call that
	// tripped the limit: user + 3 calls + 2 results.
	if got := len(session.Messages()); got != 6 {
		t.Errorf("len(Messages()) = %d, want 6", got)
	}
}

// toolCallResponse builds a scripted response that requests one echo call.
func toolCallResponse() Response {
	return Response{Messages: []Message{
		{Role: RoleAssistant, Name: "echo", ToolCallID: "call-1", ToolArguments: `{}`},
	}, Finished: true}
}

// scriptedInput answers Prompt with scripted responses ("" when exhausted),
// recording every prompt it was shown.
type scriptedInput struct {
	answers []string
	prompts []string
	options [][]string
}

var _ userinput.RequestForegroundUserInput = (*scriptedInput)(nil)

func (s *scriptedInput) Prompt(text string, allowFree bool, options []string) (string, error) {
	s.prompts = append(s.prompts, text)
	s.options = append(s.options, options)
	if len(s.answers) == 0 {
		return "", nil
	}
	answer := s.answers[0]
	s.answers = s.answers[1:]
	return answer, nil
}

func TestSessionToolRoundLimitResumeByUser(t *testing.T) {
	provider := &scriptedProvider{responses: []Response{
		toolCallResponse(), toolCallResponse(), toolCallResponse(), toolCallResponse(),
		{Text: "done", Finished: true},
	}}

	echo := Tool{
		Name:    "echo",
		Handler: func(ctx context.Context, args json.RawMessage) (string, error) { return "out", nil },
	}

	session := NewSession(NewLoop(provider))
	session.Tools = []Tool{echo}
	session.MaxToolRounds = 2
	// The user keeps granting extensions, so the model is allowed to work
	// past the limit until it finally answers.
	input := &scriptedInput{answers: []string{"Continue"}}
	session.UserInput = input

	resp, err := session.Say(context.Background(), "go")
	if err != nil {
		t.Fatalf("Say: %v", err)
	}
	if resp.Text != "done" {
		t.Errorf("Text = %q, want %q", resp.Text, "done")
	}
	// user + 4 tool calls + 4 results + the final answer.
	if got := len(session.Messages()); got != 10 {
		t.Errorf("len(Messages()) = %d, want 10", got)
	}
	if len(input.prompts) != 1 {
		t.Fatalf("user was prompted %d times, want 1", len(input.prompts))
	}
	if want := []string{"Continue", "Abort"}; !reflect.DeepEqual(input.options[0], want) {
		t.Errorf("options = %v, want %v", input.options[0], want)
	}
}

func TestSessionToolRoundLimitAbortAfterResume(t *testing.T) {
	provider := &scriptedProvider{responses: []Response{
		toolCallResponse(), toolCallResponse(), toolCallResponse(),
		toolCallResponse(), toolCallResponse(), toolCallResponse(),
		{Text: "done", Finished: true},
	}}

	echo := Tool{
		Name:    "echo",
		Handler: func(ctx context.Context, args json.RawMessage) (string, error) { return "out", nil },
	}

	session := NewSession(NewLoop(provider))
	session.Tools = []Tool{echo}
	session.MaxToolRounds = 2
	// The user grants exactly one extension, then aborts on the next limit.
	input := &scriptedInput{answers: []string{"Continue", "Abort"}}
	session.UserInput = input

	if _, err := session.Say(context.Background(), "go"); err == nil {
		t.Fatal("Say: expected a tool round limit error, got nil")
	}
	if len(input.prompts) != 2 {
		t.Fatalf("user was prompted %d times, want 2", len(input.prompts))
	}
	// Both prompts ask about the same completed round count.
	for i, text := range input.prompts {
		if want := "The model has used 2 rounds of tool calls and is requesting more"; text != want {
			t.Errorf("prompt[%d] = %q, want %q", i, text, want)
		}
	}
	// The granted extension ran three more tool rounds, then the second
	// limit aborted: user + 5 tool calls + 4 results + the 6th call.
	if got := len(session.Messages()); got != 12 {
		t.Errorf("len(Messages()) = %d, want 12", got)
	}
}

// rateLimitProvider serves a RateLimitError for the first n Send calls,
// then a normal reply.
type rateLimitProvider struct {
	fail       int
	retryAfter time.Duration
}

func (r *rateLimitProvider) Send(ctx context.Context, req Request) (Response, error) {
	if r.fail > 0 {
		r.fail--
		return Response{}, &RateLimitError{Err: errors.New("HTTP 429"), RetryAfter: r.retryAfter}
	}
	return Response{Text: "done", Finished: true}, nil
}

func (r *rateLimitProvider) SendStream(ctx context.Context, req Request, onChunk func(Chunk) error) (Response, error) {
	resp, err := r.Send(ctx, req)
	if err != nil {
		return Response{}, err
	}
	if onChunk != nil && resp.Text != "" {
		if err := onChunk(Chunk{Delta: resp.Text, Finished: true}); err != nil {
			return Response{}, err
		}
	}
	return resp, nil
}

func TestSessionRateLimitRetryByUser(t *testing.T) {
	// The loop's own retry budget is spent: a single 429 escapes Run, and
	// the session hands the decision to the user.
	provider := &rateLimitProvider{fail: 1, retryAfter: time.Millisecond}
	loop := NewLoop(provider)
	loop.Retries = 0
	session := NewSession(loop)
	input := &scriptedInput{answers: []string{"Retry"}}
	session.UserInput = input

	resp, err := session.Say(context.Background(), "go")
	if err != nil {
		t.Fatalf("Say: %v", err)
	}
	if resp.Text != "done" {
		t.Errorf("Text = %q, want %q", resp.Text, "done")
	}
	if len(input.prompts) != 1 {
		t.Fatalf("user was prompted %d times, want 1", len(input.prompts))
	}
	if want := []string{"Retry", "Abort"}; !reflect.DeepEqual(input.options[0], want) {
		t.Errorf("options = %v, want %v", input.options[0], want)
	}
	if !strings.Contains(input.prompts[0], "rate limited (HTTP 429)") {
		t.Errorf("prompt = %q, want a rate limit question", input.prompts[0])
	}
}

func TestSessionRateLimitAbortByUser(t *testing.T) {
	provider := &rateLimitProvider{fail: 10, retryAfter: time.Millisecond}
	loop := NewLoop(provider)
	loop.Retries = 0
	session := NewSession(loop)
	input := &scriptedInput{answers: []string{"Abort"}}
	session.UserInput = input

	if _, err := session.Say(context.Background(), "go"); err == nil {
		t.Fatal("Say: expected a rate limit error, got nil")
	}
	if len(input.prompts) != 1 {
		t.Fatalf("user was prompted %d times, want 1", len(input.prompts))
	}
	// The aborted turn left no trace in the conversation.
	if got := session.Messages(); len(got) != 0 {
		t.Errorf("Messages() = %v, want empty", got)
	}
}

func TestSessionRateLimitWithoutInputStops(t *testing.T) {
	provider := &rateLimitProvider{fail: 10, retryAfter: time.Millisecond}
	loop := NewLoop(provider)
	loop.Retries = 0
	session := NewSession(loop)

	_, err := session.Say(context.Background(), "go")
	var rateLimited *RateLimitError
	if !errors.As(err, &rateLimited) {
		t.Fatalf("Say error = %v, want a RateLimitError to pass through", err)
	}
}

func TestNewSessionGeneratesIDs(t *testing.T) {
	a := NewSession(nil)
	b := NewSession(nil)
	if a.ID == "" || b.ID == "" {
		t.Fatalf("IDs must not be empty: %q, %q", a.ID, b.ID)
	}
	if a.ID == b.ID {
		t.Errorf("IDs should be unique, both were %q", a.ID)
	}
}

func TestNewSessionWithHistory(t *testing.T) {
	history := []Message{
		{Role: RoleUser, Content: "one"},
		{Role: RoleAssistant, Content: "two"},
	}
	session := NewSession(nil, history...)

	// Mutating the seed must not affect the session.
	history[0].Content = "tampered"
	got := session.Messages()
	if len(got) != 2 || got[0].Content != "one" || got[1].Content != "two" {
		t.Errorf("Messages() = %v, want the seeded history", got)
	}
}

func TestSessionMessagesReturnsCopy(t *testing.T) {
	provider := &recordingProvider{}
	session := NewSession(NewLoop(provider))

	if _, err := session.Say(context.Background(), "one"); err != nil {
		t.Fatalf("Say: %v", err)
	}
	session.Messages()[0] = Message{Role: RoleSystem, Content: "tampered"}

	got := session.Messages()
	if got[0] != (Message{Role: RoleUser, Content: "one"}) {
		t.Errorf("Messages()[0] = %+v, want the original message", got[0])
	}
}
