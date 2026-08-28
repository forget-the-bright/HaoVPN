# internal/ 包索引

> **改代码去哪**：按功能查下表。完整 CODEMAP、分层与依赖规则见 [docs/architecture.md](../docs/architecture.md)（权威单一来源）。

---

## 改 X 功能来哪（FAQ）

| 想改什么 | 去这里 |
|----------|--------|
| 管理 API 路由 | `api/handler_routes.go` |
| 健康 / 审计 / Dashboard / 备份 / 日志 | `api/handler_ops.go`；Dashboard 字段 `health/dashboard.go` |
| 管理 API 多地址监听 | `api/handler_listen.go`（`StartAllListeners`） |
| API 标准错误 / 成功 / 分页信封 / since | `api/httputil.go` → `writeOK`/`writePage`/`timeutil.ParseSinceRFC3339` |
| 用户 CRUD / 删除账号 | `api/users_crud.go` → `vpnaccount.DeleteAccount` |
| VPN 策略 PATCH | `api/users_vpn.go` → `vpnaccount.ApplyVPNPatch` |
| 启禁账号 | `api/users_crud.go` → `vpnaccount.SetAccountEnabled` |
| 管理员重置用户密码 | `api/users_vpn.go` → `auth.ResetPasswordByAdmin` |
| 登录 / Session / CSRF / 自改密 | `api/auth_handlers.go`；`auth/login.go`、`password_ops.go` |
| 隧道密码校验 | `auth/tunnel_login.go`（`VerifyTunnelLogin`） |
| 分页 limit/offset / `?online=` | `paginate/parse.go`（`ParseLimitOffset`、`ParseBoolQuery`）、`clamp.go` |
| 默认 IP 租约秒 | `persist/constants.go`（`DefaultIPLeaseSec`） |
| 监控页 JSON | `readmodel/monitor.go`、`api/monitor_handler.go`；JOIN 在 `persist/query_monitor.go` |
| 用户/审计/事件列表 SQL | `persist/query_users.go`、`query_audit.go`、`query_events.go` |
| 审计 API 视图 | `readmodel/audit.go`；`persist.AuditEntriesToViews` |
| 数据保留 / 定时清理 | `maintenance/retention.go`；默认天 `config/retention.go` |
| Web 开户 / IP 分配 | `vpnaccount/provision.go`、`service.go` |
| 握手 / 策略下发 | `tunnel/handshake.go`、`server_handler.go` |
| 客户端拨号 / 重连 | `clientapp/engine_lifecycle.go`、`engine_connect.go` |
| TUN / 路由 / DNS 运行时 | `clientapp/runtime.go` |
| 桌面 GUI（Fyne） | `clientgui/`（`cmd/client-gui` 仅入口） |
| 导出客户端 YAML / ZIP | `config/client_export.go`；`api/export_zip.go`；`writeAttachment` |
| 默认证书路径 | `config/paths.go` |
| GUI 写回 yaml | `config/client_yaml_patch.go`；Node 原语 `yaml_node.go` |
| 敏感文件原子写 | `fileutil/atomic.go` |
| SQLite / RFC3339 时间 | `timeutil/sqlite.go`、`rfc3339.go` |
| 路由 / DNS / 杀开关 | `netstack/` → `platform.Command` |
| TUN / Wintun DLL | `tun/`、`tun/wintundll/` |
| Windows 网卡 / netsh | `winnet/` |
| CIDR / 地址 / 网关 | `netutil/`（`HostFromAddr`） |
| SQLite CRUD | `persist/store.go`、`users.go`、`session_store.go`、`query_*.go` |
| TLS / 证书 / 私钥加密 | `security/` |
| YAML 默认值 | `config/client.go`、`server.go` |
| Windows 服务 | `clientapp/service_windows.go`（CLI `--service`） |
| UAC 提权 | `platform/` |
| 单实例（TCP 协调） | `singleinstance/lock.go`、`coord.go` |

---

## 分层速览

```
clientapp / clientgui / serverapp
    ├── api ──► vpnaccount / auth
    ├── tunnel ──► tun
    ├── netstack ──► platform
    ├── maintenance
    └── persist + sessionmgr
netutil / winnet / fileutil / timeutil / paginate / readmodel / security / config
```

完整包一览表见 [architecture.md § CODEMAP](../docs/architecture.md#internal-包-codemap)。

---

## 第十一轮架构要点（2026-08-28）

- **vpnaccount**：`releaseDynamicIP`；`PlanVPNPatch` 在 `patch.go`；`ErrAccountNotFound` 统一。
- **api**：`users_crud` 复用 `onlineUserSet()`；导出/重置密码 404 语义对齐。
- **审计**：logs API redaction、Cookie HttpOnly/SameSite、public bind WARN、form 400 补测。
- **授权**：[docs/licensing.md](../docs/licensing.md) 发版前检查清单（法律层，无运行时校验）。

## 第十轮架构要点（2026-08-28）

- **persist**：`query_page.go`（`queryPageTotal`）；删 `ListAuditLogs`。
- **serverapp**：`engine_boot.go` 分阶段启动；`engine.go` 仅串联。
- **api**：`writeMethodNotAllowed`、`dataplaneSnapshot`、`buildMonitorItems`；`s.clientIP` + `trusted_proxy_cidrs`。
- **安全**：`logger.RedactSensitive`；密码强度；Secure Cookie；禁用账号握手测试。
- **授权**：[LICENSE](../LICENSE)、[docs/licensing.md](../docs/licensing.md)。

## 第九轮架构要点（2026-08-28）

- **叶子工具**：`paginate.ParseLimitOffset`；`clientIP`→`netutil.HostFromAddr`；删未用 `FormatListenAddrs`；`persist.DefaultIPLeaseSec`。
- **HTTP 助手**：全面 `writeOK`/`writePage`；`writeAttachment` 导出；去 logs 双重 Clamp；CSRF 归 `auth_handlers`。
- **安全**：`maintenance.StartRetentionLoop` 经 `safeutil.GoSafe`。
- **同包拆分**：`handler_listen.go`；`persist/query_{users,audit,events,monitor}.go`。
- **文档**：CODEMAP 权威在 architecture.md；本文件仅 FAQ。
