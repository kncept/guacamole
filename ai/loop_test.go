package ai

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

// fakeProvider is a Provider that fails a configurable number of times
// before succeeding.
type fakeProvider struct {
	sends   int
	streams int
	fail    int // number of initial attempts to fail
	chunks  []Chunk
}

func (f *fakeProvider) Send(ctx context.Context, req Request) (Response, error) {
	f.sends++
	if f.sends <= f.fail {
		return Response{}, errors.New("boom")
	}
	return Response{Text: "hello " + req.Model, Finished: true, Usage: Usage{InputTokens: 3, OutputTokens: 5}}, nil
}

func (f *fakeProvider) SendStream(ctx context.Context, req Request, onChunk func(Chunk) error) (Response, error) {
	f.streams++
	if f.streams <= f.fail {
		return Response{}, errors.New("boom")
	}
	var b strings.Builder
	for _, c := range f.chunks {
		if onChunk != nil {
			if err := onChunk(c); err != nil {
				return Response{}, err
			}
		}
		b.WriteString(c.Delta)
	}
	return Response{Text: b.String(), Finished: true, Raw: "stream"}, nil
}

func TestRunSuccess(t *testing.T) {
	provider := &fakeProvider{}
	loop := NewLoop(provider)
	loop.BackoffDelay = time.Millisecond

	var sawRequest, sawResponse bool
	loop.OnRequest = func(req Request) { sawRequest = true }
	loop.OnResponse = func(resp Response) { sawResponse = resp.Text == "hello m1" }

	resp, err := loop.Run(context.Background(), Request{Model: "m1", Prompt: "hi"})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if resp.Text != "hello m1" {
		t.Errorf("Text = %q, want %q", resp.Text, "hello m1")
	}
	if !resp.Finished {
		t.Error("Finished = false, want true")
	}
	if resp.Usage != (Usage{InputTokens: 3, OutputTokens: 5}) {
		t.Errorf("Usage = %+v", resp.Usage)
	}
	if provider.sends != 1 || provider.streams != 0 {
		t.Errorf("provider calls: sends=%d streams=%d", provider.sends, provider.streams)
	}
	if !sawRequest || !sawResponse {
		t.Errorf("hooks not called: sawRequest=%v sawResponse=%v", sawRequest, sawResponse)
	}
}

func TestRunRetriesThenSucceeds(t *testing.T) {
	provider := &fakeProvider{fail: 2}
	loop := NewLoop(provider)
	loop.Retries = 3
	loop.BackoffDelay = time.Millisecond

	var failed int
	loop.OnError = func(err error, attempt int) { failed++ }

	resp, err := loop.Run(context.Background(), Request{Model: "m1"})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if provider.sends != 3 {
		t.Errorf("sends = %d, want 3", provider.sends)
	}
	if failed != 2 {
		t.Errorf("OnError calls = %d, want 2", failed)
	}
	if resp.Text != "hello m1" {
		t.Errorf("Text = %q", resp.Text)
	}
}

func TestRunRetriesExhausted(t *testing.T) {
	provider := &fakeProvider{fail: 10}
	loop := NewLoop(provider)
	loop.Retries = 2
	loop.BackoffDelay = time.Millisecond

	_, err := loop.Run(context.Background(), Request{Model: "m1"})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !errors.Is(err, err) || !strings.Contains(err.Error(), "3 attempts failed") {
		t.Errorf("error = %v, want wrapped '3 attempts failed'", err)
	}
	if !strings.Contains(err.Error(), "boom") {
		t.Errorf("error = %v, want underlying 'boom'", err)
	}
	if provider.sends != 3 {
		t.Errorf("sends = %d, want 3", provider.sends)
	}
}

func TestRunProcessorApplied(t *testing.T) {
	provider := &fakeProvider{}
	loop := NewLoop(provider)
	loop.Processor = ProcessorFunc(func(ctx context.Context, req Request, resp Response) (Response, error) {
		resp.Text = strings.ToUpper(resp.Text)
		return resp, nil
	})

	resp, err := loop.Run(context.Background(), Request{Model: "m1"})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if resp.Text != "HELLO M1" {
		t.Errorf("Text = %q, want %q", resp.Text, "HELLO M1")
	}
}

func TestRunProcessorErrorRetries(t *testing.T) {
	provider := &fakeProvider{}
	loop := NewLoop(provider)
	loop.Retries = 1
	loop.BackoffDelay = time.Millisecond

	calls := 0
	loop.Processor = ProcessorFunc(func(ctx context.Context, req Request, resp Response) (Response, error) {
		calls++
		if calls == 1 {
			return Response{}, errors.New("bad output")
		}
		return resp, nil
	})

	resp, err := loop.Run(context.Background(), Request{Model: "m1"})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if calls != 2 {
		t.Errorf("processor calls = %d, want 2", calls)
	}
	if resp.Text != "hello m1" {
		t.Errorf("Text = %q", resp.Text)
	}
}

func TestRunNonRetryableErrorStops(t *testing.T) {
	provider := &fakeProvider{fail: 10}
	loop := NewLoop(provider)
	loop.Retries = 5
	loop.BackoffDelay = time.Millisecond
	loop.Retryable = func(err error) bool { return false }

	_, err := loop.Run(context.Background(), Request{Model: "m1"})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if provider.sends != 1 {
		t.Errorf("sends = %d, want 1", provider.sends)
	}
}

func TestRunCanceledDuringBackoff(t *testing.T) {
	provider := &fakeProvider{fail: 10}
	loop := NewLoop(provider)
	loop.Retries = 2
	loop.BackoffDelay = time.Hour // long enough that the cancel wins

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := loop.Run(ctx, Request{Model: "m1"})
	if !errors.Is(err, context.Canceled) {
		t.Errorf("err = %v, want context.Canceled", err)
	}
}

func TestRunStream(t *testing.T) {
	provider := &fakeProvider{chunks: []Chunk{
		{Delta: "he"},
		{Delta: "llo"},
		{Delta: "!", Finished: true},
	}}
	loop := NewLoop(provider)
	loop.Stream = true

	var got []string
	loop.OnChunk = func(c Chunk) error {
		got = append(got, c.Delta)
		return nil
	}

	resp, err := loop.Run(context.Background(), Request{Model: "m1"})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if provider.streams != 1 || provider.sends != 0 {
		t.Errorf("provider calls: sends=%d streams=%d", provider.sends, provider.streams)
	}
	if strings.Join(got, "") != "hello!" {
		t.Errorf("chunks = %v, want [he llo !]", got)
	}
	if resp.Text != "hello!" {
		t.Errorf("Text = %q, want %q", resp.Text, "hello!")
	}
	if resp.Raw != "stream" {
		t.Errorf("Raw = %v, want %q", resp.Raw, "stream")
	}
}

func TestRunStreamAbortDoesNotRetry(t *testing.T) {
	provider := &fakeProvider{chunks: []Chunk{
		{Delta: "he"},
		{Delta: "llo"},
	}}
	loop := NewLoop(provider)
	loop.Stream = true
	loop.Retries = 3
	loop.BackoffDelay = time.Millisecond

	seen := 0
	loop.OnChunk = func(c Chunk) error {
		seen++
		if seen == 2 {
			return errors.New("enough already")
		}
		return nil
	}

	_, err := loop.Run(context.Background(), Request{Model: "m1"})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	var aborted *StreamAbortedError
	if !errors.As(err, &aborted) {
		t.Fatalf("err = %T %v, want *StreamAbortedError", err, err)
	}
	if !errors.Is(err, err) || !strings.Contains(err.Error(), "enough already") {
		t.Errorf("err = %v, want wrapped abort reason", err)
	}
	if provider.streams != 1 {
		t.Errorf("streams = %d, want 1 (no retry after abort)", provider.streams)
	}
}

func TestRequestConversation(t *testing.T) {
	if m := (Request{Prompt: "hi"}).Conversation(); len(m) != 1 || m[0].Role != RoleUser || m[0].Content != "hi" {
		t.Errorf("prompt-only request = %+v", m)
	}

	explicit := []Message{{Role: RoleSystem, Content: "sys"}, {Role: RoleUser, Content: "hi"}}
	if m := (Request{Messages: explicit}).Conversation(); len(m) != 2 || m[0].Content != "sys" {
		t.Errorf("explicit request = %+v", m)
	}

	// Explicit messages win over prompt.
	if m := (Request{Prompt: "hi", Messages: explicit}).Conversation(); len(m) != 2 {
		t.Errorf("messages should take precedence: %+v", m)
	}

	if m := (Request{}).Conversation(); m != nil {
		t.Errorf("empty request = %+v, want nil", m)
	}
}

func TestResponseEmpty(t *testing.T) {
	if (Response{}).Empty() != true {
		t.Error("empty response should be Empty")
	}
	if (Response{Text: "x"}).Empty() != false {
		t.Error("text response should not be Empty")
	}
	if (Response{Messages: []Message{{Role: RoleAssistant}}}).Empty() != false {
		t.Error("message response should not be Empty")
	}
}
