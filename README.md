# Guacamole

N.B. Guacamole is in early WIP, but is showing great promise.

Tired of having to change AI tool every time you need to ask a detailed question on a different subject?

Guacomole - Allowing you to easily switch experts within the same prompt, so that you can get a full 360 degree understanding of your interest.

Tokenmaxxing? Or just need the BEST AI response, regardless of the model? Enable Magi Mode - Multi AI operation to ensure that only the highest quality responses are included

## Cool Tech Things

1. Runs a chat session with history — saved to `~/.guac/session/<session ID>.json` after every turn and resumable with `--ses <session ID>`
2. Tool use: the model can call `read_file`, `ls` and `write_file` to work with the local filesystem; every call is printed as it happens, and writes need your approval
3. Has an `ai` package that factors out the request/processing/response loop every AI tool runs

## Sessions

Every conversation is a session with a random ID. The session is saved after every turn to `~/.guac/session/<session ID>.json`, and on exit (Ctrl-D or Ctrl-C) the save location and the `guacamole --session <session ID>` resume command are printed.

- `/new` starts a fresh session with a new ID.
- `--ses <session ID>` resumes a saved session; `-s` and `--session` are synonyms.

## Permissions

`write_file` needs your approval per directory. The first time the model tries to write into a directory you get a `[Y/n]` prompt:

```
→ write_file {"path":"notes/todo.md","content":"..."}
Grant write access to /home/me/notes (and its subdirectories)? [Y/n]
```

Granting covers that directory and everything under it, forever: grants are persisted in `~/.guac/permissions.json`, so you are never asked twice about the same directory. Denying (or Ctrl-D) sends the error back to the model as the tool result, and it will be asked again next time.

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
- `ai.Tool` — a function the model can call: name, description, JSON Schema parameters, and a `ToolHandler`. When a response requests tool calls, `Say` executes them, appends the calls and results to the history, and runs again until the model answers — up to `MaxToolRounds`. Handler errors become tool results so the model can recover. The `tools` package provides `read_file`, `ls` and `write_file`; `write_file` takes an `allowWrite` guard, which the `permissions` package implements with per-directory grants persisted to `~/.guac/permissions.json`.
- Hooks: `OnRequest`, `OnChunk`, `OnResponse`, `OnError` for logging, debugging, and UIs.

```go
session := ai.NewSession(loop)
session.Tools = tools.AllTools(nil) // nil allows all writes
session.Say(ctx, "my name is Ada")
resp, _ := session.Say(ctx, "what is my name?") // the model remembers: history is sent with every turn
```

The REPL (`promptrunner`) is built on top of it: each prompt goes through a streaming session and prints chunks as they arrive, so a conversation has memory until you type `/new`. Tool calls are printed as they run (e.g. `→ read_file {"path":"go.mod"}`).

## Cool things TODO

1. Add a 'Magi' mode (default 3)  where agents vote on the best answer
2. Interactions Modes. Default to REPL, but add a GUI mode as well.