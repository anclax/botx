# Botx Telegram Bot Skill

Define Telegram bot behavior in `botx.yaml`, generate Go handlers, and implement business logic with Botx.

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

In short:
- Describe pages, routes, buttons, and forms in YAML.
- Generate handlers with the Botx CLI.
- Implement `StateProvider`, `FormValidator`, `CommandHandler`, and `Handler`.
- Register the generated handler with your Telegram connector.

Fallback behavior:
- Use `Handler` to handle uncaught text or callback data.

No manual edits to generated code—change YAML or source files and regenerate.
