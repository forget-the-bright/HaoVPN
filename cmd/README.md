# cmd/ 入口说明

三个二进制入口，均保持**薄**：只解析 flag、加载配置、委托 `internal/*` 编排。

| 目录 | 二进制 | 职责 |
|------|--------|------|
| `cmd/server` | `haovpn-server` | `-c server.yaml` → `serverapp.Engine.Run()` |
| `cmd/client` | `haovpn-client` | CLI 拨号；Windows `--service` → `clientapp` |
| `cmd/client-gui` | `haovpn-client-gui` | flag / UAC / 单实例 / 主题 / 服务入口 → `clientgui.Run`；亦可 `--service` |

## 通用 flag

| Flag | 说明 |
|------|------|
| `-version` | 构建版本（根目录 `VERSION` + ldflags） |
| `-c <path>` | 配置文件；client / client-gui 可省略（`config.ResolveClientConfigPath`） |

## cmd/client

- **单实例**：`singleinstance`（127.0.0.1 TCP 协调）；冲突时 `clientapp.SingleInstanceHint`（服务占用会提示 `--service stop` 或 GUI 接管）。
- **非管理员**：stderr 警告 + 日志 `cli_not_admin=true`（不自动 UAC，与 GUI 区分）。
- **Windows 服务**：`--service install|uninstall|start|stop` → `clientapp.RunServiceCommand`（CLI 薄封装；**SCM 安装/启停实现在 `internal/autostart`**，与 GUI 托盘共用）。
- **无界面运行**：argv `service`（SCM/systemd/launchd 启动）→ `RunServiceLoop`（预热、无 FailFast、持续重连）。
- **拨号**：`clientapp.RunCLI`（预热 + 首连 FailFast 45s + ICS 告警 stderr）/ 共用 `Engine`。

## cmd/client-gui

| 文件 | 职责 |
|------|------|
| `main.go` | flag、UAC 前后 Probe、Acquire 单实例、调用 `clientgui.Run` |
| `theme.go` | 可读主题，注入 `clientgui.AppTheme` |

**UI 逻辑不在 cmd**：登录/主窗/托盘/提示框均在 [`internal/clientgui`](../internal/clientgui/)。

流程概要：

1. 解析 `-c` / `-version`；非管理员可 `platform.RelaunchElevated()`。
2. UAC 前/后 `ClientAlreadyRunning`；**服务占用** → `handleOccupiedInstance`（`AskServiceTakeover` 对话框，经 `clientapp.ServiceAutostartStatus`）；**CLI/GUI 互斥** → `ShowAlreadyRunning` + `clientapp.SingleInstanceUserMessage`。
3. `clientgui.Run`：托盘、登录窗、主窗；拨号经 `PrepareEngine` + 共用 `clientapp.Engine`。

## cmd/server

- 首次启动可生成 `server.yaml`、证书、数据库。
- 数据保留：`maintenance.StartRetentionLoop`。
- 路由表见 [docs/architecture.md](../docs/architecture.md)。

## 构建

```powershell
.\scripts\build-local.ps1   # → bin/
.\scripts\build-release.ps1 # → dist/
```

## 相关文档

- [internal/README.md](../internal/README.md)
- [docs/architecture.md](../docs/architecture.md)
