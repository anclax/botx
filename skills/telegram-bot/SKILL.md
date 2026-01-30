---
name: telegram-bot
description: Use Botx to define Telegram bot behavior with botx.yaml, generate Go handlers, and implement StateProvider, FormValidator, and user handlers. Apply when setting up Botx flows, wiring Telegram runtime, or customizing fallback handlers for uncaught text or callback data.
---

# Botx Telegram Bot

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

## Define behavior in botx.yaml

Keep YAML focused on routes, pages, forms, and navbar.

```yaml
package: main

navbar:
  rows:
    - columns:
        - label: Back
          onClick: back
        - label: Home
          onClick: /

handlers:
  - match: /start
    matchType: exact
    type: command
    action: router.push(ctx, "/")

pages:
  /:
    view:
      message: "Welcome"
      buttons:
        grid:
          rows:
            - columns:
                - label: "Open List"
                  onClick: /list

  /form:
    form:
      required: [title]
      fields:
        title:
          label: Title
          input:
            schema:
              type: string
          validator: validateTitle
    state:
      type: object
      required: [success, error]
      properties:
        success:
          type: boolean
        error:
          type: string
    view:
      message: "${cond(state.success, \"Saved\", state.error)}"

components:
  schemas:
    Item:
      type: object
      required: [ID, title]
      properties:
        ID:
          type: integer
          format: int64
        title:
          type: string
```

## Generate handlers

```bash
go run ./cmd/botx gen -c ./path/to/botx.yaml -o ./path/to/botx_gen.go --package main
```

Do not edit generated code directly. Update YAML or supporting source files and regenerate.

## Implement business logic

Implement the generated interfaces:

- `StateProvider`: Build view state for each page.
- `FormValidator`: Validate form input values.
- `CommandHandler`: Handle commands like `/start`.
- `Handler`: Fallback for uncaught text or callback events.

Wire everything:

```go
sm, _ := session.NewMemorySessionManager()
connector, _ := bot.NewTelegramBot(os.Getenv("BOTX_TELEGRAM_TOKEN"), sm, logger)

stateProvider := &MyStateProvider{}
formValidator := &MyFormValidator{}
defaultHandler := &MyHandler{}
commandHandler := &MyCommandHandler{}

botxgen.Register(connector, sm, stateProvider, formValidator, defaultHandler, commandHandler)
```

## Customize behavior

- Update messages, buttons, and forms in `botx.yaml`.
- Add route parameters and use them in state builders.
- Use pagination grids for list views.
- Extend validators for custom rules and error messages.
- Use navbar rows for consistent navigation.

## Handle uncaught input

Use `Handler` to cover cases not handled by YAML routes:

- `HandleTextMessage` for unknown text input.
- `HandleCallbackData` for unknown callback data.
- `HandleError` for runtime errors.
