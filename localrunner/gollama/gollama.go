package gollama

import (
	"fmt"
	"log"

	"github.com/dianlight/gollama.cpp"
)

/*
*
WIP: Have an 'embedded runner'
*
*/
func Run() {
	// Initialize the library
	gollama.Backend_init()
	defer gollama.Backend_free()

	// Load model
	params := gollama.Model_default_params()
	model, err := gollama.Model_load_from_file("path/to/model.gguf", params)
	if err != nil {
		log.Fatal(err)
	}
	defer gollama.Model_free(model)

	// Create context
	ctxParams := gollama.Context_default_params()
	ctx, err := gollama.Init_from_model(model, ctxParams)
	if err != nil {
		log.Fatal(err)
	}
	defer gollama.Free(ctx)

	// Tokenize and generate
	prompt := "The future of AI is"
	tokens, err := gollama.Tokenize(model, prompt, true, false)
	if err != nil {
		log.Fatal(err)
	}

	// Create batch and decode
	batch := gollama.Batch_init(int32(len(tokens)), 0, 1)
	defer gollama.Batch_free(batch)

	// for i, token := range tokens {
	// 	gollama.Batch_add(batch, token, int32(i), []int32{0}, false)
	// }

	if err := gollama.Decode(ctx, batch); err != nil {
		log.Fatal(err)
	}

	// Sample next token
	// logits := gollama.Get_logits_ith(ctx, -1)
	// candidates := gollama.Token_data_array_init(model)

	sampler := gollama.Sampler_init_greedy()
	defer gollama.Sampler_free(sampler)

	// newToken := gollama.Sampler_sample(sampler, ctx, candidates)

	// Convert token to text
	// text := gollama.Token_to_piece(model, newToken, false)
	text := gollama.Token_to_piece(model, tokens[0], false)
	fmt.Printf("Generated: %s\n", text)
}
