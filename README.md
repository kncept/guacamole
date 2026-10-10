# Guacamole

Tired of having to change AI tool every time you need to ask a detailed question on a different subject?

Guacomole - Allowing you to easily switch experts within the same prompt, so that you can get a full 360 degree understanding of your interest.

Absolutely need the BEST AI response, regardless of the model? Or just Tokenmaxxing? Enable Magi Mode - Multi AI operation to ensure that only the highest quality responses are included

## Cool Tech Things

1. Runs a chat session with history — saved to `~/.guac/session/<session ID>.json` after every turn and resumable with `--ses <session ID>`
2. Tool use: the model can call `read_file`, `ls` and `write_file` to work with the local filesystem, run shell commands, and fetch URLs with `http_fetch`; every call is printed as it happens, and the permission categories decide what needs your approval
3. Has an `ai` package that factors out the request/processing/response loop every AI tool runs

## Sessions

Every conversation is a session with a random ID. The session is saved after every turn to `~/.guac/session/<session ID>.json`, and on exit (Ctrl-D or Ctrl-C) the save location and the `guacamole --session <session ID>` resume command are printed.

- `/new` starts a fresh session with a new ID.
- `--ses <session ID>` resumes a saved session; `-s` and `--session` are synonyms.

## Permissions

Permissions are grouped into three categories, all persisted in `~/.guac/config.json`:

| Category | Rules | Enforced on |
|---|---|---|
| **Filesystem** | Read/Write tracked per directory, plus an `allowAll` override | `read_file`, `ls`, `write_file`, `glob`, `edit_file`, `create_directory`, `move_file`, `list_allowed_directories` |
| **Shell** | One `allow`/`ask`/`deny` policy for *all* shell operations | every shell tool (`bash`, `sh`, `zsh`) |
| **Web** | `allow`/`ask`/`deny` per domain, plus an `allowAll` override | `http_fetch`, including every redirect target |

```json
{
  "permissions": {
    "filesystem": {
      "allowAll": false,
      "directories": [
        {"directory": "/home/me/notes", "read": "allow", "write": "allow"},
        {"directory": "/etc", "read": "deny", "write": "deny"}
      ]
    },
    "shell": {"policy": "ask"},
    "web": {
      "allowAll": false,
      "domains": [{"domain": "example.com", "policy": "allow"}]
    }
  }
}
```

The first time the model touches a path, runs a command or fetches a domain with no settled rule, you get a prompt:

```
→ write_file notes/todo.md
Grant filesystem write access to /home/me/notes/todo.md? [y/N]
```

- **Filesystem**: a directory grant covers that directory and everything under it; the most specific directory wins, and reads and writes are tracked separately. `list_allowed_directories` reports exactly this subset.
- **Shell**: one answer settles the policy for every command in every shell.
- **Web**: a domain rule also covers its subdomains (`example.com` covers `api.example.com`).
- **`allowAll`** (filesystem and web) is an override: everything is allowed without asking and the individual rules are ignored.

Granting or denying is persisted in `~/.guac/config.json`, so you are never asked twice about the same thing. Denying (or Ctrl-D) sends the error back to the model as the tool result and stays denied until you change the config.

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
- `ai.Tool` — a function the model can call: name, description, JSON Schema parameters, and a `ToolHandler`. When a response requests tool calls, `Say` executes them, appends the calls and results to the history, and runs again until the model answers — up to `MaxToolRounds`. Handler errors become tool results so the model can recover. The `tools` package provides the filesystem tools (`read_file`, `ls`, `write_file`, `glob`, `edit_file`, `create_directory`, `move_file`, `list_allowed_directories`), one shell tool per installed shell, and `http_fetch`; the `permissions` package enforces them through the three permission categories (filesystem, shell, web) persisted in `~/.guac/config.json`.
- Hooks: `OnRequest`, `OnChunk`, `OnResponse`, `OnError` for logging, debugging, and UIs.

```go
session := ai.NewSession(loop)
session.Tools = tools.AllTools(nil, nil) // nil checker allows everything; nil handler omits user_question
session.Say(ctx, "my name is Ada")
resp, _ := session.Say(ctx, "what is my name?") // the model remembers: history is sent with every turn
```

The REPL (`promptrunner`) is built on top of it: each prompt goes through a streaming session and prints chunks as they arrive, so a conversation has memory until you type `/new`. Tool calls are printed as they run, summarized so the user sees what happens (e.g. `→ read_file go.mod`, `→ bash: git status`, `→ ls /tmp`). The GUI shows the same lines in the session's chat window while tools run.

# Why Guac?

From GoCliForAI. This idea started from a small seed, and just kept growing
