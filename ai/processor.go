package ai

import "context"

// Processor is the "processing" half of the request/processing/response
// cycle for results: it post-processes a Response before the loop returns
// it.
//
// Use a Processor to validate output, extract structured data, redact
// secrets, or reshape the response into whatever the rest of the tool
// needs. Processors may mutate the response or return an error to fail the
// attempt (which the loop may then retry).
type Processor interface {
	Process(ctx context.Context, req Request, resp Response) (Response, error)
}

// ProcessorFunc adapts a plain function to the Processor interface.
type ProcessorFunc func(ctx context.Context, req Request, resp Response) (Response, error)

// Process calls f.
func (f ProcessorFunc) Process(ctx context.Context, req Request, resp Response) (Response, error) {
	return f(ctx, req, resp)
}

// DefaultProcessor returns responses unchanged. A Loop with no Processor set
// behaves as if it had one.
type DefaultProcessor struct{}

// Process returns resp unchanged.
func (DefaultProcessor) Process(ctx context.Context, req Request, resp Response) (Response, error) {
	return resp, nil
}
