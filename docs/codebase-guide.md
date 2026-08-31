# HaoVPN 代码库导读

> **定位**：新人 / 新对话快速建立「项目是什么、代码在哪」的全局图景。  
> **详细 CODEMAP**（改 X 找哪）仍以 [architecture.md](architecture.md) 与 [internal/README.md](../internal/README.md) 为**单一权威来源**；本文不复制完整 FAQ 表格。

接手请先读 [记忆.md](../记忆.md) 阅读顺序。

---

## 1. 根目录

| 路径 | 职责 |
|------|------|
| [`VERSION`](../VERSION) | **唯一版本号**（仅开发者可改；AI 禁止修改） |
| [`README.md`](../README.md) | 产品简介、快速启动、文档入口 |
| [`记忆.md`](../记忆.md) | 接手阅读顺序与当前阶段 |
| [`cmd/`](../cmd/) | 三个二进制入口（见 [cmd/README.md](../cmd/README.md)） |
| [`internal/`](../internal/) | **全部核心逻辑**（见下文分层） |
| [`web/`](../web/) | WebUI 模板 + `static/*.js`（`go:embed`，见 [web/README.md](../web/README.md)） |
| [`scripts/`](../scripts/) | 构建、验收、field gate（见 [scripts/README.md](../scripts/README.md)） |
| [`docs/`](../docs/) | 活文档（本目录） |
| [`bin/`](../bin/) | `build-local.ps1` 输出 |
| [`dist/`](../dist/) | `build-release.ps1` 交叉编译输出 |

---

## 2. internal/ 分层总览

```
应用编排层    serverapp / clientapp / clientgui
              ↓ 组装 HTTP、隧道、TUN、探针、持久化
领域层        vpnaccount / sessionmgr / auth / probedefense
协议层        api / tunnel / transport / tun / netstack
持久化        persist / logstore / audit
叶子工具      netutil / fileutil / timeutil / paginate / safeutil / dialerr / autherr / logger / security / config / readmodel / platform / …
```

**依赖底线**（详见 architecture § 依赖规则）：

- 叶子包（`netutil`、`fileutil`、`safeutil`、`dialerr`…）**不得** import `api` / `serverapp` / `sessionmgr`
- `serverapp` **可以** import `api` 以启动 HTTP（`boot_api.go`）；公网绑定等**审计文案**走 `audit/public_bind.go`，勿把审计逻辑塞回 api
- `clientgui` **不得** import `tunnel`（展示 DTO 走 `clientapp.ManagedRouteView`）；**不得** import `tun` / `winnet` / `credentials`（预热/服务凭据经 `clientapp`）
- `clientapp` **不得** import `winnet`（经 `netstack` 门面）
- peer 写路径经 `vpnaccount.PeerPolicyApplier`（`peer_write.go`），api 只做 HTTP
- `tunnel` **不得** import `probedefense`；`autherr` **不得** import `transport`

---

## 3. 应用编排（改启动 / 客户端引擎）

### serverapp — 服务端启动

| 文件 | 做什么 |
|------|--------|
| `engine.go` / `engine_boot.go` | 总入口与启动顺序 |
| `boot_persist.go` | SQLite、自检、admin、**公网绑定审计**（`audit.LogPublicBindEnabled`） |
| `boot_session.go` | 探针 Guard 挂载 transport |
| `boot_tun.go` / `boot_tunnel.go` / `boot_api.go` | TUN、隧道 listener、管理 API |

### clientapp — CLI / 服务共用拨号引擎

| 文件 | 做什么 |
|------|--------|
| `engine_connect.go` / `engine_lifecycle.go` | 拨号、Soft 重连（`protectForReconnect`）、握手 |
| `hard_restart.go` | **HardRestart** / **WaitDNSReady**（手动全量重连契约） |
| `runtime*.go` | TUN、路由、策略增量 apply |
| `via_exit.go` | via/ICS：指纹跳过；空 local_lans 先 `HasICSResidue` 再清理（native 优先） |
| `runtime_routes.go` | 清路由：先 RestoreDNS 再删；`route_install`；零成功硬失败 / 部分失败 warn |
| `runtime_policy.go` | `policy_apply stage=tun|routes|dns|via_cleanup` 分段 elapsed |
| `route_view.go` | **`ManagedRouteView`** 展示 DTO（供 GUI 托盘） |
| `fatal_auth.go` / `dial_errors.go` | 封禁 / 鉴权致命错误 UX（直接 `autherr`/`dialerr`，无薄 Is* re-export） |

**重连双路径**：Soft = `transport.ReconnectClient` + 保 dataplane；Hard = `HardRestart`（全清后再拨）。GUI/CLI 禁止第三套编排。

### clientgui — 桌面托盘（Fyne）

| 文件 | 做什么 |
|------|--------|
| `run.go` | 启动；后台 `clientapp.WarmupTun`（与拨号重叠）；`applyFyneDoMigration` |
| `fyne_meta.go` | `SetMetadata(Migrations.fyneDo)`；构建另须 `-tags migrated_fynedo` |
| `tray.go` / `tray_routes.go` | 托盘菜单与路由展示；Disconnecting；手动重连入口 |
| `tray_tooltip.go` | 悬停文案：预算 63；IP→连接自→主机；Connecting+IP / 正在断开 |
| `reconnect_dns.go` | 调用 `clientapp.HardRestart`；UI 经 `fyne.Do` 挂载 |
| `engine_stop.go` | 异步 Stop；nil/非空 `onDone` 均 `fyne.Do` |
| `login.go` / `admin.go` | 登录与服务提权（登录页仍 FailFast，勿走 HardRestart） |

线程规则：后台只算账，UI 只 `fyne.Do`；主线程回调勿再 `DoAndWait`；重活禁止塞进 `Do`。

### Windows 子进程与 IP Helper 调用链

```
clientapp/via_exit.go
    → netstack.HasICSResidue / CleanupICSResidue / DisableICSPair（优先）/ DisableAllICS（门面 → winnet）
    → PreferVPNSourceWithICS / RemoveICSAddressesKeepVPN（winnet/address；经 netstack/ICS 路径）
tun assignIPv4
    → winnet.InterfaceHasIPv4 / SetInterfaceIPv4OnIndex（iphlp → netsh → PS）
netstack Setup/Teardown
    → forward_windows.go（IP 转发）
    → nat_windows.go（WinNAT）→ 失败则 ics_nat_windows.go（RememberICSPair）
    → route_ops_windows.go（iphlp → route.exe；method=）/ dns_windows.go（dns_apply；DNS method=netsh）
clientgui → beginEngineOp + 按钮 Disable；Stop 异步；WarmupTun / SaveServiceCredentials 仅经 clientapp
所有 PowerShell → RunPS*（一律一进程）；脚本模板 → `winnet/ps_snippets.go`
家庭版 → IsWindowsHomeSKU → 跳过 WinNAT → ICS
DNS → SetInterfaceDNSServers（iphlp→netsh）；路由 → AddOnLinkRouteIPHelper（LUID+MIB→route.exe）
WinNAT → 会话缓存 / sku_home 后直接 ICS（nat_windows.go）
开关 → client.yaml windows.use_ip_helper → NewEngine → netstack.ConfigureWindows
手动重连 → clientapp.HardRestart（Stop + WaitDNSReady + 新 Engine）
鉴权后 → Engine 早写 vpnIP；applyPolicy 后才 StateConnected
托盘悬停 → refreshTrayTooltip（预算 63；busy→正在断开）→ systray.SetTooltip
```

---

## 4. 管理 API 与 WebUI

### api — HTTP 薄层

| 文件 | 做什么 |
|------|--------|
| `handler_routes.go` | **路由表**（新增 API 先改这里） |
| `auth_handlers.go` | 登录/Session/CSRF；`requireAuth` **注入 Session 到 Context** |
| `session_context.go` | Context 存 Session；`sessionFromRequest` 优先读 Context |
| `httputil.go` | JSON 错误、分页、`requireMethod`、客户端 IP |
| `handler_security_*.go` | 探针事件/封禁/豁免（已按职责拆分） |
| `handler_peers_*.go` | 托管路由、互访、应用生效 |
| `users_*.go` | 账号 CRUD / VPN 策略 |

### web — 前端资源

| 路径 | 对应 API / 页 |
|------|----------------|
| `templates/*.html` | 页面骨架 |
| `static/security_probe.js` | `/security` 探针页（events/blocks/exempts） |
| `static/*.js` | 各管理页逻辑（CSP `script-src 'self'`） |

---

## 5. 安全与探针防御（横切）

```mermaid
flowchart LR
  Accept[transport Accept] --> Guard[probedefense.Guard]
  TLSFail[TLS握手失败] --> Guard
  Guard --> Blocks[(ip_blocks)]
  Guard --> Events[(security_events)]
  TLS[tls/frame 读错误] --> Guard
  Handshake[tunnel 握手拒绝] --> Guard
  API[api handler_security_*] --> Guard
  API --> Blocks
  Audit[audit.Logger] --> DB[(audit_logs)]
```

| 包 / 文件 | 职责 |
|-----------|------|
| `probedefense/guard.go` | Accept 门禁、RecordReject、ManualBan |
| `probedefense/classify_tls.go` | TLS/帧特征分类 |
| `probedefense/classify_handshake.go` | 握手拒绝 → signature |
| `probedefense/signatures.go` | 特征码常量（与 labels 同源） |
| `probedefense/auto_ban.go` | 窗口计数自动封禁 |
| `probedefense/exempt.go` | 封禁豁免合并 |
| `transport/probe_banner.go` | TLS 前 banner I/O（哨兵/常量在 `dialerr`） |
| `dialerr/` | `ErrIPBanned` / `ErrSourceDenied` / `ErrPlaintextBeforeTLS` / banner 常量 |
| `transport/server.go` | Accept → CheckAccept → `WriteRejectBanner` → Close |
| `clientapp/dial_errors.go` | `FormatDialError` 中文提示 |
| `clientapp/engine_connect.go` | `onDialError`：致命拨号错误置 Idle |
| `persist/security_store.go` | security_events / ip_blocks / exempt SQL |
| `probedefense/manual_ban.go` | ManualBanStore 手动封禁（须过豁免） |
| `autherr/classify.go` | 握手/拨号错误统一分类 + 线上 code |
| `tunnel/handshake_reject.go` | `OnHandshakeReject`（不 import probedefense） |
| `audit/public_bind.go` / `tun_listen.go` | 公网绑定 / TUN 管理口审计 |

**配置语义**（`enabled` / `record_events` / `auto_ban`）：见 [security-hardening.md §4.2](security-hardening.md#42-探针防御与安全事件)。

---

## 6. 协议与数据面

| 包 | 职责 | 关键文件 |
|----|------|----------|
| **transport** | TLS-TCP 帧、重连、Probe 钩子 | `server.go`, `transport.go`, `conn_loops.go`, `probe_banner.go`, `reconnect.go`（生产路径 GoSafe） |
| **tunnel** | 握手、IP 转发 | `server_handler.go`, `server_handshake_auth.go`, `server_handshake_session.go`, `handshake_reject.go` |
| **sessionmgr** | 在线会话、报文路由、横向隔离 | `register.go`, `route*.go` |
| **vpnaccount** | 开户、策略、peer 写+应用生效 | `peer_write.go`, `peer_apply.go`, `peer_policy.go` |
| **tun** / **netstack** | TUN 设备、路由/DNS/NAT/杀开关；Windows 门面 | 平台分文件；`winnet_facade.go` |

---

## 7. 叶子工具包（高复用）

| 包 | 何时用 | 关键文件 |
|----|--------|----------|
| **autherr** | 握手/拨号错误分类 + code 映射 | `classify.go` |
| **dialerr** | 拨号哨兵与 banner 常量（叶子；Error 中文） | `errors.go`, `classify.go` |
| **netutil** | CIDR/IP/监听/源白名单/TrimLower/子网 hint | `validate_ip.go`, `source_ip.go`, `strings.go`, `gateway.go` |
| **fileutil** | 原子写、目录、ACL、`RestrictToAdminsOnly` | `atomic.go`, `fs.go`, `perm_*.go` |
| **timeutil** | SQLite UTC、RFC3339、配置秒 → Duration | `sqlite.go`, `duration.go` |
| **paginate** | API limit/offset、bool 查询 | `parse.go`, `clamp.go` |
| **safeutil** | `GoSafe`、`RetryN`、`ExpBackoff`、Ticker | `goroutine.go`, `retry.go` |
| **winnet** | Windows 网卡 / IP Helper / 统一 PS / ICS / DNS | `options.go`, `iphlp_*`, `ps_*.go`, `ics_windows.go`, `dns_*`, `address_windows.go` |
| **logger** | 分级日志 + `RedactSensitive` | `redact.go` |
| **readmodel** | API 读模型 DTO（与 persist 解耦） | `peers.go` 等 |
| **config** | YAML 加载/默认值/校验 / TUN 名 | `defaults.go`, `server.go`, `client.go`, `tun_name.go` |

---

## 8. 改完去哪查

| 场景 | 文档 |
|------|------|
| 改 X 功能找哪个文件 | [architecture.md](architecture.md) FAQ |
| 包索引短表 | [internal/README.md](../internal/README.md) |
| 部署 / 配置项 | [deploy.md](deploy.md) |
| 生产安全清单 | [security-hardening.md](security-hardening.md) |
| 流量走向 | [traffic-routing.md](traffic-routing.md) |
| 进度与轮次 | [dev-log.md](dev-log.md) |

---

*最后更新：2026-08-31 · 架构解耦第 24 轮 / VERSION 0.1.3*
