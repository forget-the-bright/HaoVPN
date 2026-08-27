# internal/ 包索引

> **改代码去哪**：按功能查下表；分层与依赖规则见 [docs/architecture.md](../docs/architecture.md)。

---

## 改 X 功能来哪（FAQ）

| 想改什么 | 去这里 |
|----------|--------|
| 管理 API 路由 / 健康 / 备份 | `api/handler.go` |
| 用户 CRUD / 删除账号 | `api/users.go` → 业务 `vpnaccount.DeleteAccount` |
| 管理员重置用户密码 | `api/users.go` → `POST /api/v1/users/{id}/password`；Web `/users` 改密按钮 |
| 登录 / Session / CSRF | `api/auth_handlers.go` |
| 分页 limit/offset 规范化 | `paginate/clamp.go`（api、persist、logstore 共用） |
| 监控页 JSON 形状 | `readmodel/monitor.go`、`api/monitor_handler.go` |
| 数据保留 / 定时清理 | `maintenance/retention.go`（由 `serverapp` 启动） |
| Web 开户 / IP 分配 | `vpnaccount/provision.go`、`service.go` |
| 删号踢线 + 释放 IP | `vpnaccount/delete.go` |
| 握手 / 策略下发 | `tunnel/handshake.go`、`server_handler.go` |
| 服务端 TUN 读写 | `tunnel/server_handler.go`（`tun.Device` 字段） |
| 客户端拨号 / 重连 | `clientapp/engine.go` |
| TUN / 路由 / DNS 运行时 | `clientapp/runtime.go` |
| 路由 / DNS / 杀开关 | `netstack/route_*.go`、`dns_*.go`、`killswitch_*.go` |
| 无窗口 route/netsh 子进程 | `netstack/` → `platform.Command` |
| TUN 设备抽象 | `tun/tun.go`、`tun_windows.go` |
| Windows Wintun 适配器/日志 | `tun/wintun_adapter_windows.go`、`wintun_log_windows.go` |
| Windows 网卡 / netsh | `winnet/` |
| CIDR / 地址 / 网关 | `netutil/` |
| SQLite CRUD / 分页查询 | `persist/store.go`、`query_ext.go` |
| 行扫描 / JSON 列 / 时间格式 | `persist/scan.go`、`jsoncol.go`、`timefmt.go` |
| TLS / 证书 / 私钥加密 | `security/` |
| YAML 默认值 | `config/client.go`、`server.go` |
| 桌面 GUI（Fyne） | `cmd/client-gui/`（逻辑在 `main.go` + `theme.go`） |
| Windows 服务 install | `cmd/client/service_windows.go` |
| UAC 提权 | `platform/`（GUI 与 netstack 共用） |

---

## 分层速览

```
clientapp / serverapp          # 应用编排
    ├── api ──► vpnaccount     # HTTP 不直接碰 ippool
    ├── tunnel ──► tun         # 服务端 Handler 持有 tun.Device
    ├── netstack ──► platform  # 无窗口子进程执行 route/netsh
    ├── maintenance            # 后台保留清理（与 api 解耦）
    └── persist + paginate     # SQL 与分页参数
netutil / winnet / fileutil / readmodel / security / config  # 无业务状态公共能力
```

---

## 包一览

| 包 | 职责 | 关键文件 |
|----|------|----------|
| **clientapp** | CLI/GUI 共用拨号引擎 | `engine.go`, `runtime.go`, `credentials.go` |
| **serverapp** | 服务端启动编排 | `engine.go` |
| **api** | HTTP 管理 API + WebUI | `handler.go`, `users.go`, `auth_handlers.go`, `httputil.go`, `monitor_handler.go`, `logs.go` |
| **vpnaccount** | IP 模式、开户、删号 | `service.go`, `provision.go`, `delete.go` |
| **tunnel** | 握手协议 | `handshake.go`, `server_handler.go`, `client_handshake.go` |
| **transport** | TLS-TCP 帧、重连 | `transport.go`, `frame.go`, `reconnect.go` |
| **sessionmgr** | 在线会话、报文路由 | `manager.go`, `conn.go` |
| **netstack** | 路由/DNS/杀开关 | `route_*.go`, `dns_*.go`, `killswitch_*.go` |
| **tun** | TUN 设备抽象 | `tun.go`, `tun_windows.go` |
| **winnet** | Windows 网卡/netsh | `resolver_windows.go`, `netsh_windows.go` |
| **platform** | UAC 提权、无窗口子进程 | `elevate_windows.go`, `cmd_windows.go`, `cmderr.go` |
| **paginate** | 分页 limit/offset 纯函数 | `clamp.go` |
| **maintenance** | 数据保留后台任务 | `retention.go` |
| **persist** | SQLite | `store.go`, `query_ext.go`, `scan.go`, `jsoncol.go`, `timefmt.go` |
| **readmodel** | Web/API DTO | `types.go`, `monitor.go` |
| **auth** | Web Session | `user.go` |
| **ippool** | VPN IPv4 池 | `pool.go` |
| **netutil** | CIDR/地址/监听/MTU | `cidr.go`, `addr.go`, `gateway.go` |
| **config** | YAML 加载/默认值 | `config.go`, `client.go`, `server.go` |
| **security** | TLS、密钥加密 | `tls_client.go`, `cert.go`, `keyenc.go` |
| **health** | 启动自检 | `health.go` |
| **logstore** | 结构化历史日志 | `logstore.go` |
| **audit** | 管理审计 | `audit.go` |
| **logger** | 分级日志 | `logger.go` |
| **crypto** | 隧道加解密 | `wg_crypto.go` |
| **credentials** | Windows DPAPI | `windows.go` |
| **fileutil** | 文件系统小工具 | `mkdir.go` |
| **safeutil** | GoSafe、Ticker | `goroutine.go`, `ticker.go` |
| **singleinstance** | 客户端单实例锁 | `lock.go` |
| **brand** | 产品名/路径常量 | `brand.go` |
| **version** | 构建版本 | `version.go` |

每个包均有中文 `doc.go`；注释规范见 [docs/comment-style.md](../docs/comment-style.md)。

---

## 第五轮架构要点（2026-08-27）

- **paginate**：从 api/httputil 抽出 `ClampLimit`/`ClampOffset`，persist、logstore 复用。
- **persist 辅助**：`query_ext.go`（分页列表）、`scan.go`、`jsoncol.go`、`timefmt.go` 减轻 store 膨胀。
- **vpnaccount.DeleteAccount**：删号踢线 + 按 `ip_mode` 释放 IP；api 只调 Service。
- **maintenance**：保留清理从 api 迁至独立包，由 serverapp 启动 ticker。
- **api 解耦**：生产代码不再 import `ippool`（测试 testutil 仍可构造 Pool）。
- **netstack → platform**：Windows route/NAT 子进程统一 `platform.Command`。
- **tunnel → tun**：`ServerHandler.TunDev` 类型为 `tun.Device`。
- **client-gui**：Fyne 桌面入口在 `cmd/client-gui/`，共用 `clientapp.Engine`。
