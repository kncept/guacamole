# Tools

The `tools` package implements the functions a model can call. All of them are
registered on every `ai.Session` (see the `ai` docs), and each one that
touches something outside the conversation is checked against the matching
permission category first — filesystem, shell, or web.

`tools.AllTools(checker, userInput)` returns the full set:

| Group | Tools |
| ----- | ----- |
| Filesystem | `read_file`, `ls`, `write_file`, `glob`, `edit_file`, `create_directory`, `move_file`, `list_allowed_directories` |
| Console | one shell tool per installed shell on the `PATH` — `bash`, `sh`, and/or `zsh` |
| Web | `http_fetch` |
| User | `user_question` (only when a non-nil `userInput` is passed) |

> The permission categories and the `allow`/`ask`/`deny` model are documented
> in [PROVIDERS.md](PROVIDERS.md#permissions) (the config shape) and in the
> package docs for `permissions`.

---

## Filesystem tools

Every filesystem tool checks the **filesystem** permission category against
its path before acting — `read` for reads and listings, `write` for writes,
creates, edits, and moves. A nil `AccessChecker` (no permissions) allows
everything.

### `read_file`

Reads a file's contents.

| Parameter | Type | Required | Notes |
| --------- | ---- | -------- | ----- |
| `path` | string | yes | Path of the file to read |

The result is capped at 64 KiB. Larger files return the first 64 KiB with a
truncation note; empty files return `(empty file)`.

### `ls`

Lists a directory's entries: permissions, size in bytes, name (directories are
suffixed with `/`).

| Parameter | Type | Required | Notes |
| --------- | ---- | -------- | ----- |
| `path` | string | no | Directory to list; defaults to `.` |

Empty directories return `(empty directory)`.

### `write_file`

Writes content to a file, creating parent directories as needed.

| Parameter | Type | Required | Notes |
| --------- | ---- | -------- | ----- |
| `path` | string | yes | Path of the file to write |
| `content` | string | yes | Content to write |

### `glob`

Finds files matching a pattern using [doublestar](https://github.com/bmatcuk/doublestar).

| Parameter | Type | Required | Notes |
| --------- | ---- | -------- | ----- |
| `pattern` | string | yes | Doublestar glob pattern |
| `path` | string | no | Base directory to search from (default: current directory) |

Pattern syntax:

- `*` — any sequence of non-path-separators
- `**` — zero or more directories (surrounded by separators, like `/**/`)
- `?` — any single non-path-separator
- `[class]` — character classes (ranges, negation with `^` or `!`)
- `{alt1,...}` — alternation
- `\` — escapes special characters

Matches are sorted. `(no matches)` when nothing matches.

### `edit_file`

Edits a file by replacing content, in either `string` (literal) or `regex`
mode.

| Parameter | Type | Required | Notes |
| --------- | ---- | -------- | ----- |
| `path` | string | yes | Path of the file to edit |
| `mode` | string | yes | `string` for exact replacement, `regex` for pattern replacement |
| `old` | string | yes | Text or regex pattern to find |
| `new` | string | yes | Text to replace with |
| `replaceAll` | boolean | no | Replace all occurrences (default false) |

In both modes, `old` must match exactly once unless `replaceAll` is set.
Returns `(no changes)` when the replacement made no difference.

### `create_directory`

Creates a directory, including parent directories.

| Parameter | Type | Required | Notes |
| --------- | ---- | -------- | ----- |
| `path` | string | yes | Path of the directory to create |

### `move_file`

Moves or renames a file or directory. Permissions are checked on **both**
source and destination.

| Parameter | Type | Required | Notes |
| --------- | ---- | -------- | ----- |
| `source` | string | yes | Source path |
| `destination` | string | yes | Destination path |

### `list_allowed_directories`

Reports the filesystem permission subset — the `allowAll` override and the
per-directory `read`/`write` rules — i.e. what the model may touch without
being asked. Takes no arguments.

---

## Console tools

One tool per shell found on the `PATH`: `bash`, `sh`, and/or `zsh`. Each runs
a single command line through `sh -c <command>`-style invocation.

| Parameter | Type | Required | Notes |
| --------- | ---- | -------- | ----- |
| `command` | string | yes | The command line to execute |

Every command is checked against the **shell** permission category first: one
`allow`/`ask`/`deny` policy shared by all shell operations, across every
shell. Standard output and error are captured and returned together.

---

## Web tools

### `http_fetch`

Fetches a URL over HTTP(S), like a minimal `curl`. The request's domain — and
every redirect target — is checked against the **web** permission category
before the request is made. Requests time out after 30 seconds and follow at
most 10 redirects.

| Parameter | Type | Required | Notes |
| --------- | ---- | -------- | ----- |
| `url` | string | yes | Absolute `http://` or `https://` URL |
| `method` | string | no | HTTP method to use (default: `GET`) |
| `headers` | object | no | Extra request headers, name/value string pairs |
| `body` | string | no | Request body (mainly for POST/PUT) |

The response is returned as `HTTP <status>` followed by the body, capped at
64 KiB (with a truncation note). A default `User-Agent` of
`guacamole-http_fetch/1.0` is sent unless the caller sets one.

---

## User tools

### `user_question`

Lets the model ask the user a question **in the foreground**, through the same
`RequestForegroundUserInput` as every other prompt (permission checks,
tool-round limits, rate-limit retries). Only registered when the caller passes
a real user input.

| Parameter | Type | Required | Notes |
| --------- | ---- | -------- | ----- |
| `question` | string | yes | Question to ask the user |
| `responses` | string[] | yes | Suggested responses, at most 8 |
| `freetext` | boolean | yes | Whether the user may type a free-form answer |

---

## Displaying tool calls

`tools.Summarize(name, args)` renders one tool call as a short
human-readable line for the transcript next to the user's prompt. Shell calls
carry the **full command line** (so the user sees exactly what runs); file
tools carry their path. Unrecognized calls fall back to a compact rendering of
their string arguments, truncated to 80 characters. Both the REPL and the GUI
use it to show `→ bash: git status`, `→ ls /tmp`, `→ edit_file src/main.go`
and so on as calls run.