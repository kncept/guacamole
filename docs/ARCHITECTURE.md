# Architecture

Guacamole is a Go codebase with a small core — the `ai` package — and two
frontends on top of it: a terminal REPL (`cmd/cli` + `promptrunner`) and a
Fyne desktop GUI (`cmd/gui` + `app`). This document covers the internals that
the [README](../README.md) deliberately omits.

## The `ai` package

Every AI tool, from the REPL to the GUI, runs the same three-phase cycle:

1. **Request** — a `Request` is sent to a `Provider` (e.g. `restfulai` for
   OpenAI-compatible APIs, `anthropicai` for the Anthropic Messages API).
2. **Processing** — the provider streams or returns output; a `Processor`
   post-processes the result (validation, extraction, transformation).
3. **Response** — the finished `Response` (text, usage, raw payload) is
   returned.

`ai.Loop` owns the cycle end to end, adding what production AI tools always
need: retries with exponential backoff, streaming, cancellation, and
observation hooks.

```go
provider := restfulai.NewRestfulAI(conf)
loop := ai.NewLoop(provider) // 2 retries, 500ms doubling backoff
loop.Stream = true
loop.OnChunk = func(c ai.Chunk) error { fmt.Print(c.Delta); return nil }

resp, err := loop.Run(ctx, ai.Request{Prompt: "hello"})
```

The pieces:

- `ai.Request` — model, prompt or full message list, system prompt,
  temperature, max tokens, plus a `Tag` for caller metadata.
- `ai.Provider` — `Send` (one-shot) and `SendStream` (per-chunk callback that
  can abort mid-stream).
- `ai.Processor` — transforms or validates the final `Response`; may fail the
  attempt, which the loop then retries.
- `ai.Loop` — `Run` executes request → processing → response, retrying
  transient failures and never retrying a cancelled context or an aborted
  stream. Providers report HTTP 429s as an `ai.RateLimitError`.

### Sessions

`ai.Session` is a stateful conversation on top of a `Loop`: `Say` appends each
user message to the history, sends the whole conversation, and records the
reply. Failed turns leave the history untouched; `Clear` starts over.

Every session has a random ID, and `sessionstore` saves/loads sessions as JSON
files (`<dir>/<session ID>.json`).

```go
session := ai.NewSession(loop)
session.UserInput = userInput // a userinput.RequestForegroundUserInput
session.Tools = tools.AllTools(nil, nil) // nil checker allows everything; nil userInput omits user_question
session.Say(ctx, "my name is Ada")
resp, _ := session.Say(ctx, "what is my name?") // the model remembers: history is sent with every turn
```

Attributes on a session: `Model`, `SystemPrompt`, `Temperature`, `MaxTokens`,
`Tools`, `MaxToolRounds` (default 10, but `NewSession` actually starts at 30),
and `Magi` (see below).

A `UserInput` (a `userinput.RequestForegroundUserInput`) decides what happens
when a model keeps calling tools past `MaxToolRounds`, and offers a retry
when the provider rate limits a request (HTTP 429).

### Magi Mode

`ai.Magi` is a panel of `ai.MagiMember`s that replaces the single-model turn
with a two-phase one. Each member owns its own `ai.Loop` (and so its own
model), and `Magi.Run` takes the same `ai.Request` a `Loop` would:

1. **Inference** — every member's `Loop.Run` is called with the request in
   parallel; each answers independently.
2. **Voting** — once all answers are in, every member's `Loop.Run` is called
   again in a single request carrying the *other* members' answers, and asked
   to score each one out of 100.
3. **Collation** — the scores are totaled per answer and the highest-scoring
   answer is returned; ties keep the earlier member.

Each phase runs its members concurrently through a small generic `runPhase`
helper and waits for all of them, so votes never race ahead of the answers
they judge. A judge's reply is parsed first as the JSON object the prompt asks
for, falling back to scanning for a magi name followed by a number; a judge
that returns no usable score simply contributes nothing to the totals.

`ai.Session.Magi`, when set, switches a session into Magi Mode: `Say` runs the
panel instead of the session's own `Loop`, and the winning response is then
executed exactly like a normal reply — its text is emitted through the
session's `Loop.OnChunk`/`OnResponse` hooks (so a UI needs no special case),
its tool calls run, and the conversation loops on the tool output.

```go
magi := &ai.Magi{Members: []ai.MagiMember{
	{Name: "Melchior", Loop: ai.NewLoop(providerA), Model: "gpt-4o"},
	{Name: "Balthasar", Loop: ai.NewLoop(providerB), Model: "claude-3-5-sonnet"},
	{Name: "Casper", Loop: ai.NewLoop(providerC), Model: "llama-3.1-70b"},
}}
session.Magi = magi // Say now runs the panel
```

The magi run non-streaming (`Loop.Stream = false`): each returns a whole
response, and only the winner is surfaced to the UI, as a single chunk.
`Magi.OnPhase`, when set, reports each phase with a short line (the GUI wires
it to activity lines in the transcript). The panel is configured in
Preferences → Magi and selected per session through the virtual **Magi** model
at the top of the model list (see [GUI.md](GUI.md#magi)).

### Tool use

When a response requests tool calls, `Say` executes them, appends the calls
and results to the history, and runs again until the model answers — up to
`MaxToolRounds`, after which the session's `UserInput` prompts the user to
reset the round count and keep going, or abort. Handler errors become tool
results so the model can recover. See [TOOLS.md](TOOLS.md) for the built-in
tools.

### Hooks

`OnRequest`, `OnChunk`, `OnResponse`, `OnError` are available for logging,
debugging, and UIs.

### User input

`userinput.RequestForegroundUserInput` is the single interface behind every
foreground user prompt: permission checks, the model's `user_question` tool,
tool-round-limit decisions, and rate-limit retries. Its
`Prompt(text, allowFree, options)` renders the question (GUI buttons, terminal
menu) and returns the chosen option or free text. The REPL implements it with
a numbered terminal menu; the GUI swaps the input area for buttons.

## Frontends

- **`promptrunner`** — the REPL core. Each prompt goes through a streaming
  session and prints chunks as they arrive, so a conversation has memory until
  you type `/new`. Tool calls are printed as they run, summarized so the user
  sees what happens (e.g. `→ read_file go.mod`, `→ bash: git status`,
  `→ ls /tmp`). The GUI shows the same lines in the session's chat window
  while tools run.
- **`app`** — the Fyne GUI: tabbed sessions, per-session provider/role
  dropdowns, and the Preferences screen (see [GUI.md](GUI.md)).

## Providers and model clients

- `modelclient` picks the implementation for a provider's API type:
  `restfulai` (OpenAI-compatible) or `anthropicai` (Anthropic), and dispatches
  model listing.
- `restfulai` uses the `openai-go` client's Responses API, and attaches
  OpenRouter app attribution headers when the provider opts in (see
  [PROVIDERS.md](PROVIDERS.md#app-attribution)).

## Permissions

Permissions are grouped into three categories, all persisted in
`~/.guac/config.json`:

| Category | Rules | Enforced on |
| --- | --- | --- |
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

The first time the model touches a path, runs a command or fetches a domain
with no settled rule, you get a prompt:

```
→ write_file notes/todo.md
Grant filesystem write access to /home/me/notes/todo.md? [y/N]
```

Rule semantics:

- **Filesystem**: a directory grant covers that directory and everything under
  it; the most specific directory wins, and reads and writes are tracked
  separately. `list_allowed_directories` reports exactly this subset.
- **Shell**: one answer settles the policy for every command in every shell.
- **Web**: a domain rule also covers its subdomains (`example.com` covers
  `api.example.com`).
- **`allowAll`** (filesystem and web) is an override: everything is allowed
  without asking and the individual rules are ignored.

Granting or denying is persisted in `~/.guac/config.json`, so you are never
asked twice about the same thing. Denying (or Ctrl-D) sends the error back to
the model as the tool result and stays denied until the config changes. The
`permissions` package resolves each check: the most specific rule that
answers, falling through to the user input whose lasting answers are recorded
as rules.