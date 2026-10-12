# GUI

The GUI is a desktop application built on [Fyne](https://fyne.io) (Go). It lets
you run multiple chat sessions side by side, each with its own model and role,
and manage your providers, roles, saved sessions, and permissions.

Run it:

```sh
go run ./cmd/gui
```

Or build a binary:

```sh
go build -o guacamole_gui ./cmd/gui
```

On macOS, `./run.sh run` builds and opens a `.app` bundle (see
[Building and running](#building-and-running) below). The GUI reads its
providers, roles, sessions, and permissions from the guacamole config
(`~/.guac/config.json`) and sessions directory (`~/.guac/session`) — see
[Configuration and data locations](#configuration-and-data-locations).

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
  `openrouter / openrouter/auto`). Selecting one rebuilds the session's AI loop
  against that provider and model. At the top of the list is the virtual
  **Magi** entry: selecting it puts the session into **Magi Mode**, a panel of
  three models that answer and judge one another, keeping only the best reply
  (see [Preferences → Magi](#magi) and
  [FEATURES.md](FEATURES.md#magi-mode-always-get-the-best-response)). Magi
  Mode needs all three magi assigned, so it can only be selected once the panel
  is configured.

New sessions start on the stored default model — the model of the active
session is recorded as you switch, so the app reopens on the last model you
were using. The first role is preselected when a tab is created. The stored
default can also be seen as the first entry of the dropdown when no
preference is saved yet.

### Input and output

- The input is a **multi-line** text field, so **Enter inserts a newline**. To
  send a prompt, click **Submit**.
- On submit, the prompt is appended to the transcript as a user bubble, the
  input is cleared, and the reply is streamed back chunk by chunk into an AI
  bubble. While the request is in flight a spinner sits at the bottom with a
  red ✕ button that aborts the whole turn.
- User and AI messages are rendered as rounded, selectable bubbles (drag to
  select, then Ctrl/Cmd+C or right-click → Copy).
- If the call fails, the transcript gets an `Error: …` line instead of a reply.
  The GUI keeps running.

> **Note:** the GUI does not currently persist chat transcripts. A session's
> conversation lives in memory for the life of the window. (Saved-session files
> are managed under Preferences → Sessions, backed by `~/.guac/session`.)

### Creating and closing sessions

- **New Session tab:** the rightmost tab, always labeled **New Session**.
  Clicking it creates a fresh session and keeps **New Session** in the last
  position. Its placeholder text reads *"Click to create new session."*
- **File → New Tab** does the same thing.
- **Close a tab (✕):** removes that session. The GUI never lets you close down
  to zero sessions — closing the last one opens a fresh **Session 1**.

Session tabs are named `Session 1`, `Session 2`, … using a monotonic counter
that only ever increases, so names stay unique even as you close tabs.

### Tools

Every session's model can call the tools from the `tools` package — the
filesystem tools, one shell tool per installed shell, `http_fetch`, and
`user_question`. Each one is described in [TOOLS.md](TOOLS.md).

Tool calls run in the background as the model works. Each call is shown in
the transcript as an italic activity line the moment it starts — e.g.
`→ bash: git status` (the full command line), `→ ls /tmp`,
`→ edit_file src/main.go` (rendered by `tools.Summarize`) — and the line is
marked with the error (in red) if the call fails. The final reply text follows
once the model is done.

Permission checks apply as in the CLI: when a call needs approval, the input
area is replaced with the question and its answers as buttons until you reply.

The same prompt mechanism backs the model's `user_question` tool, the
tool-round limit, and rate-limit retries: whenever the app needs a foreground
answer, the input area is swapped for the question, its options as buttons
(plus a free-text entry when allowed), and the session blocks until you
answer. When the model keeps requesting tool calls past the session's round
limit, the buttons are **Continue** and **Abort**; when the provider rate
limits a request (HTTP 429), they are **Retry** and **Abort**.

---

## Menu

The main window menu has two top-level menus:

- **File**
  - **New Tab** — create a new session tab.
  - **Preferences** — open the Preferences screen (see below).
  - **Quit** — close the app.
- **About**
  - **About** — replaces the window with a short about screen:
    *"Guacamole GUI — A simple Fyne GUI with tabbed sessions."*

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
Sections: **Providers**, **Roles**, **Magi**, **Sessions**, **Permissions**.

### Providers

Lists every configured provider, each row showing the provider name (bold),
its API type (`OpenAI`, `Anthropic`, or `OpenCode`) and base URL, with three
buttons:

- **Edit** — opens the provider editor for that provider.
- **Models** — opens the provider's model list (see below).
- **Delete** — removes the provider (after a confirmation dialog).

Below the list is an **Add Provider** button.

#### Adding / editing a provider

The editor has a **Provider** dropdown with the presets — `OpenCode`,
`openrouter`, `nVidia`, `HuggingFace` — plus `Custom`. Picking a preset fills in
its fixed base URL and API type and locks them; **Custom** leaves every field
editable:

- **Name** — display name (always editable).
- **Base URL** — the API base URL (locked for presets).
- **API Type** — `OpenAI` or `Anthropic` (locked for presets; OpenCode is
  applied automatically by the OpenCode preset).
- **API Key** — optional for OpenAI-compatible endpoints (stored obfuscated
  in the config file).
- **Attribution** — a checkbox: *"Send app attribution headers
  (HTTP-Referer, X-OpenRouter-Title, X-OpenRouter-Categories)"*. When checked,
  every request to this provider carries those headers, so OpenRouter
  attributes the usage to Guacamole. Off by default. See
  [PROVIDERS.md](PROVIDERS.md#app-attribution).

**Save** writes the provider to the config and returns to Preferences; the
change takes effect in open sessions immediately.

#### Provider models

The **Models** screen for a provider shows every model the provider offers, in
a table with a checkbox per row (plus **Size**, **Free**, and **Description**
columns when the provider reports them). Checking or unchecking a model saves
immediately. **Re-fetch Models** re-queries the provider's model catalog;
**Save** persists the selection and returns.

The models left checked are the ones available in every session's
**Provider / Model** dropdown.

### Roles

A list of all roles. Selecting a role shows its name (bold, with its set name
in parentheses when present) and its full system prompt below the list. The
first role is selected by default. Shows *"No roles found."* if there are none.

### Magi

Configures **Magi Mode** (see [FEATURES.md](FEATURES.md#magi-mode-always-get-the-best-response)).
The screen explains the mode — *"Magi Mode chooses the best output from the
agents below"* — and then shows one model dropdown per magi, in the fixed
order **Melchior**, **Balthasar**, **Casper**. Picking a model saves it
immediately.

The dropdowns list every real model (the same list as a session's **Provider /
Model** dropdown), but deliberately **exclude** the virtual **Magi** model: a
magi cannot take a role. If no providers are configured yet, the screen says
so and no dropdowns are shown. The chosen models are stored in the guacamole
config (`~/.guac/config.json`), and changes apply to open Magi Mode sessions
when you leave Preferences.

### Sessions

Lists every saved session on disk (from `~/.guac/session`), each row showing
the session ID with a **Delete** button on the right. **Delete** removes the
file and refreshes the list. If a delete fails, an error dialog is shown.

Empty or unreadable states show guidance instead of a blank list, e.g.
*"No saved sessions in …"* or *"Could not list saved sessions: …"*.

### Permissions

Both the CLI and the GUI share one set of permission rules in
`~/.guac/config.json`, documented in the companion docs. The section edits
them directly:

- **Filesystem** — an *"Allow all filesystem access"* override checkbox, then
  one row per directory rule: a `Read` and a `Write` dropdown
  (`allow`/`ask`/`deny`) and the directory path, with a delete button per row.
  **Add Directory** appends a new rule.
- **Shell** — a single policy dropdown (`allow`/`ask`/`deny`) shared by every
  shell command.
- **Web** — an *"Allow all web access"* override checkbox, then one row per
  domain rule: a policy dropdown and the domain, with a delete button per row.
  **Add Domain** appends a new rule.
- **Reset all permissions to defaults** — clears every rule and reinstates the
  defaults (shell policy `ask`).
- **Save** — writes the rules to the config. Most changes are written
  immediately as you make them.

---

## Configuration and data locations

| What | Where |
| ---- | ----- |
| Providers, roles-independent config, permissions, selected model, magi models | `~/.guac/config.json` |
| Saved sessions | `~/.guac/session/<session ID>.json` |

The provider list is read at startup and whenever providers change, and open
sessions' dropdowns are refreshed in place. See [PROVIDERS.md](PROVIDERS.md)
for the config file shape.

---

## Building and running

`./run.sh` wraps the common tasks inside the repo:

```sh
./run.sh help                # all commands
./run.sh dependencies        # native Fyne deps for this OS (Xcode tools / OpenGL, X11 / MinGW)
./run.sh build               # host CLI + host GUI (macOS: .app); installs missing native deps first
./run.sh build:cli           # host CLI binary only
./run.sh build:gui           # host GUI binary (needs the dependencies above)
./run.sh build:macos         # macOS Guacamole.app bundle (needs a macOS host)
./run.sh run                 # build and run for the host (GUI on macOS, CLI elsewhere)
./run.sh test                # go test ./...
./run.sh clean               # remove .build/ and .DS_Store files
```

---

## Structure

| File | Responsibility |
| ---- | -------------- |
| `cmd/gui/main.go` | Entry point — creates the app and starts the event loop |
| `cmd/macos/main.go` | Entry point for the macOS `.app` bundle |
| `cmd/icongen/main.go` | Draws the app icon for the `.app` bundle |
| `app/app.go` | The `Guac` app: main window, tabs, menu, session lifecycle |
| `app/sessontab.go` | One session tab: banner, input, output, prompt handling |
| `app/preferences.go` | The Preferences screen (Providers, Roles, Magi, Sessions, Permissions) |
| `app/chatlog.go` | Message bubbles, spinner/cancel row, activity lines |
| `config` | Providers, permissions, config file load/save |
| `roles` | Loads roles (system prompts) |
| `sessionstore` | Lists/deletes saved session files |
| `ai`, `restfulai`, `anthropicai`, `tools` | The underlying AI loop, providers, and tools |

### Key types

- **`Guac`** — owns the window, tab container (including the pinned
  Preferences tab), the list of session tabs, the loaded models/roles, and the
  (lazily built) Preferences screen.
- **`preferencesScreen`** — the in-window Preferences screen: Back button,
  section navigation, and the Providers / Roles / Magi / Sessions / Permissions
  panels.
- **`sessionTab`** — bundles a chat session with its tab item, banner, and
  layout. Knows where it is displayed and which model/role it uses.
- **`sessionBanner`** — the role and model selectors at the top of a tab, plus
  the change handlers wired to them.
- **`chat`** — a single session's state: its ID, output container, input
  field, message list, active spinner/cancel row, and underlying `ai.Session`.