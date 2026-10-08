# GUI

The GUI is a desktop application built on [Fyne](https://fyne.io) (Go). It lets
you run multiple chat sessions side by side, each with its own model and role,
and manage your models, roles, and saved sessions.

Run it:

```sh
go run ./cmd/gui
```

Or build a binary:

```sh
go build -o guacamole_gui ./cmd/gui
```

The GUI reads its models from your opencode config at startup, so that file
must exist before it can do anything useful (see [Configuration and data
locations](#configuration-and-data-locations)).

---

## Main window

The main window (titled **Guacamole GUI**, opens at 600×400) is a tab bar of
chat sessions. Each session is its own tab.

The leftmost item is a pinned **Preferences** tab (a gear icon). It is not a
session: clicking it replaces the window content with the Preferences screen
(see [Preferences](#preferences)). Every other tab is a session.

A session tab is laid out as a vertical stack:

| Region | Widget | Purpose |
| ------ | ------ | ------- |
| Top border | Banner (two dropdowns) | Pick the **role** and the **provider / model** for this session |
| Middle | Scrolling output | The running transcript of the conversation |
| Bottom border | Input box + **Submit** button | Type a prompt and send it |

### Banner (role and model)

Two selectors control what the session talks to:

- **Current role:** picks a role (a named system prompt). Selecting one sets
  the session's system prompt.
- **Provider / Model:** picks a model, shown as `Provider / Model` (e.g.
  `OpenAI / GPT-4o`). Selecting one rebuilds the session's AI loop against that
  provider and model.

The first model and first role in the config are preselected when a tab is
created.

### Input and output

- The input is a **multi-line** text field, so **Enter inserts a newline**. To
  send a prompt, click **Submit** (or use the entry's submit shortcut).
- On submit, the prompt is appended to the transcript as `You: …`, the input is
  cleared, and the reply is streamed back chunk by chunk, prefixed `AI: `.
- If the call fails, the transcript gets an `Error: …` line instead of a reply.
  The GUI keeps running.

> **Note:** the GUI does not currently persist chat transcripts. A session's
> conversation lives in memory for the life of the window. (Saved-session files
> are managed under Preferences → Sessions, backed by `~/.guac/session`.)

### Creating and closing sessions

- **New Session tab:** the rightmost tab, always labeled **New Session**.
  Clicking it creates a fresh session and keeps **New Session** in the last
  position. Its placeholder text reads *“Click to create new session.”*
- **File → New Tab** does the same thing.
- **Close a tab (✕):** removes that session. The GUI never lets you close down
  to zero sessions — closing the last one opens a fresh **Session 1**.

Session tabs are named `Session 1`, `Session 2`, … using a monotonic counter
that only ever increases, so names stay unique even as you close tabs.

### Tools

Every session's model can call the tools from the `tools` package:

- `read_file` — read a file (capped at 64 KiB, with a truncation note).
- `ls` — list a directory's entries.
- `write_file` — write a file, creating parent directories as needed.
- One shell tool per available console — `bash`, `sh`, and/or `zsh` — each
  running a command line.

Tool calls run in the background as the model works; the GUI shows the final
reply text but does not render individual tool invocations or prompt for write
approval. (The README describes a per-directory write-approval flow, but it is
not implemented in the current code — `write_file` runs unguarded.)

---

## Menu

The main window menu has two top-level menus:

- **File**
  - **New Tab** — create a new session tab.
  - **Preferences** — open the Preferences screen (see below).
- **About**
  - **About** — shows a short about screen:
    *“Guacamole GUI — A simple Fyne GUI with tabbed sessions.”*

---

## Preferences

Opened from the pinned gear tab at the far left of the tab bar (or **File →
Preferences**). Opening it swaps the main window's content over to the
Preferences screen rather than opening a new tab; the tab bar is replaced while
it is shown.

The screen is a **Back** button and a *Preferences* title on top, with a list
of sections on the left and the selected section's screen on the right. The
last-viewed section is remembered the next time you open Preferences. **Back**
returns to the conversation tabs, reselecting the session you were on.
Sections:

### Models

Lists every model available from your opencode configuration, grouped under a
provider heading. Each provider group shows:

- the provider name (bold), and
- its base URL, if set,

followed by each model as a bullet: `- Model name` (with the model ID in
parentheses when it differs from the display name).

If no models are configured, the screen shows:
*“No models found. Add providers to your opencode configuration.”*

### Roles

A list of all roles. Selecting a role shows its name (bold) and its full system
prompt below the list. The first role is selected by default. Shows
*“No roles found.”* if there are none.

### Sessions

Lists every saved session on disk (from `~/.guac/session`), each row showing
the session ID with a **Delete** button on the right. **Delete** removes the
file and refreshes the list. If a delete fails, an error dialog is shown.

Empty or unreadable states show guidance instead of a blank list, e.g.
*“No saved sessions in …”* or *“Could not list saved sessions: …”*.

---

## Configuration and data locations

| What | Where |
| ---- | ----- |
| Provider/model config | `~/.config/opencode/opencode.json` |
| Saved sessions | `~/.guac/session/<session ID>.json` |

The GUI loads models and roles once at startup, so changes to the config take
effect the next time the GUI is launched.

---

## Structure

| File | Responsibility |
| ---- | -------------- |
| `cmd/gui/main.go` | Entry point — creates the app and starts the event loop |
| `app/app.go` | The `Guac` app: main window, tabs, menu, session lifecycle |
| `app/sessontab.go` | One session tab: banner, input, output, prompt handling |
| `app/preferences.go` | The Preferences screen (Models, Roles, Sessions) |
| `config` | Loads models and provider connection details |
| `roles` | Loads roles (system prompts) |
| `sessionstore` | Lists/deletes saved session files |
| `ai`, `restfulai`, `tools` | The underlying AI loop, provider, and tools |

### Key types

- **`Guac`** — owns the window, tab container (including the pinned
  Preferences tab), the list of session tabs, the loaded models/roles, and the
  (lazily built) Preferences screen.
- **`preferencesScreen`** — the in-window Preferences screen: Back button,
  section navigation, and the Models / Roles / Sessions panels.
- **`sessionTab`** — bundles a chat session with its tab item, banner, and
  layout. Knows where it is displayed and which model/role it uses.
- **`sessionBanner`** — the role and model selectors at the top of a tab, plus
  the change handlers wired to them.
- **`chat`** — a single session's state: its ID, output label, input field,
  message list, and underlying `ai.Session`.
