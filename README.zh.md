![botx logo](assets/logo.svg)

[English](README.md) | 中文

# botx

像构建 Web 应用一样构建机器人：页面、路由、组件——告别 FSM 失控。
在 `botx.yaml` 中定义行为，生成 Go 处理器，专注业务逻辑。
一份 YAML 规范即可驱动 Telegram、CLI，以及任何实现 connector 的后端。

## 安装

macOS / Linux：

```bash
mkdir -p ~/.local/bin
curl -fsSL https://raw.githubusercontent.com/cloudcarver/botx/main/dev/download.sh | sh -s -- ~/.local/bin
```

Windows（PowerShell）：

```powershell
iwr -useb https://raw.githubusercontent.com/cloudcarver/botx/main/dev/install.ps1 | iex
```

安装指定版本：macOS/Linux 通过 `BOTX_VERSION`，PowerShell 通过 `-Version`（如 `v1.2.3`）。

## 工作原理

- YAML 描述页面、路由、按钮和表单。
- 生成器产出 `Register(...)`、页面/表单渲染器和接口定义。
- 你在 `StateProvider` 与 `FormValidator` 中实现业务逻辑。
- 后端（`bot_telegram.go`、`bot_cli.go`）接收更新并发送消息/表单。
- 兜底处理器覆盖无法匹配的文本/回调数据。

## 架构

- **Connector**：`bot.BotConnector` 实现（Telegram、CLI 等）。
- **User bot**：处理器使用的 `bot.Bot` 封装（`SendMessage`、`SendForm`、`Route`）。
- **生成的处理器**：负责路由、表单处理与渲染的 `Register(...)`。
- **StateProvider**：为视图构建页面状态。
- **FormValidator**：在提交前校验表单输入。
- **Renderer**：通过 connector 发送 `bot.Message` / `bot.Form`。

## 最小使用

1) 编写 YAML 配置（参考 `samples/cli/botx.yaml`）。
2) 生成 Go 代码：

```bash
go run ./cmd/botx gen -c ./path/to/botx.yaml -o ./path/to/botx_gen.go
```

3) 在应用中注册生成的处理器：

```go
sm, _ := session.NewMemorySessionManager()
connector, _ := bot.NewTelegramBot(os.Getenv("BOTX_TELEGRAM_TOKEN"), sm, logger)

stateProvider := &MyStateProvider{}
formValidator := &MyFormValidator{}
defaultHandler := &MyHandler{}
commandHandler := &MyCommandHandler{}

botxgen.Register(connector, sm, stateProvider, formValidator, defaultHandler, commandHandler)
```

后端会将文本/回调事件路由到生成的处理器，处理器通过 `bot.Message` 和 `bot.Form` 渲染页面和表单。

## 开发流程

1) 在 YAML 中定义行为。
2) 使用 `cmd/botx` 生成 Go 代码。
3) 实现 `StateProvider`、`FormValidator`，以及可选的命令/兜底处理器。
4) 在目标后端注册生成的处理器。
5) 运行机器人并迭代。

## 国际化 (i18n)

- 在 YAML 中定义翻译并设置默认语言。
- 在消息/标签表达式中使用 `${content.key}`。
- 语言解析顺序：会话偏好 → 入站消息语言 → `i18n.default`。
- 通过回调数据 `lang:zh-hans` 切换语言。

示例：

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

## 示例

CLI 示例（包含一个简单的终端前端）：

```bash
go run ./samples/cli
```

然后输入 `/start` 开始。

Telegram 示例（需要 bot token）：

```bash
BOTX_TELEGRAM_TOKEN=... go run ./samples/telegram
```

## 测试

运行 CLI 端到端测试：

```bash
go test ./samples/cli
```

## 仓库结构

- `doc/design/zh.md`：设计思路与 YAML 示例。
- `pkg/codegen`：YAML 解析器与代码生成器。
- `samples/common/botx_gen.go`：示例使用的生成处理器。
- `pkg/core/bot`：Bot 抽象与后端（`bot_telegram.go`、`bot_cli.go`）。
- `samples/cli/frontend`：用于交互测试的 CLI 前端。
- `pkg/core/session`：会话接口与内存实现。
- `cmd/botx`：生成器 CLI。
- `samples/cli`：端到端 CLI 示例 + YAML 配置。
- `samples/telegram`：Telegram 示例。

## 备注

## 为什么选择 Botx

- **规范清晰**：YAML 体积小且语义明确。
- **模板化管道**：路由、解析、渲染由工具生成。
- **后端无关**：一份规范可用于 Telegram、CLI 或自定义后端。
- **显式兜底**：未捕获消息有统一处理器。

## 概念

**页面与路由**
- 每个页面就是一条路由（如 `/`、`/todo/{ID}`），支持参数。

**StateProvider**
- 构建渲染页面所需的数据，类似“视图模型”。

**表单与校验**
- 表单是多步骤输入；校验器对每个字段做规则检查。

**导航**
- 按钮通过 `bot.CallbackData` 触发回调数据。
- 使用 `route:/path` 做路由，`lang:xx` 做语言切换。
- 在处理器中可使用 `Bot.Route(ctx, chatID, "/path")`。
- 可以全局追加 `navbar` 以保持一致导航。

**兜底处理器**
- `Handler` 接收未知文本或回调数据。
- 适合帮助消息、自定义命令与安全兜底。

## 自定义建议

- 列表类页面可配合 `pagination` 网格做分页。
- 在 `components.schemas` 中定义共享 schema 以复用。
- 视图格式留在 YAML，业务逻辑留在 provider。
- 优先改 YAML，避免直接修改生成代码。

## 备注

- `samples/common/botx_gen.go` 为演示/测试用途纳入版本控制。
- CLI 后端用于快速迭代，再接入生产消息后端。
