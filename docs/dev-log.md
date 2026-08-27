# 项目开发日志

> 记录开发历程、重要决策、踩坑与待办。**每完成一个 step 或遇到值得记录的问题，追加一条。**
>
> 格式：`## YYYY-MM-DD · 标题` + 正文

---

## 2026-08-27 · 管理控制台性能与体验（一次性交付）

### 完成项
- **日志**：`readLogTail` 从文件尾读取；维护页支持 live/滚动文件/历史库；`logs.db` 异步入库 + 分页/级别/关键字检索。
- **API**：账号/审计/连接事件分页与筛选；`monitor/accounts` JOIN 去 N+1；`events?limit` 上限 200。
- **保留**：审计/连接事件/历史日志按天清理（默认 90 天，可配置 `-1` 关历史库）。
- **WebUI**：账号搜索、审计筛选、连接事件表、维护入口全站导航；总览改 `/api/v1/dashboard`。
- `go test ./...` ✅；`build-local` ✅。

---

### 完成项
- **品牌**：MyVPN/go-vpn → **HaoVPN/haovpn**；`internal/brand` 集中常量；二进制 `haovpn-*`。
- **破坏性变更**：`haovpn0`、`haovpn.db`、`.haovpn-key`、`%ProgramData%\HaoVPN`、`HAOVPN_*` 环境变量。
- **仓库**：`.gitignore` 补 `home/`、`*.live.log`、`assets/` 等；`code-audit.md` → `docs/archive/`。
- **文档**：README/记忆/dev-log 索引；troubleshooting 链 deploy；deploy 增迁移表。
- **性能**（连接受控）：Windows 路由 ifIndex 缓存、TUN IP 就绪检测去 PowerShell。
- `go test ./...` ✅；`.\scripts\build-local.ps1` ✅；`.\scripts\dev-acceptance.ps1` ✅（27 PASS）；二进制无 myvpn 字符串。
- **回归修复**：`EnsureAdmin` 首启 sync 不清除须改密；acceptance 导出/锁定断言对齐现行行为。

### 提交说明（开发者自行 commit，AI 不执行）
```
feat: HaoVPN 首版 — 全量重命名、文档治理与 gitignore

- 模块 go-vpn → haovpn；二进制/服务/TUN/DB/凭据路径统一 HaoVPN
- 文档去重：troubleshooting 链 deploy；code-audit 归档
- gitignore 补充 home、*.live.log、assets
```

---

## 2026-08-26 · 客户端重连体感 + GUI 可用性

### 完成项
- **重连**：`DialTimeout` 默认 3s；断线后立即重拨（≤200ms）；退避上限默认 3s；配置 `dial_timeout_sec` / `reconnect.max_sec`；日志带 `dial_timeout=`/`backoff=`。
- **GUI**：可读主题（深色字）；日志 Entry 不再 Disable；`-H windowsgui` 无黑控制台；默认 exe 旁 `client.yaml` 并显示路径；非管理员 UAC `runas`，拒绝则中文提示。
- 文档：`troubleshooting.md`、`VERIFY.md`、`TEST-ACCOUNT.md` 对齐。
- `go test ./...` ✅；`.\scripts\build-local.ps1 -ClientOnly` ✅（GUI subsystem=2）。

### 诚实边界
- 不修复 ZeroTier 丢包本身；不改 UDP；不关 TLS 校验。公司机 zip 重打需用户确认后再执行。

---

## 2026-08-26 · B 后原则审计修复（完备/安全/可用/一致）

### 完成项
- **must_change_password**：隧道 `VerifyTunnelLogin` 与 Web 同语义硬拒绝；单测覆盖。
- **杀开关无泄漏**：`kill_switch=true` 时 Enable 失败**禁止** clearRoutes；`LastError`/`KillSwitchOK` 供 GUI 展示。
- **WFP**：安装前按子层 GUID 枚举删除本产品全部旧过滤器（含崩溃残留）；`SelectProductFilterIDs` 单测。
- **GoSafe**：租约 cleaner、TUN 读循环、GUI 轮询、API listener、Windows 服务客户端路径。
- **DNS**：非 Windows 明确报错「仅支持 Windows」，不打 `dns_applied`。
- **易用**：登录锁定等对外错误改中文。
- `go test ./...` ✅；`.\scripts\build-local.ps1` ✅。

### 非缺陷（诚实标注）
- Win11 家庭版 NAT、step11 field 实跑、管理 API HTTPS：不装完成。

---

## 2026-08-26 · B 档后审计修复（P0/P1 当场修完）

### 完成项
- **DPAPI**：`CRYPTPROTECT_LOCAL_MACHINE`；旧 CurrentUser blob 解密失败硬失败并要求删除重存；服务（LocalSystem）可读。
- **IP 池**：启动恢复全部 `ListActiveUserIPs`（含租约）再补 fixed；撞车单测。
- **僵尸会话**：先 `RegisterVPN` 再 `SetOnClose`；非 Connected 立刻 `RemoveIfConn`。
- **DNS**：`RestoreDNS` 不经 `ApplyDNS`；TUN recreate 只 Restore 一次。
- **TOCTOU / 锁序**：策略成功后同一 `activeMu` 校验仍为 active；Stop/onClose 均为 `activeMu`→`mu`；断线 `protectThenClearRoutes`（先 WFP 再清路由）。
- **导出 / AllowedIPs / CA**：缺 `server.crt` 导出失败；空 AllowedIPs deny；未 skip 须有效 `ca_file`。
- **杀开关**：Windows **WFP**（`FWPM_LAYER_ALE_AUTH_CONNECT_V4`）；启用失败硬失败；非 Windows 开启则启动报错。
- `go test ./...` ✅；`.\scripts\build-local.ps1` ✅（server / client / client-gui）。

### 验收注意
- 旧「保存供服务使用」凭据须管理员重存一次。
- 杀开关需管理员；日志：`killswitch enabled (WFP)`。

---

## 2026-08-26 · B 档全面加固（安全 + DNS + 杀开关）

### 完成项
- **P0 安全**：移除公钥-only 握手；`users.is_admin` + Web 仅 admin 登录；`must_change_password` 服务端强制；导出/GUI 默认 `insecure_skip_verify: false`；自签证书 SAN 扩展（`cert_sans` + listen 推导）。
- **P0 稳定**：握手前 `SetOnClose`；`dynamic_lease` 断线不 Release 池；`writeLoop` 写失败 Close；TUN 读错误不退出；断线清路由；踢旧连接异步 Close 防死锁。
- **MTU**：移除假 `ProbeMTU`，以握手 `policy.mtu` 为准。
- **DNS 推送**：`vpn.dns_servers` → `HandshakePolicy.dns_servers`；Windows `netsh` 应用/恢复 TUN DNS。
- **杀开关**：`security.kill_switch` + GUI 勾选；Windows **WFP** 阻断 AllowedIPs 出站（连接成功拆除；断线先装过滤器再清路由）。
- **WebUI**：维护页 backup/日志；monitor 增加 `reconnect_count` / `allowed_ips`。
- **Windows 服务凭据**：LocalMachine DPAPI 存 `%ProgramData%\HaoVPN\credentials`；GUI「保存供服务使用」。
- `go test ./...` ✅

### 公司机复测要点（在 2026-08-25 基础上追加）
1. 导出 yaml 含 `ca_file` + `insecure_skip_verify: false`（须带 `certs/server.crt`）。
2. GUI 勾选杀开关后断线，访问工控网段应被阻断（管理员 + WFP）。
3. 家里 `vpn.dns_servers` 配置后，公司机 `nslookup` 验证 DNS。
4. 工程师账号 Web 登录应被拒绝（非 admin）。
5. 服务模式：管理员重存凭据后 `--service start` 能登录。

---

## 2026-08-25 · 账号密码隧道登录 + Fyne GUI 客户端

### 完成项
- **握手下发 `gateway_ip`**：`HandshakePolicy.gateway_ip`；客户端 `PreferGateway` 以应答为准，导出 yaml 不再写 gateway/私钥。
- **隧道账号密码**：8443 握手 `username+password`；服务端 `VerifyTunnelLogin`；应答下发 `client_private_key`（仅 TLS 内）。
- **稳定性**：默认心跳 15s/90s（服务端+客户端）；组播/`0.0.0.0` 入站降为 DEBUG；`reconnect` 计数修复。
- **`internal/clientapp`**：CLI/GUI 共用拨号引擎。
- **`cmd/client-gui`**：Fyne 登录窗、状态/日志、托盘、重连/退出登录。
- 导出 zip：`auth.username` + server/tls/心跳；公司包含 `haovpn-client-gui.exe`。
- `go test ./...` ✅；`build-local` 出 `haovpn-client.exe` + `haovpn-client-gui.exe`。

### 公司机复测要点
1. 重启家里服务端（新心跳 90s）。
2. 重新 `pack-company-client-test.ps1` 打 zip。
3. 管理员运行 GUI，账号密码登录；`ping 10.88.0.1 -t`（ZT 可间歇）。
4. 带回 `diag-*.zip` + `RESULT-TEMPLATE.md`。

---

## 2026-08-25 · 修复客户端 ping 不通网关 10.88.0.1

### 现象
- 公司机握手成功后 ping `10.88.0.1` 不通；日志大量 `隧道发送失败: not connected`。
- 服务端能看到组播/`0.0.0.0` 丢弃，但无 ICMP 到网关的入站（包根本没进隧道）。

### 根因
1. **Windows 路由**：TUN 为 `/32`，`route ADD … via 10.88.0.1` 下一跳不在链路，流量进不了 Wintun。
2. **握手竞态**：`applyPolicy`（创建 Wintun）阻塞在登记 `activeConn` 之前；TCP 心跳超时后仍把死连接设为 active → not connected 刷屏。

### 修复
- Windows：`route ADD dest MASK mask 0.0.0.0 IF <ifIndex>`（on-link）；先加网关 `/32`。
- 客户端：先挂 crypto/`activeConn` 再 `applyPolicy`；策略期间断线则清空；TUN 读循环对非 Connected 静默跳过。
- 单测 `TestWindowsOnLinkRouteArgs`；`build-local` 已出新 client。

### 说明
- 服务端 WinNAT/ICS 失败（家庭版）只影响访问 LAN，**不影响** ping 网关；gateway ping 通后再谈 LAN。

---

## 2026-08-25 · 手动 VPN IP + 客户端无策略字段可启动

### 问题
- `fixed` 开户只能自动分配，不能指定（PLC 白名单不便）。
- 新导出 yaml 已去掉 `vpn_ip`/`allowed_ips`，但客户端 `Validate` 仍强制 → **公司机解压后起不来**。
- `applyPolicy` 在 IP 不变改 AllowedIPs 时不清旧路由。

### 修复
- 开户/策略：`fixed` 可选手动 `vpn_ip`；`dynamic_*` 禁止指定。
- 客户端：`vpn_ip`/`allowed_ips` 可选；握手后清路由再重建；`ResolveGatewayFor`。
- 实跑：指定 `10.88.0.55` 成功；导出 LoadClient+Validate OK；动态+指定 → 400。
- `go test ./...` ✅；`build-local` 已出新 `bin/haovpn-client.exe`。

---

## 2026-08-25 · 修复 home.db 无 schema_meta 时迁移 FATAL

### 问题
- 现场 `home/data/haovpn.db` 有 `peers`、**无** `schema_meta`；`migrateV1ToV2` 查版本表直接 FATAL。
- 先前单测 fixture 带了 `schema_meta`，未覆盖真实库形态（验收疏漏）。

### 修复
- 迁移前 `CREATE TABLE IF NOT EXISTS schema_meta`。
- 单测改为无 `schema_meta`；新增 `TestMigrateHomeLikeDBNoSchemaMeta`；并对真实 home.db 做了 Open 验证（已备份 `haovpn.db.bak-pre-v2`）。

---

## 2026-08-25 · VPN 账号与策略合一（物理合并，不留债）

### 完成项

- **DB v2**：`users` 承载 Web+隧道身份；删除 `peers`；`peer_id`→`user_id`；启动自动 v1→v2 迁移（含死锁修复）。
- **API**：`POST /users` 一步开户（密钥/IP）；`PATCH /users/{id}/vpn`；导出 `/users/{id}/export.zip`；删除 `/api/v1/peers`。
- **握手**：应答下发 `vpn_ip/allowed_ips/mtu/ip_mode/policy_ver`；客户端先握手再建 TUN/路由。
- **IP 模式**：`fixed` / `dynamic_session` / `dynamic_lease` + 租约清理 ticker。
- **入站校验**：src=VPNIP、dst∈AllowedIPs、横向隔离。
- **WebUI**：账号页合一；`/peers`→`/users`；sidebar 去掉 Peer。
- **脚本/文档**：acceptance、field-gate、meta-plan、deploy、记忆 已同步。

### 验证

- `go test ./...` ✅（含迁移、握手、入站、acceptance、security_checklist）

### 迁移注意

- 存量 `home/data/*.db` 启动时自动升级；admin 保留；每用户仅保留一条 peer 合并进 users。
- **不修改 VERSION**。

---

## 2026-08-24 · 硬门禁脚本（field gate）

### 新增

- `scripts/dev-field-gate.ps1`：`require_tun=true` + `nat.enabled=true` 真路径；live.log 断言；zip 导出客户端；PLC ping + monitor 流量；Windows 服务 install/start/stop。
- `scripts/lib/field-common.ps1`：field 共用 YAML/日志函数。
- Phase A 硬断言单测：`hardgate_test.go`、`migrate_keys_test.go`、`mtu_probe_test.go`、`health/status_test.go`；握手重连 `reconnect_count` 断言。
- `dev-acceptance.ps1` 明确为 **smoke**（不再假装 WithSudo 测 TUN）。
- 客户端握手成功后打 `MTU 探测已发送` 日志（field 可观测）。

### 交付门禁（须全部满足）

1. `go test ./...`
2. `.\scripts\dev-acceptance.ps1`（smoke）
3. `.\scripts\dev-field-gate.ps1 -PlcHost <工控IP> [-UseHomeConfig]` → **0 FAIL**
4. 手工：重启后服务自连 → dev-log 写「step11.12 reboot OK」

### 验证（开发机）

- `go test ./...` ✅
- `dev-acceptance.ps1` ✅ smoke（28 PASS / 1 SKIP → field）
- `dev-field-gate.ps1 -PlcHost 192.168.1.10 -SkipSmoke` ❌ **2026-08-24 实跑**：当前 shell 非管理员，sudo 未获 UAC → `haovpn-server` 未启动，health 无响应
- **field 全绿须**：管理员 pwsh 7 + 批准 UAC + 可达 PLC

### field 实跑失败根因（2026-08-24）

- `Start-Process sudo` 在无 UAC 批准时 wrapper 进程仍存活，但 `haovpn-server.exe` 从未启动
- 脚本已加强：`Wait-FieldServerProcess`、health 最后响应诊断、live.log 收紧（须 `New-NetNat`/`MASQUERADE`/`ICS 已启用`）
- **Windows 11 家庭版**：`New-NetNat` 因无 Hyper-V 报 `0x80041010` → 已实现 **ICS 回退**（`internal/netstack/route_windows.go`）

---

## 2026-08-24 · v1.0 真实收尾（对照 meta-plan 补齐缺口）

### 已落地（原被误标为 v1.1 的 v1.0 必做项）

1. **私钥 AES-256-GCM 落库**：`internal/security/keyenc.go` + `data/.haovpn-key` / `database.encryption_key`；启动迁移明文行。
2. **防重放窗口**：`wg_crypto` 单调 counter nonce + `wireguard/replay`；`TestReplayRejected`。
3. **IP 池持久化**：启动 `ListAllPeers` → `AllocateSpecific`；`ip_allocations` 写入/回收；`TestIPPoolReloadFromPeers`。
4. **Peer VPN IP 横向隔离**：`HandleInbound` 丢弃发往其他 peer 虚拟 IP 的包；`TestLateralVPNIPBlocked`。
5. **zip 配置包**：`GET /api/v1/peers/{id}/export.zip` + WebUI；YAML 导出保留。
6. **日志快照**：`GET /api/v1/logs?tail=N`；health/Dashboard `recent_errors`。
7. **文件权限 WARN**（非 Windows）；**reconnect_count** 重连递增；**macOS NAT** `nat.enabled` 时返回 error（不假装成功）。
8. **客户端 ProbeMTU**；**panic 存活** `TestGoSafeBusinessPathSurvivesPanic`。

### 验证

- `go test ./...` ✅
- `dev-full-test.ps1` ✅
- `dev-acceptance.ps1` ✅ **28 PASS / 0 FAIL / 3 SKIP**（sudo TUN、client≤10s、PLC）
- `build-release.ps1` ✅ 6 平台 × server/client + zip

### 仍需手工（step11 实机，不假装完成）

1. **管理员 pwsh 7**：`.\scripts\dev-field-gate.ps1 -PlcHost <工控IP> -UseHomeConfig` → 0 FAIL
2. 手工：重启后服务自连 → dev-log 写「step11.12 reboot OK」

（`dev-acceptance.ps1 -WithSudo` 已移除；TUN/NAT 仅 field gate 验收）

---

## 2026-08-24 · 第 5 轮：补齐可修项 + 全量测试

### 已修

- `vpn.require_tun: true`（默认）：TUN 失败 Fatal；`home/server.yaml` 已设
- `nat.enabled=true` 时 netstack Setup 失败 Fatal
- `/api/v1/health` 增加 `tun_ok`/`nat_ok`，`ok` 综合 DB+TUN+NAT
- 配置校验：subnet/gateway 归属、NAT/隧道白名单 CIDR、listen 格式
- AuditEntry/ConnectionEvent JSON tag；RouteOutbound 按 peer ID 排序
- 客户端 Send 失败 WARN；Windows 服务 Start 检查错误
- `manager.go` 格式规范化

### 验证

- `go test ./...` ✅
- `dev-full-test.ps1` ✅
- `dev-acceptance.ps1` ✅ 25 PASS
- `pack-zt-field-test.ps1` ✅

---

## 2026-08-24 · 第 3 轮全局审查

### 新发现并已修

1. **隧道加密密钥不对称**（P0）：旧 `priv XOR peer` 致双方密钥不同；改 X25519+SHA256，`TestCrossSessionRoundTrip` 互通测试。
2. **客户端握手竞态**（P0）：先注册 `SetOnData` 再 `SendRaw`。
3. **peer 禁用无 API**（P1）：补 `POST /api/v1/peers/{id}` `action=disable|enable` + WebUI 按钮。
4. **删 peer 不踢线**（P1）：`DELETE` 前 `KickPeer`。
5. **每包写 SQLite**（性能）：`HandleInbound` 5s 节流落库。

### 验证

- `go test ./...` 通过；`dev-full-test` + `dev-acceptance` 通过
- `pack-zt-field-test.ps1` 已更新 `dist/HaoVPN-zt-field-test.zip`
- Windows TUN：netsh 失败 PowerShell 回退 + 等待 IP 就绪
- 导出 YAML 含 `ca_file`；peer 禁用进验收脚本

---

## 2026-08-24 · 认真审查：去掉假实现

### 发现的严重问题（不是猜测，代码即证）

1. **Windows netstack 空壳**：`addRoute`/`setupNAT` 只打 INFO 就 `return nil`，日志却写 “setup complete”。
2. **Linux NAT 源网段写反**：`-s LanCIDR` 应为 `-s VPNSubnet`；且硬编码 `-o eth0`。
3. **Linux TUN 未配 IP**：创建适配器后没有 `ip addr`/`ip link up`。
4. **Windows 客户端 route 命令错误**：`route ADD cidr IF name` 非法；应为 `ADD dest MASK mask gateway`。
5. **导出配置**：`0.0.0.0:8443` 被改成 `127.0.0.1`，跨网必连失败。
6. **Wintun 空读退出 / 未配 IP**：此前已修（ReadWait + netsh）。

### 修复

- 重写 `internal/netstack`：真实转发 + Linux MASQUERADE + Windows New-NetNat；失败返回错误。
- Linux TUN：`ip addr replace` + `ip link set up`。
- 客户端路由带 gateway；导出用 `REPLACE_WITH_SERVER_IP`。
- Setup 失败打 ERROR，不假装成功。

---


### 根因修复

- **SQLite 并发写 SQLITE_BUSY**：`Open()` 增加 `busy_timeout=5000`、`MaxOpenConns(1)`，日志标明 WAL 参数。
- **TLS 回声间歇超时**：`AcceptConn` 后须 `SetOnData`，避免回调时 conn 未赋值；并发测试预热 + `sync.Once` 关 channel。
- **发送队列满静默丢包**：`SendRaw`/`ProbeMTU` 队列满时打 WARN 日志（`frame dropped`），并发测试增大 `MaxQueueSize`。
- **登录锁定**：`isLocked` 未锁定时不再清除失败计数（此前会话已修）。

### 新增测试

- 并发：`ippool`/`auth`/`persist`/`sessionmgr`/`api`/`transport` 包并发单测。
- 安全清单：`TestSecurityChecklistMetaPlan`、`TestBuildClientExportYAML`。
- 验收脚本：`dev-acceptance.ps1`（虚拟 `accept-tmp` 环境）。

### 验证

- `go test ./... -count=3` 全部通过。
- `.\scripts\dev-acceptance.ps1` PASS=24 FAIL=0。

---

## 2026-08-24 · v1.0 收尾（WebUI / 安全 / 服务）

### 完成`web/embed.go` + 5 个中文 HTML 页面；`internal/api/pages.go` 从 embed 加载模板。
- **Monitor API**：`/api/v1/monitor/online|peers|events`。
- **安全测试**：CSRF、登录锁定、bindcheck、Dashboard 401；`dev-security-check.ps1`、`dev-full-test.ps1`。
- **Bug 修复**：`auth.isLocked` 在未锁定时误清失败计数，导致锁定永不触发。
- **Windows 服务**：`service_windows.go` SCM 停止时调用 `stopClient()` 运行完整 VPN 逻辑。
- **文档**：`deploy.md` 补充 launchd 示例与脚本索引。

### 验证

- `go test ./...` 全部通过。
- `.\scripts\dev-full-test.ps1`：单元测试 + E2E + 安全配置检查。

### 待办

- [ ] 现场多客户端实机验收（`sudo .\bin\haovpn-server.exe` + 真实 PLC ping）

---

### 完成

- `internal/tunnel` 握手 + 集成测试；peer 配置导出 API。
- `install-wintun.ps1`、`dev-e2e.ps1`；规则补充 Windows `sudo`。

### 问题与踩坑

- YAML 反斜杠路径解析失败；wintun.dll 与 exe 同目录；TUN 需 sudo。

---

## 2026-08-23 · 项目启动与文档体系

### 完成

- 确定 v1.0 目标：工控现场自托管 VPN，安全 > 简单 > 快。
- 完成 [meta-plan.md](meta-plan.md)：功能清单、目录结构、step1～11 开发顺序。
- **文档体系治理**（2026-08-23 第二轮）：
  - 规划文档移至 `docs/meta-plan.md`
  - 新增 [development-principles.md](development-principles.md) — 开发原则（不敷衍、实事求是、测试/日志验证）
  - 新增 [deploy.md](deploy.md) — 完整部署、验收、脚本索引
  - 新增 [dev-log.md](dev-log.md) — 本文件
  - 新增 [security-hardening.md](security-hardening.md)、[troubleshooting.md](troubleshooting.md)
  - 根目录 [记忆.md](../记忆.md)、[README.md](../README.md)
  - [scripts/](../scripts/) — build-release、dev-gen-certs、dev-smoke-test、dev-security-check、frp 示例
- 关键设计决策：
  - 持久化用 **SQLite**，配置用 **YAML**，首次启动自动生成带注释的默认配置。
  - 管理口默认不绑公网；`0.0.0.0` 须 `allow_public_bind: true` 且用户自担风险。
  - 全平台：Linux / Windows / macOS。

### 待办（下一步）

- [x] step11 单元/集成测试 + `dev-e2e.ps1` 冒烟
- [ ] 现场多客户端实机验收（`sudo .\bin\haovpn-server.exe`）

---

## 2026-08-23 · v1.0 代码落地（step1～9）

### 完成

- **规则**：`.cursor/rules/HaoVPN.mdc` 增加「做一步测一步」与「中文详细注释」要求。
- **step1**：`internal/logger`、`internal/safeutil` + 单元测试。
- **step2**：`internal/transport` 帧协议、buffer 池、TLS、心跳、重连 + 粘包/TLS 回声测试。
- **step3**：`internal/tun` Linux/Windows(Wintun)/macOS(utun) 平台实现。
- **step4**：`internal/crypto` WireGuard 密钥 + chacha20poly1305 封装 + 测试。
- **step5**：`ippool`、`persist`(SQLite)、`auth`、`sessionmgr`、`netstack` + 部分测试。
- **step6**：`internal/config` YAML 加载/首次生成/校验 + 测试（含 bindcheck 拒绝 0.0.0.0）。
- **step7-8**：`internal/api` WebUI/API、`health`、`security/cert` 自签、`audit`。
- **step9**：`cmd/server`、`cmd/client`（含 Windows `--service` 骨架）；`go test ./...` 通过；`build-local.ps1` 产出 `bin/`。
- **依赖**：`go mod tidy` 拉齐 wireguard/wintun/sqlite/yaml 等。
- **示例**：`config/server_example.yaml`、`config/client_example.yaml`。
- **构建**：`build-common.ps1` ldflags 改为注入 `internal/version`。

### 问题与踩坑

- **wireguard-go API**：`NoisePrivateKey.Generate()` 不可用，改用 `curve25519` 按 WG 规范生成密钥。
- **transport TLS 测试**：回声回调中 `serverConn` 赋值竞态导致 panic，改为闭包捕获 `sc` 指针。
- **sessionmgr 测试**：Windows 上 SQLite 文件未 Close 导致 TempDir 清理失败。

### 待办

- [x] step11 自动化测试与 E2E 冒烟
- [ ] 现场多客户端 + PLC ping

### 关联

- `internal/*`、`cmd/server`、`cmd/client`、`config/*`

---

## 2026-08-23 · 项目启动与文档体系

- 新增根目录 `VERSION`（当前 `0.1.0-dev`），**仅开发者可改，AI 禁止修改**。
- 新增 `docs/versioning.md`、`.cursor/rules/HaoVPN.mdc`（版本/Git 硬性规则）。
- 构建脚本全面升级：
  - `scripts/platforms.txt` — 6 平台（linux/win/darwin × amd64/arm64）
  - `scripts/build-release.ps1` / `.sh` — 全平台 + zip + manifest.json
  - `scripts/build-local.ps1` — Windows 本机快速构建 → `bin/`
  - `scripts/lib/build-common.ps1` — 读 VERSION、注入 ldflags
- 开发环境约定：Windows + pwsh7 + Go 1.26；Git 仅开发者提交，禁止 Co-authored-by。

### 踩坑

*（暂无 — 代码开发开始后在此记录）*

---

## 日志模板（复制使用）

```markdown
## YYYY-MM-DD · 简短标题

### 完成
- 

### 问题与踩坑
- **现象**：
- **原因**：
- **解决**：
- **教训**：

### 待办
- [ ] 

### 关联
- PR / commit / 相关文件：
```

---

*维护说明：只追加，不删历史；过时内容用删除线或注明「已废弃」*
