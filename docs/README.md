# 文档索引

| 文档 | 说明 |
|------|------|
| [development-principles.md](development-principles.md) | 开发原则 |
| [architecture.md](architecture.md) | **CODEMAP**（包导航、HTTP 路由、依赖） |
| [internal/README.md](../internal/README.md) | **包索引**（改 X 功能来哪、分层速览） |
| [comment-style.md](comment-style.md) | **注释规范**（导出符号、字段、流程） |
| [cmd/README.md](../cmd/README.md) | 三个二进制入口 |
| [versioning.md](versioning.md) | 版本管理（`VERSION` 仅开发者可改） |
| [meta-plan.md](meta-plan.md) | v1.0 规划存档（进度以 dev-log 为准） |
| [deploy.md](deploy.md) | 部署、配置、验收 |
| [troubleshooting.md](troubleshooting.md) | 故障排障 |
| [dev-log.md](dev-log.md) | 开发日志 |
| [security-hardening.md](security-hardening.md) | 生产安全加固 |
| [archive/code-audit.md](archive/code-audit.md) | 历史代码审计（归档，不再维护） |

接手请先读根目录 [记忆.md](../记忆.md) 与 [README.md](../README.md)。

## 第九轮架构变更摘要（2026-08-28）

- **叶子工具**：`paginate.ParseLimitOffset`；`clientIP`→`HostFromAddr`；`persist.DefaultIPLeaseSec`；删 `FormatListenAddrs`。
- **HTTP 助手**：`writeOK`/`writePage`/`writeAttachment` 统一；CSRF 归 `auth_handlers`；`handler_listen.go`。
- **persist**：`query_{users,audit,events,monitor}.go` 按实体拆分。
- **maintenance**：`StartRetentionLoop` 经 `safeutil.GoSafe`。
- **文档**：CODEMAP 权威在 architecture.md；internal/README 仅 FAQ。详见 [architecture.md](architecture.md)。

## 第八轮架构变更摘要（2026-08-28）

- **叶子工具**：`paginate.ParseBoolQuery`；`timeutil` RFC3339；`config.DefaultServerCertPath`；删除 `persist/timefmt` 与 api 薄包装。
- **领域边界**：`vpnaccount.ApplyVPNPatch` / `SetAccountEnabled`；`auth` 改密/须改密 API。
- **monitor**：JOIN 去 N+1；用户名筛选下沉 SQL；事件 username JOIN。
- **readmodel**：`AuditLogView`、`ConnectionEventView`。
- **同包拆分**：clientapp `engine_*`、api `handler_*`/`users_*`、config `yaml_node`、serverapp `engine_shutdown`。
- **文档**：CODEMAP 权威在 architecture.md；internal/README 仅 FAQ。详见 [architecture.md](architecture.md)。

## 第七轮架构变更摘要（2026-08-28）

- **fileutil**：`WriteFileAtomic`、`ExecutableDir`；敏感写盘统一原子写。
- **timeutil**：SQLite UTC layout；persist/logstore 共用。
- **config**：`BuildClientExportYAML`、`DefaultRetentionDays`。
- **helper 统一**与 auth/persist/sessionmgr 同包拆分。

## 第六轮架构变更摘要（2026-08-27）

- **Wintun**：日志与孤儿清理；管理员改密 API；删 DB 迁移；`paginate.ParseIntDefault`。

## 第五轮架构变更摘要（2026-08-27）

- **paginate**：`ClampLimit`/`ClampOffset` 独立包；api、persist、logstore 共用。
- **persist 辅助**：`query_*.go`（原 `query_ext` 按实体拆分）、`scan.go`、`jsoncol.go`。
- **vpnaccount**：`delete.go`（`DeleteAccount` 踢线 + 释 IP）。
- **maintenance**：数据保留从 api 迁至独立包；serverapp 启动 ticker。
- **api 解耦**：生产代码不再 import `ippool`；删号走 `vpnaccount.Service`。
- **netstack → platform**：无窗口 route/netsh 子进程统一 `platform.Command`。
- **tunnel → tun**：`ServerHandler.TunDev` 为 `tun.Device`。
- **client-gui**：Fyne 结构文档化（`cmd/README.md`）；共用 `clientapp.Engine`。
- **注释扩展**：多包 doc.go 加厚；详见 [comment-style.md](comment-style.md)。
- **包索引**：新建 [internal/README.md](../internal/README.md)。

## 第四轮架构变更摘要（2026-08-27）

- **netutil**：`addr.go`（HostFromAddr、ParseHostIP、NormalizeIPv4）；CIDR 拆分/掩码从 netstack 迁入。
- **api 拆分**：`users.go`、`auth_handlers.go`、`httputil.go`。
- **clientapp**：`runtime.go`（TUN/路由/DNS）。
- **transport**：`frame.go`、`reconnect.go`。
- **readmodel**：Web/API DTO 与 persist 解耦。
- **fileutil**：`EnsureParentDir`。
- **sessionmgr**：`PacketConn` 窄接口。
