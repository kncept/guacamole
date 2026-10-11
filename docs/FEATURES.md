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

## Magi Mode (always get the best response)

Magi Mode is the **multi-AI mode**: instead of trusting one model, every turn
is sent to a panel of models, the models judge one another's answers, and only
the best answer is kept.

- Want the BEST answer regardless of the model? Magi Mode runs the request
  across several models and picks the best output.
- Just Tokenmaxxing? Run the panel on cheap models when you want quality
  without paying flagship prices.

### The panel

Magi Mode has three fixed agents, in order: **Melchior**, **Balthasar**, and
**Casper**. Each one plays a real model of your choosing — set them in
**Preferences → Magi** (see [GUI.md](GUI.md#magi)). A model may hold more than
one seat.

### How a turn works

Every turn runs in two phases before the answer is executed:

1. **Inference** — each magi is sent the current conversation (your prompt, or
   the latest tool output) and answers independently, in parallel.
2. **Voting** — once *all* magi have answered, each magi receives the other
   magi's answers in a **single** request and scores each one out of 100.
3. **Collation** — the scores are totaled per answer (a model's total is the
   sum of the scores the *other* magi gave it) and only the highest-scoring
   answer is kept. Ties keep the earlier magi. A judge that returns no usable
   score simply contributes nothing.
4. **Execution** — the kept answer is then handled exactly like a normal
   reply: its text is shown, any tool calls it requested run, and the
   conversation loops on the tool output — which re-enters the panel for the
   next turn.

The magi never see their own answer while judging, and only the winner is ever
shown: the losing answers and the raw votes stay out of the transcript. The
session log reports the panel's progress with activity lines such as
`→ Magi: answers in from Melchior, Balthasar, Casper` and
`→ Magi: Melchior 130, Balthasar 170, Casper 90 — Balthasar wins`.

> **Note:** Magi Mode needs a model assigned to all three magi. Selecting
> **Magi** before that shows an error and leaves the session on its previous
> model.

The `ai` package backs this with `ai.Magi` (a panel of `ai.MagiMember`s run
through `ai.Loop`s); see [ARCHITECTURE.md](ARCHITECTURE.md#magi-mode).