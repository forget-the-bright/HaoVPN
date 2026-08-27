# cmd/ 入口说明

三个二进制入口，均保持**薄**：只解析 flag、加载配置、委托 `internal/*` 编排。

| 目录 | 二进制 | 职责 |
|------|--------|------|
| `cmd/server` | `haovpn-server` | 现场服务端：`-c server.yaml` → `serverapp.Engine.Run()` |
| `cmd/client` | `haovpn-client` | 工程师 CLI 拨号；Windows 支持 `--service` 安装/卸载 |
| `cmd/client-gui` | `haovpn-client-gui` | Fyne 桌面 GUI：登录、日志、托盘、UAC 提权 TUN |

## 通用 flag

| Flag | 说明 |
|------|------|
| `-version` | 打印构建版本（来自根目录 `VERSION` + ldflags） |
| `-c <path>` | 配置文件；client / client-gui 默认可省略（`ResolveClientConfigPath`） |

## cmd/client 特有

- **单实例锁**：`singleinstance.AcquireClient()`，重复启动提示并退出。
- **Windows 服务**：`--service install|uninstall|start|stop`（见下方「服务代码位置」）。
- **凭据**：`clientapp.ResolveCredentials`（yaml / DPAPI / `HAOVPN_PASSWORD`）。

### 服务代码位置

| 文件 | 说明 |
|------|------|
| `cmd/client/service_windows.go` | Windows 服务 install/start/stop/uninstall（`golang.org/x/sys/windows/svc`） |
| `cmd/client/service_other.go` | 非 Windows 空实现（`runServiceCommand` 直接返回 false） |

服务逻辑**仅在 CLI 客户端**；GUI 不支持 `--service`，工程师自连用 CLI 安装服务。

## cmd/client-gui 特有

### 文件结构

| 文件 | 职责 |
|------|------|
| `main.go` | `main`、单实例锁、UAC 提权、`uiApp` 状态机 |
| `theme.go` | `readableTheme`（高对比度可读主题） |

### uiApp 流程（`main.go`）

1. **启动**：解析 `-c` / `-version`；非管理员时 `platform.RelaunchElevated()`。
2. **单实例**：锁失败则弹窗提示（Fyne dialog）并退出。
3. **登录窗** `showLogin`：服务器/用户名/密码、杀开关勾选、提权提示 `elevHint`。
4. **连接** `tryConnect`：`clientapp.NewEngine` + `ResolveCredentials` + `Start()`。
5. **主窗** `showMain`：状态/ VPN IP / 日志区、托盘菜单、定时轮询 `startPoll`。
6. **退出** `shutdown`：停止引擎、释放单实例锁。

- 日志：`logger.SetSink` 重定向到 GUI 文本框（`appendLog`）。
- **不重复实现拨号**：共用 `clientapp.Engine`，与 CLI 行为一致。

## cmd/server 特有

- 首次启动可自动生成 `server.yaml`、证书、数据库（`config.LoadServer`）。
- 管理 API 默认 `127.0.0.1:8080`；数据保留由 `maintenance.StartRetentionLoop` 后台执行。
- 详见 [docs/architecture.md](../docs/architecture.md) HTTP 路由表。

## 构建

```powershell
.\scripts\build-local.ps1   # → bin/
.\scripts\build-release.ps1 # → dist/
```

## 相关文档

- [internal/README.md](../internal/README.md) — 改功能去哪找
- [docs/architecture.md](../docs/architecture.md) — CODEMAP
