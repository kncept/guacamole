# Guacamole

A tool I was using was giving me errors, due to an upstream bug that has been closed out as will not fix.
So, I built this as a workaround.

Of course, it turned out that the bug in question _wasn't_ the issue, but hey, now I have a cool tool that does a bunch of things.

## Cool things

1. Runs a chat session with history — saved to `./sessions/<session ID>.json` after every turn and resumable with `--ses <session ID>`
2. Has an `ai` package that factors out the request/processing/response loop every AI tool runs

## Sessions

Every conversation is a session with a random ID. The session is saved after every turn to `./sessions/<session ID>.json`, and on exit (Ctrl-D or Ctrl-C) the save location and the `guacamole --session <session ID>` resume command are printed.

- `/new` starts a fresh session with a new ID.
- `--ses <session ID>` resumes a saved session; `-s` and `--session` are synonyms.

## The `ai` package

Every AI tool, from this REPL to a future multi-agent 'Magi' mode, runs the same cycle:

1. **Request** — a `Request` is sent to a `Provider` (e.g. `restfulai` for OpenAI-compatible APIs).
2. **Processing** — the provider streams or returns output; a `Processor` post-processes the result (validation, extraction, transformation).
3. **Response** — the finished `Response` (text, usage, raw payload) is returned.

`ai.Loop` owns the cycle end to end, adding what production AI tools always need: retries with exponential backoff, streaming, cancellation, and observation hooks.

```go
provider := restfulai.NewRestfulAI(conf)
loop := ai.NewLoop(provider) // 2 retries, 500ms doubling backoff
loop.Stream = true
loop.OnChunk = func(c ai.Chunk) error { fmt.Print(c.Delta); return nil }

resp, err := loop.Run(ctx, ai.Request{Prompt: "hello"})
```

- `ai.Request` — model, prompt or full message list, system prompt, temperature, max tokens, plus a `Tag` for caller metadata.
- `ai.Provider` — `Send` (one-shot) and `SendStream` (per-chunk callback that can abort mid-stream).
- `ai.Processor` — transforms or validates the final `Response`; may fail the attempt, which the loop then retries.
- `ai.Loop` — `Run` executes request → processing → response, retrying transient failures and never retrying a cancelled context or an aborted stream.
- `ai.Session` — a stateful conversation on top of a `Loop`: `Say` appends each user message to the history, sends the whole conversation, and records the reply. Failed turns leave the history untouched; `Clear` starts over. Every session has a random ID, and `sessionstore` saves/loads sessions as JSON files (`<dir>/<session ID>.json`).
- Hooks: `OnRequest`, `OnChunk`, `OnResponse`, `OnError` for logging, debugging, and UIs.

```go
session := ai.NewSession(loop)
session.Say(ctx, "my name is Ada")
resp, _ := session.Say(ctx, "what is my name?") // the model remembers: history is sent with every turn
```

The REPL (`promptrunner`) is built on top of it: each prompt goes through a streaming session and prints chunks as they arrive, so a conversation has memory until you type `/new`.

## Cool things TODO

1. Add a 'Magi' mode (default 3)  where agents vote on the best answer
2. Interactions Modes. Default to REPL, but add a GUI mode as well.