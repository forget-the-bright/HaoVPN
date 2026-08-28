# internal/ 包索引

> **改代码去哪**：按功能查下表；分层与依赖规则见 [docs/architecture.md](../docs/architecture.md)。

---

## 改 X 功能来哪（FAQ）

| 想改什么 | 去这里 |
|----------|--------|
| 管理 API 路由 / 健康 / 备份 | `api/handler.go` |
| API 标准错误 JSON / since 解析 | `api/httputil.go` |
| 用户 CRUD / 删除账号 | `api/users.go` → `vpnaccount.DeleteAccount` |
| 管理员重置用户密码 | `api/users.go` → `POST /api/v1/users/{id}/password` |
| 登录 / Session / CSRF | `api/auth_handlers.go`；实现 `auth/login.go`、`session.go` |
| 隧道密码校验 | `auth/tunnel_login.go`（`VerifyTunnelLogin`） |
| 分页 limit/offset | `paginate/clamp.go` |
| 监控页 JSON | `readmodel/monitor.go`、`api/monitor_handler.go` |
| 数据保留 / 定时清理 | `maintenance/retention.go`；默认天 `config/retention.go` |
| Web 开户 / IP 分配 | `vpnaccount/provision.go`、`service.go` |
| 握手 / 策略下发 | `tunnel/handshake.go`、`server_handler.go` |
| 客户端拨号 / 重连 | `clientapp/engine.go` |
| TUN / 路由 / DNS 运行时 | `clientapp/runtime.go` |
| 桌面 GUI（Fyne） | `clientgui/`（`cmd/client-gui` 仅入口） |
| 导出客户端 YAML | `config/client_export.go`（`api/export.go` 薄封装） |
| GUI 写回 yaml | `config/client_yaml_patch.go` |
| 敏感文件原子写 | `fileutil/atomic.go` |
| SQLite 时间格式 | `timeutil/sqlite.go` |
| 路由 / DNS / 杀开关 | `netstack/` → `platform.Command` |
| TUN 设备 | `tun/` |
| Windows 网卡 / netsh | `winnet/` |
| CIDR / 地址 / 网关 | `netutil/` |
| SQLite CRUD | `persist/store.go`、`users.go`、`session_store.go`、`query_ext.go` |
| TLS / 证书 / 私钥加密 | `security/` |
| YAML 默认值 | `config/client.go`、`server.go` |
| Windows 服务 | `clientapp/service_windows.go`（CLI `--service`） |
| UAC 提权 | `platform/` |
| 单实例锁 | `singleinstance/lock.go` |

---

## 分层速览

```
clientapp / clientgui / serverapp
    ├── api ──► vpnaccount
    ├── tunnel ──► tun
    ├── netstack ──► platform
    ├── maintenance
    └── persist + auth + sessionmgr
netutil / winnet / fileutil / timeutil / paginate / readmodel / security / config
```

---

## 包一览

| 包 | 职责 | 关键文件 |
|----|------|----------|
| **clientapp** | CLI/GUI 共用拨号引擎 | `engine.go`, `runtime.go`, `credentials.go` |
| **clientgui** | Fyne 桌面 UI | `run.go`, `login.go`, `app.go`, `tray.go`, `notice.go` |
| **serverapp** | 服务端启动编排 | `engine.go` |
| **api** | HTTP 管理 API + WebUI | `handler.go`, `users.go`, `httputil.go`, `export.go` |
| **vpnaccount** | IP 模式、开户、删号 | `service.go`, `provision.go`, `delete.go` |
| **tunnel** | 握手协议 | `handshake.go`, `server_handler.go` |
| **transport** | TLS-TCP 帧、重连 | `transport.go`, `frame.go`, `reconnect.go` |
| **sessionmgr** | 在线会话、报文路由 | `manager.go`, `register.go`, `kick.go`, `route.go`, `stats.go` |
| **netstack** | 路由/DNS/杀开关 | `route_*.go`, `dns_*.go`, `killswitch_*.go` |
| **tun** | TUN 设备抽象 | `tun.go`, `tun_windows.go` |
| **winnet** | Windows 网卡/netsh | `resolver_windows.go`, `netsh_windows.go` |
| **platform** | UAC、无窗口子进程 | `elevate_windows.go`, `cmderr.go` |
| **paginate** | 分页纯函数 | `clamp.go` |
| **maintenance** | 数据保留后台 | `retention.go` |
| **persist** | SQLite | `store.go`, `users.go`, `audit_store.go`, `session_store.go` |
| **readmodel** | Web/API DTO | `types.go`, `monitor.go` |
| **auth** | Web Session + 隧道登录 | `service.go`, `login.go`, `tunnel_login.go`, `session.go` |
| **ippool** | VPN IPv4 池 | `pool.go` |
| **netutil** | CIDR/地址/MTU | `cidr.go`, `addr.go`, `gateway.go` |
| **config** | YAML 加载/导出 | `config.go`, `client_export.go`, `client_yaml_patch.go` |
| **security** | TLS、密钥加密 | `tls_client.go`, `datakey.go`, `keyenc.go` |
| **health** | 启动自检 | `health.go` |
| **logstore** | 结构化历史日志 | `logstore.go` |
| **audit** | 管理审计 | `audit.go` |
| **logger** | 分级日志 | `logger.go` |
| **crypto** | 隧道加解密 | `wg_crypto.go` |
| **credentials** | Windows DPAPI | `windows.go` |
| **fileutil** | 文件系统工具 | `mkdir.go`, `atomic.go`, `exe.go` |
| **timeutil** | SQLite 时间文本 | `sqlite.go` |
| **safeutil** | GoSafe、Ticker | `goroutine.go`, `ticker.go` |
| **singleinstance** | 客户端单实例 | `lock.go` |
| **brand** | 产品名/路径常量 | `brand.go` |
| **version** | 构建版本 | `version.go` |

每个包均有中文 `doc.go`；注释规范见 [docs/comment-style.md](../docs/comment-style.md)。

---

## 第七轮架构要点（2026-08-28）

- **fileutil**：`WriteFileAtomic`、`ExecutableDir`；配置/凭据/密钥/wintun 敏感写盘统一原子写。
- **timeutil**：SQLite UTC layout；persist/logstore 共用，避免 logstore→persist。
- **config**：`BuildClientExportYAML`、`DefaultRetentionDays`；api 导出只做薄封装。
- **helper 统一**：`writeAPIError`、`parseSinceQuery`、`CommandOutputError`、`RunTickerStop`、`AlreadyRunningMessage`。
- **胖文件同包拆分**：auth / persist / sessionmgr（导出 API 不变）。
- **文档**：GUI 逻辑在 `clientgui`；单实例为 TCP 协调（非文件锁）。
