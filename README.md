# Guacamole

Tired of having to change AI tool every time you need to ask a detailed question on a different subject?

Guacomole - Allowing you to easily switch experts within the same prompt, so that you can get a full 360 degree understanding of your interest.

Absolutely need the BEST AI response, regardless of the model? Or just Tokenmaxxing? Enable Magi Mode - Multi AI operation to ensure that only the highest quality responses are included

## What it does

- A chat REPL (`./run.sh run`, or `go run ./cmd/cli`) and a desktop GUI
  (`go run ./cmd/gui`) that talk to any configured AI provider — OpenAI and
  OpenAI-compatible APIs (OpenRouter, nVidia, HuggingFace, local servers),
  Anthropic, and OpenCode.
- Multiple chat sessions side by side (GUI tabs), each with its own role and
  provider/model.
- Tool use: the model can work with the local filesystem, run shell commands,
  and fetch URLs — every call is shown as it happens, and permission rules
  decide what needs your approval.
- **Magi Mode**: a panel of three models (Melchior, Balthasar, Casper) answers
  each turn, scores one another's answers, and only the best reply is kept —
  pick it as the **Magi** model in a session.
- Conversations with history, saved after every turn and resumable with
  `guacamole --session <session ID>`.

## Quick start

```sh
./run.sh run                 # build & run: macOS GUI, or the host CLI elsewhere
go run ./cmd/cli             # the terminal REPL
go run ./cmd/gui             # the desktop GUI
./run.sh test                # go test ./...
```

You need at least one provider configured. Add one from the GUI
(Preferences → Providers); the CLI will point you there on first run too. See
[PROVIDERS.md](docs/PROVIDERS.md).

## Docs

The `docs/` folder has the details:

| Doc | What's inside |
| --- | ------------- |
| [FEATURES](docs/FEATURES.md) | What Guacamole does: tool-based chat, mid-session model/provider switching, Magi Mode |
| [GUI](docs/GUI.md) | The desktop app: sessions, preferences (providers, roles, magi, sessions, permissions), building |
| [TOOLS](docs/TOOLS.md) | The tools a model can call: filesystem, shell, web, user questions |
| [PROVIDERS](docs/PROVIDERS.md) | API types, presets, the config file, app attribution, model selection & listing |
| [CONFIG](docs/CONFIG.md) | Complete config file reference: permissions, providers, selected model, magi, env fallback |
| [ARCHITECTURE](docs/ARCHITECTURE.md) | Internals: the `ai` request/processing/response loop, sessions, tool use, permissions |
| [CONTRIBUTING](docs/CONTRIBUTING.md) | How to contribute: fork → branch → pull request |

# Why Guac?

From GoCliForAI. This idea started from a small seed, and just kept growing