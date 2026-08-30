# HaoVPN 手机客户端 · 实施计划（权威）

> **文档性质**：实施蓝图。按本文即可开工；**当前仓库尚未实现手机客户端代码**。  
> **状态**：仅文档落盘（2026-08-30）。开工后每完成一步在 [dev-log.md](dev-log.md) 记验收，并回写本文「进度勾选」。  
> **关联**：桌面 CODEMAP 仍以 [architecture.md](architecture.md) 为准；本文是移动线补充，不替代桌面文档。  
> **原则**：遵守 [development-principles.md](development-principles.md) — 安全 > 简单 > 快；一步一验；禁止「下次再说」。

---

## 0. 一句话结论

**Flutter 只做 UI；一份 Go `vpncore` 做协议引擎（跑在系统 VPN 进程里）；Android `VpnService` / iOS `NEPacketTunnelProvider` 做系统隧道壳。**  
对接现有 `haovpn-server`，**不重写服务端协议**。  
不是「Go 远程 HTTP 后端 + Flutter 分别调 android/ios 两套后端」。

---

## 1. 交付承诺与硬规则

### 1.1 交付范围（一次做完）

| 必须交付 | 说明 |
|----------|------|
| 公共 `vpncore` | 可 gomobile 导出；复用 TLS-TCP / 握手 / AEAD |
| Android 壳 | `VpnService` + `protect(fd)` + Builder 路由/DNS |
| iOS 壳 | `NEPacketTunnelProvider` + App Group Keychain |
| Flutter UI | 登录 / 连接 / 状态 / 错误 / 配置导入 |
| Go 侧硬伤修复 | protect、PacketIO、策略回调、TLS pin、凭据、日志等（见 §6） |
| 全局文档收口 | CODEMAP、FAQ、安全、部署、排障、各包说明 |

### 1.2 硬规则（违反即不合格）

1. **进程归属**：`vpncore` **只**运行在 Android VPN Service / iOS Network Extension 内；Flutter UI 进程不得持有隧道数据面。
2. **禁止**在移动路径调用 `tun.Open` / `netstack.AddClientRoute` / 桌面 via/ICS。
3. **禁止**把 `internal/...` 当作 gomobile 导出包；必须有公共 `vpncore/`（或模块根下非 internal 包）。
4. **禁止** Flutter `SharedPreferences` / 明文文件存密码或隧道私钥。
5. **禁止** `vpncore` 内 `os.Exit` / `logger` Fatal 杀进程（会杀扩展）。
6. **禁止** release 构建启用 `insecure_skip_verify` 或 Trace 级明文包内容。
7. **禁止**「先能跑、安全以后再说」；临时方案必须同交付内偿还并记入 `dev-log`。
8. **桌面不回归**：移动改动用 build tag / 适配器；每步后 `go test ./...`（至少受影响包）须绿。
9. **AI 不得修改根目录 `VERSION`**；手机版本规则另见 §11，由开发者确认后写入 [versioning.md](versioning.md)。
10. **一步一验**：不过验收门禁不得进入下一步。

### 1.3 环境前提（硬依赖）

| 环境 | 用途 |
|------|------|
| Windows + pwsh + Go（与现网一致） | 改 `vpncore` / `transport` 等；桌面回归 |
| Android SDK + NDK | 编 AAR、装真机/模拟器 |
| macOS + Xcode + Apple Developer（含 Network Extension 能力） | iOS 扩展签名与真机联调 |
| Flutter SDK | UI 工程 |
| 可连的 `haovpn-server` + 测试账号 | 联调 |

无 Mac 时可先完成 Go + Android + Flutter Android；iOS 代码仍须按本文写全，真机验收在有 Mac 的机器上完成并记 `dev-log`。

---

## 2. 原理与设计决策

### 2.1 为什么必须系统 VPN API

| 能力 | 桌面现状 | 手机 OS 要求 |
|------|----------|--------------|
| 收发 IP 包 | Wintun / utun / `/dev/net/tun` | 只能经 VpnService TUN fd / NE `packetFlow` |
| 路由 / DNS | `netstack` 改系统表 | 只能经 Builder / `NEPacketTunnelNetworkSettings` |
| 后台保活 | 服务 / 托盘 | 系统 VPN 会话；普通 App 会被杀 |
| 商店上架 | 自研二进制 | 须 VPN 权限 / Network Extension entitlement |

因此：**跨平台的是 UI + 协议核心；隧道壳两端原生，不可省。**

### 2.2 为什么核心用 Go、不用 Dart 重写

- 现网协议：TLS-TCP 帧 + JSON 握手 + ChaCha20-Poly1305（见 `internal/transport`、`internal/tunnel`、`internal/crypto`）。
- Dart/KMP 重写 = 双实现，必然与服务端漂移。
- 桌面已将 UI（Fyne）与引擎（`clientapp`）分开；移动只换 UI 与数据面适配器。

### 2.3 为什么不是「Flutter 调两个 Go 后端」

- **一份** Go 源码 → gomobile 产出 **AAR + XCFramework**。
- `mobile/flutter/android`、`ios` 是宿主与系统 VPN 壳，不是两套业务后端。
- Flutter → MethodChannel → 原生开 VPN → 原生进程内调 **同一份** `vpncore`。

### 2.4 与 HaoVPN 服务端的关系

- 手机客户端 = 另一种 **客户端**；服务端协议 **零变更** 即可接入（同一 TLS 端口与握手）。
- 桌面 `cmd/client`、`cmd/client-gui` **保留**；共享逻辑收敛到 `vpncore` + 桌面适配器，避免复制握手。

### 2.5 产品边界（明确拒绝，不是后置）

| 不做 | 原因 |
|------|------|
| 手机当 via ExitLAN / ICS 枢纽 | OS 与工控出口模型不适配；移动构建对非空 `local_lans` **直接报错** |
| Fyne / SCM / 桌面服务接管 | 桌面专属 |
| 本交付重写服务端协议 | 无必要 |
| 假装 IPv6 双栈 | 与现网一致：**仅 IPv4**；遇 v6 拒绝或忽略并打日志 |

---

## 3. 代码架构

### 3.1 分层图

```text
┌─────────────────────────────────────────────────────────┐
│  Flutter UI（Dart）                                      │
│  登录 / 开关 / 状态 / 错误 / 配置导入                       │
└──────────────────────────┬──────────────────────────────┘
                           │ MethodChannel / 事件流
┌──────────────────────────▼──────────────────────────────┐
│  原生壳（Kotlin / Swift）                                 │
│  Android: VpnService + Builder + protect(fd)             │
│  iOS: NEPacketTunnelProvider + NetworkSettings           │
│  凭据: Keystore / Keychain（App Group）                   │
└──────────────────────────┬──────────────────────────────┘
                           │ FFI / gobind（同进程）
┌──────────────────────────▼──────────────────────────────┐
│  vpncore（Go，公共可导出）                                 │
│  Configure / Start / Stop / State / PacketIO / OnPolicy   │
│  内部复用: transport · tunnel(client) · crypto · security │
└──────────────────────────┬──────────────────────────────┘
                           │ TLS-TCP + 握手（现有协议）
┌──────────────────────────▼──────────────────────────────┐
│  haovpn-server（现有，不改协议）                            │
└─────────────────────────────────────────────────────────┘
```

### 3.2 目标目录树（实施时创建）

```text
go-vpn/
├── vpncore/                      # 【新建】公共导出 API（禁止放 internal）
│   ├── doc.go
│   ├── engine.go                 # 生命周期与状态机
│   ├── config.go                 # 内存配置 DTO
│   ├── packetio.go               # PacketIO 接口
│   ├── policy.go                 # 策略回调 DTO
│   ├── allowlist_test.go         # 依赖白名单测试（可选 CI）
│   └── README.md
├── mobile/
│   ├── go/                       # gomobile bind 入口（薄包装）
│   │   └── export.go
│   ├── flutter/                  # Flutter 工程
│   │   ├── lib/                  # Dart UI
│   │   ├── android/              # 宿主 + VpnService 插件
│   │   └── ios/                  # 宿主 + Network Extension
│   └── README.md                 # 移动线构建说明
├── internal/
│   ├── transport/                # 【改】可注入 Dialer / ProtectSocket
│   ├── security/                 # 【改】内存 CA + SPKI pin
│   ├── clientapp/                # 【改】拆 SessionController；桌面 dataplane 适配
│   ├── crypto/                   # 【改】瘦身 + 缓冲池
│   ├── credentials/              # 【改】Store 接口 + 平台实现
│   ├── logger/                   # 【改】可注入 Writer；vpncore 路径禁 Exit
│   ├── tun/ · netstack/          # 桌面继续用；移动构建不链路由变更
│   └── ...
├── cmd/client · cmd/client-gui   # 桌面入口，保持
└── docs/
    ├── mobile-client-plan.md     # 本文
    ├── mobile-audit.md           # 开工 Step A 产出
    ├── mobile.md                 # 架构速查（开工后）
    ├── mobile-faq.md
    ├── mobile-store-compliance.md
    └── deploy-mobile.md
```

### 3.3 包职责与「改 X 找哪」（实施后须同步进 architecture）

| 需求 | 去哪 |
|------|------|
| 拨号 / 握手 / 重连 / 状态 | `vpncore/` |
| TLS 帧、心跳、队列 | `internal/transport` |
| 握手 JSON、策略字段 | `internal/tunnel`（仅 client 侧进移动） |
| 载荷 AEAD | `internal/crypto` |
| TLS CA / pin | `internal/security` |
| 桌面 TUN + 路由 | `internal/tun`、`internal/netstack`、`clientapp` 桌面适配 |
| Android 开隧道 / protect | `mobile/flutter/android/...` VpnService |
| iOS 开隧道 / settings | `mobile/flutter/ios/...` PacketTunnel |
| UI 页面 | `mobile/flutter/lib/` |
| 凭据安全存储 | 原生 Keystore/Keychain + `credentials.Store` |
| 服务端账号/策略权威 | **不变**：现有 server + WebUI |

### 3.4 `vpncore` 建议导出面（约 8～12 个）

| API | 职责 |
|-----|------|
| `Configure(cfg JSON/DTO)` | 服务器地址、CA PEM、pin、用户名等（无明文落盘义务） |
| `SetCredentials(user, pass)` | 会话前设置；由原生从 Keystore 注入 |
| `SetProtect(fn)` / Dialer 钩子 | Android `protect`；桌面 nil |
| `AttachPacketIO(io)` | 原生注入读写 IP 包 |
| `Start()` / `Stop()` | 生命周期 |
| `State()` / `VPNIP()` / `LastError()` | UI 轮询或事件 |
| `OnPolicy(callback)` | 下发 VPNIP、AllowedIPs、DNS、MTU → 原生设隧道 |
| `OnLog(callback)` | 注入日志，禁 Fatal Exit |

### 3.5 移动状态机（必须）

```text
Idle
  → PreparingNative      # 原生已建立 VPN 会话 / 拿到 fd 或 packetFlow
  → Dialing              # protect 后 TLS Dial
  → Authed               # 握手成功，已有 policy
  → DataplaneReady       # PacketIO 已附着，原生已 apply settings
  → Connected            # 包泵运行
  → Reconnecting         # 可保留或重建数据面（产品定一种，文档写死）
  → Failed / Idle        # 致命鉴权 → 停自动重连
```

缺 `PacketIO` 或未 protect 时：**Fail closed**，不得假装 Connected。

### 3.6 依赖允许清单（vpncore-lite）

**允许**：`transport`、`tunnel`（client）、`crypto`、`security`（client TLS）、`netutil`、`config` DTO 子集、`logger`（可注入）、`auth` 哨兵错误、`safeutil`、`brand`。

**禁止链入移动二进制**：`clientgui`/Fyne、`persist`/SQLite、`api`、`serverapp`、`sessionmgr`、`wintun`、`winnet`、`autostart`、`singleinstance`、via/ICS 路径。

CI：`go list -deps` + allowlist 测试；`GOOS=android` / `ios` 编译门禁。

---

## 4. 关键流程

### 4.1 一次连接（成功路径）

```text
用户点「连接」
  → Flutter MethodChannel「startVpn」
  → 原生：请求 VPN 权限 → 启动 VpnService / startTunnel
  → 原生：从 Keystore/Keychain 取凭据 → vpncore.Configure + SetCredentials
  → 原生：establish() / 准备 packetFlow → AttachPacketIO
  → 原生：注入 ProtectSocket
  → vpncore.Start → TLS Dial（已 protect）→ ClientHandshake
  → OnPolicy(vpn_ip, allowed_ips, dns, mtu)
  → 原生：Builder / setTunnelNetworkSettings 应用策略
  → 状态 DataplaneReady → Connected
  → 循环：TUN 读包 → Go 加密 Send；Go 收包解密 → TUN 写包
```

### 4.2 断线重连

- 指数退避与桌面一致（现 `transport` / `clientapp` 行为，收敛到 vpncore 后保持语义）。
- 致命鉴权（密码错、禁用、账号已在线等）：**停止自动重连**，错误上抛 UI（对齐 `clientapp/fatal_auth.go`）。
- Android：重连前确认 socket 再次 `protect`。
- iOS：扩展进程内重连；凭据从 App Group Keychain 读，不依赖 Flutter 前台。

### 4.3 路由环（必须用日志验证已消除）

```text
错误：未 protect → 隧道 TCP 被路由进 VPN → 黑洞 / 重连风暴
正确：protect(fd) 或 NE 排除路径 → 隧道走物理网卡；仅策略 CIDR 进隧道
验收埋点：protect_ok=true；连通后仍能访问服务器公网地址
```

---

## 5. 实施步骤（严格串行）

每步结束：**单元测试和/或日志证据** + 更新 `dev-log` + 勾选下表。不过门禁不进入下一步。

| 步骤 | 内容 | 验收门禁 | 状态 |
|:----:|------|----------|:----:|
| **A** | 产出 `docs/mobile-audit.md`，§6 每条含位置/修复/验收 | 与本文编号一致、无遗漏 | ⬜ |
| **B** | P0：Dial protect、PacketIO、策略回调、公共 `vpncore`、TLS 内存 CA+pin | 单测绿；桌面行为不变；移动路径无 `tun.Open` | ⬜ |
| **C** | P1：凭据接口、host_id、logger 注入、crypto 瘦身、拒 via、MTU clamp、IPv6 策略 | 单测 + `GOOS=android/ios` 编译 vpncore | ⬜ |
| **D** | 桌面全量回归 `go test ./...` | 全绿 | ⬜ |
| **E** | Android VpnService + 包泵 + 联调现网服务端 | 无环路日志；重连；致命鉴权停连 | ⬜ |
| **F** | iOS NE + App Group Keychain + 链接裁剪 | 同上 + 扩展内存/依赖检查 | ⬜ |
| **G** | Flutter UI 全页 + 安全存储对接 | 双端 UI→连接→断开走通 | ⬜ |
| **H** | 全局文档（§10）+ 最终门禁 | 文档与代码一致；`dev-log` 记交付日 | ⬜ |

### Step A — 审计落盘

- 将 §6 全文写入 `docs/mobile-audit.md`（可增补实测细节）。
- 每条字段：`ID`、`Severity`、`Location`、`Defect`、`Fix`、`Acceptance`。

### Step B — P0 Go 内核

| 改动 | 关键位置（现行） | 做法 | 验收 |
|------|------------------|------|------|
| Protect Dial | `internal/transport` | `Config` 增加 `Dialer` 或 `ProtectSocket`；`net.Dialer{Control:…}` | 单测 Control 被调用；桌面 nil 行为不变 |
| PacketIO | `clientapp/runtime_policy.go` 等 | 接口 Read/Write/Close/MTU；移动由原生注入 | 移动构建无 `tun.Open` |
| 策略回调 | `runtime_routes.go`、`netstack` | 纯解析 policy → 回调；桌面适配器内再调 netstack | 策略 DTO 单测 |
| 公共 vpncore | 新建 `vpncore/` | 门面 + 状态机；gomobile 可 bind | `go list` allowlist |
| TLS | `internal/security` | `CAPem`、`PinnedSPKI`；release 禁 insecure | pin 失败单测 |

**建议日志埋点（脱敏）**：`protect_ok`、`policy_emit`、`packetio_attach`、`tls_pin_fail`、`state_transition`。

### Step C — P1 加固

- `credentials.Store`：Save/Load/Delete；Win DPAPI / Android Keystore / iOS Keychain。
- `host_id`：安装期随机写入安全存储；**禁止** `os.Hostname()`。
- logger：注入 `io.Writer`/回调；vpncore **禁止** `os.Exit`。
- crypto：去掉仅 typedef 用的 `wireguard/device` 重依赖；AEAD 缓冲池。
- 移动：非空 `local_lans` → 明确错误；蜂窝 MTU clamp（如 ≤1280）；仅 IPv4。

### Step D — 桌面回归

```powershell
go test ./...
```

### Step E — Android

- `VpnService` + 前台通知 + `Builder.addAddress/addRoute/addDnsServer`。
- TLS 套接字 `protect`。
- 与 vpncore 双向包泵。
- 联调清单见 §8.2。

### Step F — iOS

- `NEPacketTunnelProvider`；`setTunnelNetworkSettings` 应用 OnPolicy。
- App Group Keychain 供 UI 与扩展共享凭据。
- 扩展 target **只**链 vpncore-lite。
- 联调清单同 Android 语义。

### Step G — Flutter

- 页面：配置导入、登录、连接开关、状态（VPN IP、AllowedIPs 只读）、错误文案、简单日志。
- 配置字段兼容服务端导出客户端包（见 `internal/config` 导出逻辑）。
- 不把密码/私钥写入 Dart 持久化。

### Step H — 文档与最终门禁

见 §10、§8.3。

---

## 6. 缺陷清单（实施必须全部关闭）

开工 Step A 时复制到 `docs/mobile-audit.md` 并逐项勾选。

### 6.1 P0 阻断

| ID | 缺陷 | 位置（现行） | 修复要点 | 验收 |
|----|------|--------------|----------|------|
| P0-1 | 无 socket protect，VPN 后隧道黑洞 | `internal/transport` Dial | 可注入 Protect/Dialer | 单测 + 真机无环路 |
| P0-2 | 引擎自建 TUN，移动 OS 契约错误 | `clientapp` `tun.Open` | PacketIO 注入；移动禁 Open | 移动构建无 tun.Open |
| P0-3 | Go 改系统路由/DNS | `runtime_routes`、`netstack` | 策略回调原生 | 移动无 AddClientRoute |
| P0-4 | `internal` 无法 gomobile 导出 | 架构缺失 | 公共 `vpncore/` | bind 成功 |
| P0-5 | Go 跑在错误进程 | 设计 | 仅 Service/NE 内跑引擎 | 架构文档 + 实现一致 |
| P0-6 | TLS 仅文件 CA、无 pin | `security` TLS client | 内存 CA + SPKI pin | 单测 |
| P0-7 | 密码/私钥落盘不适配商店 | credentials / YAML | Keystore/Keychain | 无明文 prefs |

### 6.2 P1 高

| ID | 缺陷 | 修复要点 |
|----|------|----------|
| P1-8 | 无移动状态机 / Attach 门闸 | §3.5 |
| P1-9 | via / local_lans 不适配手机 | 移动拒绝 |
| P1-10 | 无跨平台凭据抽象 | `credentials.Store` |
| P1-11 | 依赖 exe 旁 YAML | 内存 Configure |
| P1-12 | Fatal/Exit 杀扩展 | 可注入日志、禁 Exit |
| P1-13 | crypto 依赖过重 / 每包分配 | 瘦身 + pool |
| P1-14 | DNS 仍走桌面路径 | 仅原生 settings |
| P1-15 | 多端同账号 / reject_second | 产品文案 + UI 提示 |
| P1-16 | Hostname 作 host_id（PII/不稳） | 随机稳定 ID |
| P1-17 | 易链入 Fyne/SQLite/server | allowlist CI |
| P1-18 | 缺上架合规说明 | `mobile-store-compliance.md` |

### 6.3 P2 中（同交付做完）

| ID | 缺陷 | 修复要点 |
|----|------|----------|
| P2-19 | Engine 过重难测 | SessionController vs DesktopDataplane |
| P2-20 | Reconnect 缺 context/自定义 dial | 改造 ReconnectClient |
| P2-21 | tunnel 易拖进 server | client/server 隔离或 build tag |
| P2-22 | 杀开关语义不同 | 桌面 WFP；移动系统 Always-on 文档化 |
| P2-23 | singleinstance/autostart | 不进 vpncore |
| P2-24 | 日志脱敏审计 | 扫客户端路径 |
| P2-25 | 威胁模型未写移动 | security-hardening 增补 |
| P2-26 | 蜂窝 MTU | clamp + 日志 |
| P2-27 | IPv6 含糊 | 明确仅 IPv4 |
| P2-28 | 文档 CODEMAP/FAQ 缺口 | §10 |

---

## 7. 安全与权限清单

### 7.1 原则

- 密码、私钥、token、PEM、Authorization **永不入日志**。
- release：无 insecure TLS、无 Trace 包内容、无明文凭据文件。
- 扩展/服务：最小依赖；无 GUI；无服务端 SQLite 栈。

### 7.2 Android

- 权限：`BIND_VPN_SERVICE`、前台服务类型（按目标 SDK）、网络。
- 必须：`VpnService.protect(fd)`。
- 可选：Always-on VPN / `setBlocking`（慎用；依赖 protect 正确）。
- 密钥：Android Keystore；进程内仅会话内存持有。

### 7.3 iOS

- Network Extension entitlement；App Group；Keychain Sharing。
- 主 App：配置 VPN、启停、UI。
- 扩展：跑 vpncore + packetFlow；内存预算紧，严格裁剪。
- On Demand 规则若启用，须在文档写清触发条件。

### 7.4 TLS

- 默认校验服务器证书；支持内置/下发 CA PEM。
- 推荐 SPKI/证书 pin（与导出配置一并分发）。
- release 构建编译期禁止 skip verify。

### 7.5 多端会话

- 若服务端 `session_policy: reject_second`：手机与电脑互踢；UI 须明确提示。
- 行为以服务端为准；客户端只做可读错误映射（对齐现有 fatal auth）。

---

## 8. 测试与验收

### 8.1 自动化（每步相关包）

```powershell
# 桌面与共享
go test ./internal/transport/... ./internal/security/... ./internal/crypto/... ./vpncore/...
go test ./...

# 移动编译门禁（示例；以实际模块路径为准）
$env:CGO_ENABLED=1  # gomobile 需要时
go build -o NUL ./vpncore/   # 可再加 GOOS=android 交叉检查
```

必测场景举例：

- Protect/Control 钩子被调用。
- pin 错误证书 → 失败。
- 策略 DTO 解析（allowed_ips、dns、mtu）。
- 移动构建拒绝 local_lans。
- 致命鉴权错误 → 不再重连。
- 日志不含 password/PEM 子串（可用现有 redact 测试风格）。

### 8.2 手工联调清单（Android / iOS 各一份）

1. 导入配置 / 登录。  
2. 首次授权 VPN → 连接成功 → 显示 VPN IP。  
3. 访问 AllowedIPs 内目标可达；非允许目标按策略不可达。  
4. 断网 / 杀服务端 → 自动重连；恢复后通。  
5. 错误密码 → 明确错误且不空转重连。  
6. 同账号桌面已在线（若 reject_second）→ 符合服务端策略的提示。  
7. 日志文件/logcat：**无**密码与私钥。  
8. 确认隧道服务器地址在 VPN 开启后仍可达（证明无环路）。

### 8.3 最终门禁

- [ ] `go test ./...` 绿  
- [ ] vpncore `GOOS=android` / `ios` 编译通过  
- [ ] Android 联调清单全过  
- [ ] iOS 联调清单全过（或记明环境阻塞与补测计划——仅环境，不欠代码）  
- [ ] §10 文档全部存在且与代码一致  
- [ ] `dev-log` 有交付记录；`记忆.md` 当前阶段已更新  

---

## 9. 功能范围（客户端定义）

### 9.1 必须有

- 配置导入（兼容服务端导出字段）
- 账号登录、连接、断开
- 状态：连接态、VPN IP、AllowedIPs / 路由只读、最后错误
- 自动重连 + 致命鉴权停连
- 握手策略 → 系统 VPN 路由与 DNS
- protect、TLS 校验/pin、安全存储、日志脱敏

### 9.2 明确不做

- via ExitLAN / ICS / 手机当枢纽
- 桌面托盘、SCM、Fyne
- 服务端协议重写
- IPv6 双栈（本文交付仅 IPv4）

---

## 10. 开工后须更新的文档（同交付，不漏）

| 文档 | 动作 |
|------|------|
| [architecture.md](architecture.md) | 增加 Mobile CODEMAP 分层与依赖规则 |
| [../记忆.md](../记忆.md) | 阅读顺序 / 当前阶段 / 平台范围含手机 |
| [../internal/README.md](../internal/README.md) | FAQ 增加移动行 |
| `vpncore/README.md`、关键 `internal/*/README.md` | 包职责与文件索引 |
| [security-hardening.md](security-hardening.md) | pin、Keystore、NE、日志 |
| `mobile.md` / `mobile-faq.md` / `mobile-store-compliance.md` / `deploy-mobile.md` | 新建 |
| [troubleshooting.md](troubleshooting.md) | protect 缺失=环路；NE OOM 等 |
| [dev-log.md](dev-log.md) | 每步验收 |
| [versioning.md](versioning.md) | 手机产物版本规则（开发者确认后） |
| [README.md](README.md)（本目录索引） | 已链到本文；实施后链到 mobile.md |

FAQ 必须能答：「连接 / protect / 策略 / 凭据 / Flutter 通道 → 哪个目录哪个文件」。

---

## 11. 版本与发版

- 根目录 `VERSION`：**仅开发者可改**；AI 禁止改。
- 手机 App 可用：同号不同产物，或独立 `mobile` 版本字段——**开工前由开发者定一条规则写入 versioning.md**。
- 桌面 `build-release.ps1` 与 mobile 构建脚本 **分离**；勿把 Flutter 塞进现有桌面发版默认路径 unless 明确扩展。

---

## 12. 与当前桌面进度的关系

- 现网 v1.0 桌面 + step11 field 门禁见 [记忆.md](../记忆.md)。
- 手机线可并行开发，但 **合并前** 桌面与移动测试都要绿；冲突时优先保现场桌面稳定。
- [meta-plan.md](meta-plan.md) 写明「移动端仍不做」为 **v1.0 历史范围**；本文是后续产品线蓝图，不篡改 meta-plan 假装 v1.0 已含手机。

---

## 13. 开工检查单（人动手前）

1. 通读本文 §1～§5。  
2. 准备好 §1.3 环境（至少 Go + Android；iOS 有则更好）。  
3. 从 **Step A** 开始：写 `mobile-audit.md` → 再改代码。  
4. 每步：测试/日志 → `dev-log` → 勾选 §5 表格。  
5. 全部勾选且 §8.3 过 → 更新 `记忆.md` 当前阶段。

---

## 14. 术语

| 术语 | 含义 |
|------|------|
| vpncore | 公共 Go 客户端引擎库（手机 + 将来可被桌面收敛） |
| PacketIO | 由 OS VPN 提供的 IP 包读写抽象（替代移动上的 tun.Open） |
| protect | Android 将套接字排除出 VPN 路由，避免环路 |
| NE | iOS Network Extension（Packet Tunnel） |
| 策略回调 | 握手得到的 VPNIP/AllowedIPs/DNS/MTU 交给原生设置隧道 |
| vpncore-lite | 裁剪依赖后的移动链接集合 |

---

*文档版本：2026-08-30 · 仅计划落盘，代码未开工*  
*维护：实施过程中以 `dev-log` 为进度；本文步骤表同步勾选；行为变更以代码为准并回写本文*
