# Providers

Guacamole (CLI and GUI) talks to AI backends through **model providers**. A
provider bundles a display name, an API type, a base URL, and an API key. The
provider list lives in `~/.guac/config.json` and is managed from the GUI
(Preferences → Providers) or edited by hand.

## API types

`apiType` chooses the wire API — both for inference and for model listing:

| Type | API | Notes |
| ---- | --- | ----- |
| `OpenAI` | OpenAI Responses API (`/responses`) | Also used by any OpenAI-compatible endpoint (OpenRouter, nVidia, HuggingFace, local servers) |
| `Anthropic` | Anthropic Messages API (`/v1/messages`) | Model listed through the Anthropic Models API |
| `OpenCode` | OpenAI-compatible inference + OpenCode's own model listing | Applied automatically by the OpenCode preset; not user-selectable |

## Presets

The provider dropdown offers presets whose connection details are fixed, so
they only need a name (optional) and an API key. Everything else is entered
by hand via **Custom**.

| Preset | Base URL | API type |
| ------ | -------- | -------- |
| `OpenCode` | `https://opencode.ai/zen/v1` | `OpenCode` |
| `openrouter` | `https://openrouter.ai/api/v1` | `OpenAI` |
| `nVidia` | `https://integrate.api.nvidia.com/v1` | `OpenAI` |
| `HuggingFace` | `https://router.huggingface.co/v1` | `OpenAI` |
| `Custom` | whatever you enter | `OpenAI` or `Anthropic` |

An API key is optional for OpenAI-compatible endpoints (the config can fall
back to the `OPENAI_API_KEY` environment variable); providers like Anthropic
require one.

## Config file

Providers are stored under `modelProviders` in `~/.guac/config.json`:

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
      "apiKey": "sk-…",
      "models": ["openrouter/auto", "openrouter/flash"],
      "appAttribution": true
    }
  ],
  "selectedModel": "openrouter / openrouter/auto"
}
```

- `apiKey` is stored XOR-obfuscated so it is not readable straight out of the
  file.
- `models` is the checked subset of the provider's catalog (see below) and
  drives the session's **Provider / Model** dropdown.
- `appAttribution` is the opt-in app attribution flag (see below).
- `selectedModel` is the default model for new sessions — the GUI updates it
  as you switch models.

The file is read at startup; GUI edits save it immediately.

### Permissions

The same file holds the `permissions` rules both frontends share. See
[ARCHITECTURE.md](ARCHITECTURE.md#permissions) for the full rules, or edit
them from the GUI (Preferences → Permissions).

## App attribution

OpenRouter attributes usage to an app from three headers, documented at
[openrouter.ai/docs/app-attribution](https://openrouter.ai/docs/app-attribution):

- `HTTP-Referer: https://guacamole.kncept.com`
- `X-OpenRouter-Title: Guacamole`
- `X-OpenRouter-Categories: cli-agent,programming-app`

Each provider has an opt-in **Attribution** checkbox in its editor
(Preferences → Providers → Edit). When checked, every request to that provider
— inference and model listing — carries the three headers. It is off by
default, and harmless for providers that do not recognize the headers. The
values are fixed and shared by every configured provider.

## Model selection

The **Provider / Model** dropdown is built from the configured providers: one
option per provider per listed model, labeled `Provider / Model`. A provider
with no listed models does not appear.

When no model is explicitly selected, the CLI's `config.DefaultConfig()`
resolves one in this order:

1. The stored `selectedModel`, when it still matches a configured provider and
   model;
2. The first provider that lists models — using its first model;
3. The first configured provider — leaving the model empty so the provider's
   own default applies.

## Model listing

The provider's catalog is fetched with the **Models** button on a provider (or
the **Re-fetch Models** button on the model list screen), then checked models
are saved to `models`:

- `OpenAI` / compatible — `GET <baseUrl>/models` (the standard OpenAI listing).
- `Anthropic` — the Anthropic Models API.
- `OpenCode` — `GET <baseUrl>/models`, accepting both the standard OpenAI
  shape and the legacy `{"models":[…]}` shape; model IDs ending in `-free` are
  marked free.

Listing requests carry the provider's app attribution headers too, when
enabled.