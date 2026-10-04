package ai

import "context"

// Provider sends Requests to an AI backend and returns Responses.
//
// A Provider is the "request" half of the request/processing/response
// cycle: it translates a normalized Request into the wire format of a
// specific API and translates the wire response back into a normalized
// Response. Implementations must be safe for concurrent use.
type Provider interface {
	// Send performs a single non-streaming exchange.
	Send(ctx context.Context, req Request) (Response, error)

	// SendStream performs a streaming exchange. onChunk is called for each
	// piece of output as it arrives; it may be nil. Returning a non-nil
	// error from onChunk aborts the stream, and that error is returned from
	// SendStream. The returned Response is the accumulated whole: its Text
	// is the concatenation of every chunk.
	SendStream(ctx context.Context, req Request, onChunk func(Chunk) error) (Response, error)
}
