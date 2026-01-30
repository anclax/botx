![botx logo](assets/logo.svg)

English | [中文](README.zh.md)

# botx

Build bots like web apps: pages, routes, components—no FSM sprawl.
Define behavior in `botx.yaml`, generate Go handlers, and focus on business logic.
One YAML spec powers Telegram, CLI, and any backend that implements the connector.

## Install

macOS / Linux:

```bash
mkdir -p ~/.local/bin
curl -fsSL https://raw.githubusercontent.com/cloudcarver/botx/main/dev/download.sh | sh -s -- ~/.local/bin
```

Windows (PowerShell):

```powershell
iwr -useb https://raw.githubusercontent.com/cloudcarver/botx/main/dev/install.ps1 | iex
```

Install a specific version by setting `BOTX_VERSION` (macOS/Linux) or `-Version` (PowerShell), e.g. `v1.2.3`.

## How it works

- YAML describes pages, routes, buttons, and forms.
- The generator emits `Register(...)`, page/form renderers, and interfaces.
- You implement business logic in `StateProvider` and `FormValidator`.
- A backend (`bot_telegram.go`, `bot_cli.go`) delivers updates and sends messages/forms.
- Fallback handlers cover uncaught text/callback data.

## Architecture

- **Connector**: `bot.BotConnector` implementation (Telegram, CLI, etc.).
- **User bot**: `bot.Bot` wrapper used by handlers (`SendMessage`, `SendForm`, `Route`).
- **Generated handler**: wires routing, form handling, and rendering (`Register(...)`).
- **StateProvider**: builds page state for views.
- **FormValidator**: validates form inputs before submission.
- **Renderer**: sends `bot.Message` / `bot.Form` via the connector.

## Minimal usage

1) Write a YAML config (see `samples/cli/botx.yaml` for reference).
2) Generate Go code:

```bash
go run ./cmd/botx gen -c ./path/to/botx.yaml -o ./path/to/botx_gen.go
```

3) Register the generated handler in your app:

```go
sm, _ := session.NewMemorySessionManager()
connector, _ := bot.NewTelegramBot(os.Getenv("BOTX_TELEGRAM_TOKEN"), sm, logger)

stateProvider := &MyStateProvider{}
formValidator := &MyFormValidator{}
defaultHandler := &MyHandler{}
commandHandler := &MyCommandHandler{}

botxgen.Register(connector, sm, stateProvider, formValidator, defaultHandler, commandHandler)
```

The backend routes text/callback events to generated handlers, which render pages and forms using `bot.Message` and `bot.Form`.

## Development workflow

1) Define behavior in YAML.
2) Generate Go code with `cmd/botx`.
3) Implement `StateProvider`, `FormValidator`, and optional command/default handlers.
4) Register the generated handler on your chosen backend.
5) Run the bot and iterate.

## Internationalization (i18n)

- Define translations in YAML and set a default language.
- Use `${content.key}` in message/label expressions.
- Language resolution order: session preference → incoming message language → `i18n.default`.
- Switch language by sending callback data like `lang:zh-hans`.

Example:

```yaml
i18n:
  default: en
  content:
    hello:
      en: "Hello"
      zh-hans: "你好"

pages:
  /:
    view:
      message: ${content.hello}
      buttons:
        grid:
          rows:
            - columns:
                - label: 中文
                  onClick: lang:zh-hans
                - label: English
                  onClick: lang:en
```

## Samples

CLI sample (includes a simple terminal frontend):

```bash
go run ./samples/cli
```

Then type `/start` to begin.

Telegram sample (requires a bot token):

```bash
BOTX_TELEGRAM_TOKEN=... go run ./samples/telegram
```

## Tests

Run the CLI end-to-end test:

```bash
go test ./samples/cli
```

## Repository layout

- `doc/design/zh.md`: Design rationale and YAML examples.
- `pkg/codegen`: YAML parser + code generator.
- `samples/common/botx_gen.go`: Generated bot handlers used by the samples.
- `pkg/core/bot`: Bot abstraction and backends (`bot_telegram.go`, `bot_cli.go`).
- `samples/cli/frontend`: Sample CLI frontend for interactive testing.
- `pkg/core/session`: Session interfaces and in-memory implementation.
- `cmd/botx`: Generator CLI.
- `samples/cli`: End-to-end CLI sample + YAML config.
- `samples/telegram`: Telegram sample.

## Notes

## Why Botx

- **Readable specs**: YAML stays small and expressive.
- **Generated plumbing**: routing, parsing, rendering are handled for you.
- **Backend-agnostic**: one spec works for Telegram, CLI, or custom backends.
- **Explicit escape hatches**: fallback handlers for uncaught messages.

## Concepts

**Pages and routes**
- Each page is a route (e.g. `/`, `/todo/{ID}`) with parameters.

**StateProvider**
- Builds the data for rendering pages. Think “view model.”

**Forms and validators**
- Forms are multi-step inputs; validators enforce rules per field.

**Navigation**
- Buttons trigger callback data via `bot.CallbackData`.
- Use `route:/path` for routing and `lang:xx` for language switching.
- Use `Bot.Route(ctx, chatID, "/path")` in handlers for convenience.
- `navbar` can be appended globally for consistent navigation.

**Handlers (fallbacks)**
- `Handler` receives unknown text or callback data.
- Great for help messages, custom commands, and safety nets.

## Customization tips

- Add pagination for lists with `pagination` grids.
- Use shared schemas in `components.schemas` for reuse.
- Keep view formatting in YAML, keep logic in providers.
- Prefer YAML changes; regenerate instead of editing generated code.

## Notes

- `samples/common/botx_gen.go` is checked in for demo/testing.
- The CLI backend is intended for quick iteration before wiring a production messaging backend.
