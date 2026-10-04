package ai

import (
	"context"
	"errors"
	"fmt"
	"time"
)

// DefaultBackoffDelay is the default pause before the first retry of a Loop.
const DefaultBackoffDelay = 500 * time.Millisecond

// Retryable decides whether the loop should retry after an error.
type Retryable func(err error) bool

// DefaultRetryable retries every error except a cancelled context.
func DefaultRetryable(err error) bool {
	return !errors.Is(err, context.Canceled)
}

// StreamAbortedError wraps an error returned by an OnChunk handler. The loop
// does not retry aborted streams: the caller made the decision to stop.
type StreamAbortedError struct {
	Err error
}

func (e *StreamAbortedError) Error() string {
	return fmt.Sprintf("ai: stream aborted: %v", e.Err)
}

func (e *StreamAbortedError) Unwrap() error { return e.Err }

// Loop is the request/processing/response cycle.
//
// Run sends the request to the Provider, hands the result to the Processor,
// and returns it. Failures are retried with exponential backoff up to
// Retries extra attempts.
type Loop struct {
	// Provider sends requests. Required.
	Provider Provider

	// Processor post-processes responses. Optional; defaults to
	// DefaultProcessor.
	Processor Processor

	// Stream makes the loop use Provider.SendStream instead of Send.
	Stream bool

	// Retries is the number of extra attempts after the first. 0 means
	// "try once".
	Retries int

	// BackoffDelay is the pause before the first retry; it doubles after
	// each retry. Defaults to DefaultBackoffDelay.
	BackoffDelay time.Duration

	// Retryable decides whether an error is worth retrying. Defaults to
	// DefaultRetryable.
	Retryable Retryable

	// OnRequest observes each attempt before it is sent.
	OnRequest func(req Request)

	// OnChunk observes streaming chunks as they arrive. Returning a
	// non-nil error aborts the stream.
	OnChunk func(chunk Chunk) error

	// OnResponse observes a successful response after processing.
	OnResponse func(resp Response)

	// OnError observes each failed attempt. attempt is 1-based.
	OnError func(err error, attempt int)
}

// NewLoop builds a Loop with sensible defaults around provider: 2 retries
// with a 500ms doubling backoff, retrying every error except cancellation.
func NewLoop(provider Provider) *Loop {
	return &Loop{
		Provider:     provider,
		Processor:    DefaultProcessor{},
		Retries:      2,
		BackoffDelay: DefaultBackoffDelay,
		Retryable:    DefaultRetryable,
	}
}

// Run executes one request/processing/response cycle for req and returns
// the processed response.
func (this *Loop) Run(ctx context.Context, req Request) (Response, error) {
	delay := this.BackoffDelay
	if delay <= 0 {
		delay = DefaultBackoffDelay
	}
	shouldRetry := this.Retryable
	if shouldRetry == nil {
		shouldRetry = DefaultRetryable
	}
	processor := this.Processor
	if processor == nil {
		processor = DefaultProcessor{}
	}

	var lastErr error
	for attempt := 0; attempt <= this.Retries; attempt++ {
		if attempt > 0 {
			select {
			case <-ctx.Done():
				return Response{}, ctx.Err()
			case <-time.After(delay):
			}
			delay *= 2
		}

		if this.OnRequest != nil {
			this.OnRequest(req)
		}

		var resp Response
		var err error
		if this.Stream {
			resp, err = this.Provider.SendStream(ctx, req, this.chunkHandler())
		} else {
			resp, err = this.Provider.Send(ctx, req)
		}
		if err == nil {
			resp, err = processor.Process(ctx, req, resp)
		}

		if err != nil {
			lastErr = err
			if this.OnError != nil {
				this.OnError(err, attempt+1)
			}

			// An aborted stream is a deliberate stop, not a failure.
			var aborted *StreamAbortedError
			if errors.As(err, &aborted) {
				return Response{}, err
			}
			if !shouldRetry(err) {
				return Response{}, err
			}
			continue
		}

		if this.OnResponse != nil {
			this.OnResponse(resp)
		}
		return resp, nil
	}

	return Response{}, fmt.Errorf("ai: %d attempts failed: %w", this.Retries+1, lastErr)
}

// chunkHandler adapts the OnChunk hook to the provider's callback: a
// non-nil return aborts the stream with a StreamAbortedError.
func (this *Loop) chunkHandler() func(Chunk) error {
	return func(chunk Chunk) error {
		if this.OnChunk == nil {
			return nil
		}
		if err := this.OnChunk(chunk); err != nil {
			return &StreamAbortedError{Err: err}
		}
		return nil
	}
}
