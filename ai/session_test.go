package ai

import (
	"context"
	"errors"
	"fmt"
	"testing"
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
