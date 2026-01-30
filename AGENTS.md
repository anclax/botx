# Repository Guidelines

## Project Structure & Module Organization
- `cmd/botx`: CLI generator that turns YAML specs into Go handlers.
- `pkg/codegen`: YAML parsing and code generation logic.
- `pkg/core`: Core runtime (bot backends, sessions, rendering).
- `samples/cli`, `samples/telegram`: Example apps and configs; `samples/cli/frontend` is a simple terminal UI.
- `samples/common/botx_gen.go`: Generated handlers used by the samples.
- `doc/design/zh.md` and `generator.md`: Design notes and generator details.

## Build, Test, and Development Commands
- `go run ./cmd/botx gen -c ./samples/cli/botx.yaml -o ./samples/common/botx_gen.go`: Generate handlers from YAML.
- `go run ./samples/cli`: Run the CLI sample locally.
- `BOTX_TELEGRAM_TOKEN=... go run ./samples/telegram`: Run the Telegram sample.
- `go test ./samples/cli`: Run the end-to-end CLI test.

## Coding Style & Naming Conventions
- Go code follows standard Go conventions; use `gofmt` (tabs for indentation).
- Exported identifiers use `CamelCase`; unexported use `lowerCamel`.
- Keep handler and provider types descriptive (e.g., `MyStateProvider`, `MyFormValidator`).

## Testing Guidelines
- Tests live alongside samples (currently `samples/cli/main_test.go`).
- Use Go’s `*_test.go` naming.
- Run tests with `go test ./samples/cli` before submitting changes.

## Commit & Pull Request Guidelines
- Recent commits use short, lowercase, imperative messages (e.g., “refactor”, “update”). Keep messages concise but be more specific when possible.
- PRs should include a brief summary, the commands you ran (e.g., `go test ./samples/cli`), and note any regenerated files.
- If you change YAML specs, regenerate `samples/common/botx_gen.go` and mention it in the PR description.

## Security & Configuration Tips
- Telegram samples require `BOTX_TELEGRAM_TOKEN`; do not commit tokens or secrets.
