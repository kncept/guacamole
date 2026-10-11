# Features

An overview of what Guacamole does and the ideas it is built around.

## Tool based

Guacamole is **tool based**: the model doesn't just answer — it can *act*. A
session makes the tools from the `tools` package available to the model, so it
can work with your machine:

- **Filesystem** — read, list, write, glob, edit, create directories, and
  move files (`read_file`, `ls`, `write_file`, `glob`, `edit_file`,
  `create_directory`, `move_file`, `list_allowed_directories`).
- **Shell** — run command lines through whatever consoles you have installed
  (`bash`, `sh`, `zsh`).
- **Web** — fetch URLs with `http_fetch`.
- **User** — ask you questions in the middle of a turn (`user_question`).

Every call is shown as it runs — `→ bash: git status`, `→ ls /tmp` — so tool
activity never looks like a frozen prompt. Permission rules (filesystem, shell,
web) decide what needs your approval, and the answers you give are remembered.
When the model keeps calling tools, the conversation keeps going round until it
answers.

Detail: [TOOLS.md](TOOLS.md).

## Change models and providers mid-session

Each session picks its own **role** (a system prompt) and **provider/model**
from whatever is configured. In the GUI you can change either one mid-session:

- The session banner's **Provider / Model** dropdown swaps the provider or
  model the conversation talks to — the session's history is kept, so you can
  ask the next model what the previous one said.
- The **Current role** dropdown swaps the system prompt of the same
  conversation.

Selecting a different provider (say, switching from an OpenAI-compatible one
to Anthropic) builds the new client for that provider, keeps the session's
conversation, and the next prompt goes to the new model. That's the core idea
behind Guacamole: switch experts within the same prompt for a full 360-degree
view of your interest.

Detail: [GUI.md](GUI.md), [PROVIDERS.md](PROVIDERS.md).

## Magi Mode (always get the best response) — planned

> **Status:** on the roadmap, not shipped yet. This section describes the
> design intent.

Magi Mode is the planned **multi-AI mode**: instead of trusting one model, a
prompt is sent to several, and only the highest-quality responses are kept.

- Want the BEST answer regardless of the model? Magi Mode runs the request
  across multiple models and picks the best output.
- Just Tokenmaxxing? Magi Mode can trade quality for cost when you want to
  save tokens.

The `ai` package is built with this in mind: the request/processing/response
loop (`ai.Loop`) and providers are factored out specifically so a future
multi-agent system can run the same cycle across several providers and
combine the results. The exact selection strategy (votes, scoring, fallbacks,
cost thresholds) is still to be decided.