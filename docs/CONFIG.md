# Configuration

Guacamole stores its persistent configuration in a single JSON file:

```
~/.guac/config.json
```

This file is created on first run, read at startup by both the CLI and the GUI, and written back immediately when you change settings through the GUI (Preferences → Providers, Roles, Magi, Sessions, Permissions).

---

## File structure

```json
{
  "sessionsDir": "/Users/you/.guac/session",
  "permissions": {
    "filesystem": { "allowAll": false, "directories": [] },
    "shell": { "policy": "ask" },
    "web": { "allowAll": false, "domains": [] }
  },
  "modelProviders": [
    {
      "name": "openrouter",
      "apiType": "OpenAI",
      "baseUrl": "https://openrouter.ai/api/v1",
      "apiKey": "XOR-obfuscated-string",
      "models": ["openrouter/auto", "openrouter/flash"],
      "appAttribution": true
    }
  ],
  "selectedModel": "openrouter / openrouter/auto",
  "magi": {
    "melchior": "openrouter / openrouter/auto",
    "balthasar": "openrouter / openrouter/flash",
    "casper": "openrouter / openrouter/auto"
  }
}
```

---

## Top-level fields

| Field | Type | Description |
|-------|------|-------------|
| `sessionsDir` | string | Directory where saved sessions are stored. Defaults to `~/.guac/session`. |
| `permissions` | object | Permission rules for filesystem, shell, and web access. See [Permissions](#permissions). |
| `modelProviders` | array | Configured model providers. See [Model Providers](#model-providers). |
| `selectedModel` | string | Default model for new sessions, stored as a `Provider / Model` label (or `Magi` for Magi Mode). |
| `magi` | object | Magi Mode model assignments. See [Magi Mode](#magi-mode). |

---

## Permissions

Permissions control what the model is allowed to do without asking. They are grouped into three categories, all persisted in `config.json`.

### Filesystem

Tracks read and write access per directory.

```json
"filesystem": {
  "allowAll": false,
  "directories": [
    { "directory": "/home/me/notes", "read": "allow", "write": "allow" },
    { "directory": "/etc", "read": "deny", "write": "deny" }
  ]
}
```

| Field | Type | Values | Description |
|-------|------|--------|-------------|
| `allowAll` | boolean | `true`, `false` | When `true`, all filesystem access is allowed without prompting; directory rules are ignored. |
| `directories` | array | — | List of directory rules. Each rule has `directory` (path), `read` (policy), `write` (policy). |

**Policy values:** `allow` (always allow), `ask` (prompt each time), `deny` (always deny).

- A directory rule covers that directory and everything under it.
- The most specific directory wins.
- Reads and writes are tracked separately.

### Shell

A single policy for all shell operations.

```json
"shell": { "policy": "ask" }
```

| Field | Type | Values | Description |
|-------|------|--------|-------------|
| `policy` | string | `allow`, `ask`, `deny` | Applies to every shell command (`bash`, `sh`, `zsh`). |

### Web

Tracks access per domain (including subdomains).

```json
"web": {
  "allowAll": false,
  "domains": [
    { "domain": "example.com", "policy": "allow" }
  ]
}
```

| Field | Type | Values | Description |
|-------|------|--------|-------------|
| `allowAll` | boolean | `true`, `false` | When `true`, all web access is allowed without prompting; domain rules are ignored. |
| `domains` | array | — | List of domain rules. Each rule has `domain` (e.g., `example.com` covers `api.example.com`) and `policy`. |

**Policy values:** `allow`, `ask`, `deny`.

---

## Model Providers

A **model provider** bundles everything needed to call an AI backend: a display name, API type, base URL, API key, and the list of models you want available in the session dropdown.

### Provider object

```json
{
  "name": "openrouter",
  "apiType": "OpenAI",
  "baseUrl": "https://openrouter.ai/api/v1",
  "apiKey": "XOR-obfuscated-string",
  "models": ["openrouter/auto", "openrouter/flash"],
  "appAttribution": true
}
```

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `name` | string | Yes | Display name shown in the GUI. |
| `apiType` | string | Yes | Wire API: `OpenAI`, `Anthropic`, `Mistral`, or `OpenCode`. |
| `baseUrl` | string | Yes | Base URL of the provider's API. |
| `apiKey` | string | No | API key, stored XOR-obfuscated. Optional for OpenAI-compatible endpoints (can fall back to `OPENAI_API_KEY` env var). |
| `models` | array of strings | No | Subset of the provider's catalog that appears in the session's **Provider / Model** dropdown. |
| `appAttribution` | boolean | No | When `true`, sends OpenRouter app attribution headers with every request (inference + model listing). Default `false`. |

### API types

| Type | Inference API | Model Listing | Notes |
|------|---------------|---------------|-------|
| `OpenAI` | OpenAI Responses API (`/responses`) | `GET /v1/models` | Used by OpenAI and OpenAI-compatible endpoints (OpenRouter, nVidia, HuggingFace, local servers). |
| `Anthropic` | Anthropic Messages API (`/v1/messages`) | Anthropic Models API | Requires API key. |
| `Mistral` | Mistral Chat Completions (`/v1/chat/completions`) | `GET /v1/models` (OpenAI-compatible) | |
| `Gemini` | Google Generative Language API (`models/{model}:generateContent`) | Models API (`GET /models`) | |
| `OpenCode` | OpenAI-compatible + OpenCode's own model listing | `GET /models` (custom shape) | Applied automatically by the **OpenCode** preset; not user-selectable. |

### Presets

The GUI offers presets with fixed connection details so you only enter a name (optional) and API key.

| Preset | Base URL | API Type |
|--------|----------|----------|
| `OpenCode` | `https://opencode.ai/zen/v1` | `OpenCode` |
| `openrouter` | `https://openrouter.ai/api/v1` | `OpenAI` |
| `nVidia` | `https://integrate.api.nvidia.com/v1` | `OpenAI` |
| `HuggingFace` | `https://router.huggingface.co/v1` | `OpenAI` |
| `Groq` | `https://api.groq.com/openai/v1` | `OpenAI` |
| `Mistral` | `https://api.mistral.ai/v1` | `Mistral` |
| `Gemini` | `https://generativelanguage.googleapis.com/v1beta` | `Gemini` |
| `Custom` | (you enter) | `OpenAI`, `Anthropic`, `Mistral`, or `Gemini` |

### Model listing

The **Models** button (or **Re-fetch Models**) queries the provider's catalog:

- `OpenAI` / compatible → `GET <baseUrl>/models`
- `Anthropic` → Anthropic Models API
- `Mistral` → `GET <baseUrl>/models`
- `Gemini` → `GET <baseUrl>/models` (Models API)
- `OpenCode` → `GET <baseUrl>/models` (accepts standard OpenAI shape and legacy `{"models":[…]}`; `-free` suffix marks free models)

Checked models are saved to the provider's `models` array and drive the session dropdown.

---

## Selected Model

`selectedModel` stores the default model for newly created sessions. It is a `Provider / Model` label (e.g., `openrouter / openrouter/auto`) or the virtual `Magi` label to start new sessions in Magi Mode.

The GUI updates this field whenever you switch models in a session. The CLI's `config.DefaultConfig()` resolves the model in this order:

1. The stored `selectedModel`, if it still matches a configured provider and model.
2. The first provider that lists models — using its first model.
3. The first configured provider — leaving the model empty so the provider's own default applies.

---

## Magi Mode

`magi` assigns the real model (a `Provider / Model` label) that each of the three magi plays.

```json
"magi": {
  "melchior": "openrouter / openrouter/auto",
  "balthasar": "openrouter / openrouter/flash",
  "casper": "openrouter / openrouter/auto"
}
```

| Field | Type | Description |
|-------|------|-------------|
| `melchior` | string | Model for the first magi (Melchior). |
| `balthasar` | string | Model for the second magi (Balthasar). |
| `casper` | string | Model for the third magi (Casper). |

- Values must be real `Provider / Model` labels from the available models (never `Magi`).
- All three must be assigned before Magi Mode can be selected in a session.
- Configured in the GUI under **Preferences → Magi**.

---

## API Key Obfuscation

API keys are stored XOR-obfuscated with a fixed key (`"guacamole-api-key-obfuscation"`). This prevents keys from being readable straight out of the config file, but is not encryption — anyone with the source can reverse it. Keys are re-obfuscated on every load to migrate any legacy plain-text values.

---

## Configuration Directory

| What | Location |
|------|----------|
| Config file (providers, permissions, selected model, magi) | `~/.guac/config.json` |
| Saved sessions | `~/.guac/session/<session ID>.json` |

The config directory (`~/.guac`) is created automatically on first run.

---

## Editing by Hand

You can edit `~/.guac/config.json` directly with any text editor. Changes are picked up on next startup (GUI) or next run (CLI). The GUI also watches for external changes and reloads providers/permissions automatically.

**Tip:** If you break the JSON, delete the file and restart — Guacamole will recreate it with defaults.

---

## Environment Variable Fallback

For OpenAI-compatible providers (`OpenAI` API type), if `apiKey` is empty or omitted, the CLI and GUI will fall back to the `OPENAI_API_KEY` environment variable. This allows you to keep keys out of the config file entirely.

---

## Related Docs

- [PROVIDERS.md](PROVIDERS.md) — Provider presets, API types, model listing, app attribution.
- [GUI.md](GUI.md#preferences) — GUI Preferences screen (Providers, Roles, Magi, Sessions, Permissions).
- [ARCHITECTURE.md](ARCHITECTURE.md#permissions) — Permission resolution internals.
- [FEATURES.md](FEATURES.md#magi-mode-always-get-the-best-response) — Magi Mode behavior.