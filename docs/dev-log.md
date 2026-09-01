# 开发日志

> 记录开发历程、重要决策、踩坑与待办。**每完成一块工作或遇到值得记的问题，追加一条。**
> 格式：## YYYY-MM-DD · 标题 + 正文。
> **本文件是进度唯一来源**；勿在 README / 记忆 / docs 索引里再堆「第 N 轮摘要」。

---

## 2026-09-01 · 软换 VPN IP 不通：ICS 在场禁止强制 /32

根因：冷启 PreferVPN 用 ics_prefix_keep 保住 ICS 扩成的主机 /24；pn_ip_inplace 却 ReplaceTUNIPv4KeepICS(..., 32)，等于再做一次 prefix_fix，NAT 死。Soft 路径也不应 Restart（会冲 137）。

修复：有 has_137 时软换用 prefix **24** + SkipAsSource；日志 pn_ip_inplace keep_ics_prefix=24。

---

## 2026-09-01 · ICS：回滚叠料；每次共享无条件 Force Restart

现场：Soft/KeepICS/restart_only 等检测常绕开真正有效的一步。手工 `Restart-Service SharedAccess -Force` 即可通；有无 137 无关。

### 改动（相对 HEAD 仅两刀）

1. `PSSnippetICSEnableSharing`：**无条件** `Restart-Service SharedAccess -Force` → Enable → PreferVPN；删除 Soft `already_paired` / Ensure-first
2. PreferVPN：`ics_prefix_keep`（禁 `prefix_fix`）

HardRestart 仍全清再冷启（无 KeepICS）。每次 ICS 日志必有 `ics_sharedaccess action=restart`。

### 验收

公司 ping 家 LAN；HardRestart 后仍通（可接受 15–25s）；**不得** `already_paired` 跳过 Restart。

---

## 2026-09-01 · 架构审计第 29 轮（PS 集中 / cmd 门面 / 死代码 / 文档）

### 结论

第 28 轮剩余项全部闭环：ICS EnableSharing 与 egress PS 迁入 `ps_snippets.go`；`cmd/client*` 读服务状态经 `clientapp.ServiceAutostartStatus`；PreferVPN 组装收口；删除未用导出符号；文档/CODEMAP 治理。**User DPAPI 未做**。

### 改动

| Phase | 做法 |
|-------|------|
| P1 cmd 门面 | `cmd/client` / `cmd/client-gui` 禁 direct `autostart.ServiceStatus`；`cmd/README` 单实例文案 |
| P2 PS | `PSSnippetICSEnableSharing`、`PSSnippetFindInterfaceInCIDR/ByRoute` + 单测 |
| P3 组装 | `ps_assemble_windows.go`：`PSAssignAdapterAndPreferVPN`、`PSAssignAdapterAndSkipAsSourceOnly` |
| P4 死代码 | 删 `teardownViaExit`、`removeFiltersLocked`、`Stack.ICSPlan`、`SetInterfaceDNSStatic`、`ReconnectClient.Conn`；删非 Context ICS/PreferVPN 薄包装 |
| P5 文档 | 符号修正（HardRestart DNS settle）；internal/README 补包；薄 doc.go；CODEMAP 去重 |
| P6 注释 | `route_policy.go` ExitLAN 旁路条件注释 |

### 验收

```text
go test ./internal/clientapp/... ./internal/clientgui/... ./internal/netutil/... ./internal/netstack/... ./internal/winnet/... ./internal/tun/... ./internal/transport/... ./internal/sessionmgr/... -count=1 -skip TestWaitDNSReady
go build ./...
```

---

## 2026-09-01 · 架构审计第 28 轮（门面补全 / 死代码 / PS / 文档）

### 结论

审计闭环：补齐 `clientgui→autostart` 违规、删除 DNS 查询死链与重复单测、netutil/PS 小步集中、GUI 单实例/错误/退出文案对齐。**User DPAPI 未做**。

### 改动

| Phase | 做法 |
|-------|------|
| P1 门面 | `ServiceStopAndWait` / `DefaultServiceStopTimeout`；`service_takeover` 禁 direct autostart |
| P2 死代码 | 删 `QueryInterfaceDNS` 链、`ShowFatalError`、`bumpOpGenLocked`；unexport `waitDNSReadyAbort`/`hasDefaultRouteOnInterface`；`ServiceAutostartDisable` 清理 cred |
| P3 netutil | `ICSPrivateIPv4Wildcard`、`InterfaceNameLooksLikeTUN`、`PreferSkipAsSourceNeedsUpdate` |
| P4 PS | `PSSnippetAssignIPv4`、`RemoveNonVPNKeepVPN`、`ProbeICSResidue`、`VerifyInterfaceExists`、`ScrubDefaultRoute` |
| P5 GUI | `SingleInstanceUserMessage`、`prepareGUIEngine`、`finishQuitApp`、`FormatConnectFailure` 统一 |
| P6 文档 | doc.go 补全、architecture 入口示例、dev-log/记忆 |

### 验收

```text
go test ./internal/clientapp/... ./internal/clientgui/... ./internal/netutil/... ./internal/netstack/... ./internal/winnet/... ./internal/tun/... ./internal/sessionmgr/... -count=1 -skip TestWaitDNSReady
go build ./...
```

---

## 2026-09-01 · 架构解耦第 27 轮（引擎契约 / netutil / GUI 去重 / 文档）

### 结论

入口层与叶子算法第二轮收口：GUI 登录复用 `PrepareEngine`/`StartAndWaitFirstAuth`；纯 IP/CIDR/DNS 逻辑进 `netutil`；GUI 网络操作前奏统一；PS 模板集中；文档 CODEMAP 单一权威。**User DPAPI / 记住密码未做**。

### 改动

| Phase | 做法 |
|-------|------|
| P1 引擎契约 | `engine_bootstrap.go`：`PrepareEngine`、`StartAndWaitFirstAuth`、`DefaultGUIRunOptions`；`login.go` 删除 magic 45s；删 `loginFailUserMessage` 重复 |
| P2 死代码 | 删 `readDNS`、重复 PS 转义测试；unexport `warmupTun`/`waitDNSReady`/`managedRouteFromTunnel`/`setOnDataplaneFailed`；修 `decideHardRestartFinish`；瘦 `winnet_facade` |
| P3 netutil | `ProbeIPForCIDR`、`IPv4IsICSPrivate`、`FilterDNSServersPoison`、`IsVirtualInterfaceName`、`ParseLocalLANsField` + 单测 |
| P4 自启 | `autostart_facade.go`；`tray_config`/`service_takeover` 薄化 |
| P5 GUI | `beginNetworkOp`；`tray_state.go`/`trayPresentationFromEngine` |
| P6 PS | `PSSnippetEnableIPv4Forwarding`、`PSSnippetGetNetNatMatch` |
| P7 UX | `ShouldStopReconnectOnDial`（消歧 `dialerr.IsFatalDialError`）；`MergeConnectWarns` 导出 + 文案分工注释 |
| P8 文档 | architecture 唯一 CODEMAP；codebase-guide/internal/README/cmd/deploy/dev-log/记忆 同步 |

### 验收

```text
go test ./internal/clientapp/... ./internal/clientgui/... ./internal/netutil/... ./internal/netstack/... ./internal/winnet/... ./internal/tun/... -count=1 -skip TestWaitDNSReady
go build ./...
```

---

## 2026-09-01 · CLI/GUI 启动契约对齐（bootstrap 下沉）

### 结论

数据面（`Engine` / `runtime_policy` / `via_exit`）本就共用；差异在**入口编排**。CLI 缺 TUN 预热、首连 FailFast、用户告警 stderr，导致冷连慢 ~1s、错密码空转重连。

### 改动

| 项 | 做法 |
|----|------|
| bootstrap | `RunOptions` + `runClient`；`RunCLI`（交互）vs `RunServiceLoop`（服务） |
| 预热 | `StartWarmupAsync`；CLI/服务/GUI 均经 `clientapp`；日志 `client_bootstrap warmup=` |
| 首连 | CLI `FailFastFirst` + `WaitConnected(45s)`；`FormatConnectFailure` 与 GUI 共用 |
| 告警 | `SetUserWarnSink` → CLI `client_user_warn` + stderr；GUI 仍 `ics_hint_shown_to_user` |
| IPv6 | warmup Create 后 `tun_warmup stage=disable_v6`（避免 reuse 跳过） |
| cmd/client | 非 admin stderr；单实例冲突 `SingleInstanceHint`（含服务占用提示） |
| GUI | `StartWarmupAsync` + `AttachDataplaneHook` 薄封装 |

### 验收

```text
go test ./internal/clientapp/... ./internal/clientgui/... ./internal/tun/... -count=1 -skip TestWaitDNSReady
go build -o bin/haovpn-client.exe ./cmd/client
```

| 场景 | 期望 |
|------|------|
| CLI 冷连 | `client_bootstrap warmup=done` + `reuse from_warmup` + `adapter=reuse` |
| CLI 错密码 | 45s 内 `first_auth=fail` 退出 |
| CLI ICS 多网卡 | stderr `client_user_warn` 全文 |
| 软换 IP | 仍 &lt;500ms（不变） |

---

## 2026-09-01 · 软换 IP iphlp SkipAsSource（消除 ~6s PS）

### 现场（scrub 快路径已验证）

| 场景 | 实测 policy_elapsed | 关键日志 |
|------|---------------------|----------|
| 冷连 Home+ICS | **12.65s** | `tun_default_route_scrub method=skip elapsed=0s`（两次） |
| 软换 IP（仍慢） | **6.38s** | `prefer_vpn_light method=skip_only elapsed=6.05s`（PS 冷启） |
| 功能 | OK | `prefix=32`；`did_setup=false`；DNS `changed=false` |

根因：软换轻量路径仍 `RunPSOneShot`（含 AssignAdapterIf），SkipAsSource 本可用 IP Helper `SetUnicastIpAddressEntry` 亚秒完成。

### 改动

| 项 | 做法 |
|----|------|
| iphlp | `iphlp_skipas_windows.go`：`ApplyPreferVPNSkipAsSource`（noop/iphlp）；`UnicastIPv4Entry.SkipAsSource` |
| prefer_vpn_light | 主路径 iphlp/noop；PS 仅 fallback 且去掉 AssignAdapterIf |
| 路由 | 软换 gw/allowed 未变 → `vpn_ip_inplace routes=keep`（`routeListsEqual`） |

### 验收

| 场景 | 目标 |
|------|------|
| 冷连 | 保持 ~12–13s |
| 软换 IP | **&lt;500ms**；`prefer_vpn_light method=noop\|iphlp`；**无** `method=skip_only`/`method=ps` 6s |

```text
go test ./internal/winnet/... ./internal/netstack/... ./internal/clientapp/... -count=1 -skip TestWaitDNSReady
```

---

## 2026-09-01 · ICS 默认路由修复后性能回归（冗余 PS scrub）

### 现场（功能已通过，变慢有证据）

| 场景 | 修复前 policy_elapsed | 慢点 |
|------|----------------------|------|
| 冷连（Home + ICS 首次） | ~25.6s | ICS COM ~14s（平台）+ **两次** `tun_default_route_scrub method=ps count=0` 各 ~4–5s |
| 软换 VPN IP | ~11.5s | 完整 `PreferVPNSourceAfterICS` PS ~6s + 重复 scrub PS ~3s |
| 自动重连 noop | 已快 | `dataplane_keep`；无 ICS |

根因：`DeleteDefaultRouteOnInterface` 在 iphlp 之后**无条件** `RunPSOneShot`；装路由后再 scrub 与嵌入 PreferVPN 重复。

### 改动

| 项 | 做法 |
|----|------|
| 快路径 | `HasDefaultRouteOnInterface`（GetIpForwardTable2）；无路由 `method=skip`；iphlp 删成功不 PS |
| 模式 | `ScrubDefaultRouteFast`（软换/装路由后） vs `ScrubDefaultRouteLate`（ICS 后，iphlp 失败才 PS） |
| 去重 | `ics_enable` Late 一次；`runtime_policy` 仅 `viaDidSetup` 时 Fast scrub |
| 软换 | `PreferVPNAfterSoftIPReplace` + `PSSnippetSkipAsSourceOnly`（Replace 已 /32） |
| DNS | 软换 poison 后 servers 未变则跳过重装 |

### 验收指标

| 场景 | 目标 |
|------|------|
| 冷连首次 ICS | **&lt;18s**（~14s COM + 其它 &lt;3s）；日志 `method=skip\|iphlp`，无两次 `method=ps count=0` |
| 软换 IP | **&lt;2s**；`prefer_vpn_light method=skip_only` |
| HardRestart | ~15s stop + ~14s 再连（预期，非 bug） |

### 验证

```text
go test ./internal/winnet/... ./internal/netstack/... ./internal/clientapp/... -count=1 -skip TestWaitDNSReady
```

---

## 2026-09-01 · 修复 ICS 注入默认路由（在线改 VPN IP）

### 根因（现场日志，非猜测）

- `route print`：`0.0.0.0/0` 经 TUN（如 `10.88.0.4`）跃点 **5** → ICS 在 TUN 私网侧注入；HaoVPN `route_ops` **从不**装默认路由。
- `ics_src_diag … prefix=24`：EnableSharing 把 TUN **主机地址**从握手 `/32` 扩成 `/24`（与 AllowedIPs 分流表无关）。
- 仅改 IP ~20～26s：旧 `viaFingerprint` 含 tunIP → `dataplane_clear` 拆 ICS 再装。

### 改动

| 项 | 做法 |
|----|------|
| PreferVPN | `PSSnippetPreferVPNAfterICS` + 独立路径复用：Prefix≠32 → Remove+New `/32`；清本 ifIndex 的 `0.0.0.0/0`；日志 `ics_prefix_fix` / `ics_default_route_scrubbed` |
| Go 纵深 | `winnet.DeleteDefaultRouteOnInterface`；ICS 后与 `applyPolicy` 装路由后 `ScrubTUNDefaultRoute` |
| 软换 IP | `ReplaceTUNIPv4KeepICS`；`viaFingerprint` 仅 `lans\|subnet`；`vpn_ip_replace_inplace` **不**清 viaFP、不拆 ICS |
| 门面 | `ScrubTUNDefaultRoute` / `PreferVPNSourceAfterICS` / `ReplaceTUNIPv4KeepICS` |
| 文档 | troubleshooting / traffic-routing / architecture / 记忆 / internal README |

### 验证

```text
go test ./internal/winnet/... ./internal/netstack/... ./internal/clientapp/... -count=1 -skip TestWaitDNSReady
```

现场（管理员）：改 VPN IP 后 `route print` 不得再有经 TUN 的 `0.0.0.0` 跃点 5；`ics_src_diag` vpn **prefix=32**；仅改 IP 无 `DisableICSPair`/`ics_enable` 长耗时。

---

## 2026-09-01 · 架构收口第 26 轮（抽取 / 去死 / 文档）

### 动机

第 1～25 轮叶子包已成型；审计仍有：噪声源/截止判定/NetNat 模板散落、门面死导出、via 清理不可取消、`register`/`ics_nat` 过胖、architecture 轮次长文与 §8/§10 交叉冲突。本轮一次性收口，**不新建工具包**，**不做** User DPAPI。

### 改动

| 项 | 做法 |
|----|------|
| netutil | `IsTUNNoiseSource`；`ResolveGateway(handshakeGW, vpnIP)` 去掉 yamlGW |
| safeutil | `IsDeadline`；`clientgui/login_fail` 去字符串比对；`sessionmgr/stats` 节流改 `AllowEvery` |
| winnet | `PSSnippetNewNetNat`/`PSSnippetRemoveNetNat`；删死 `IsWintunOrphanName` |
| netstack 门面 | 仅 Configure/Has/Cleanup(Context)/Remove；删 PreferVPN/Disable* 死导出 |
| clientapp | `CleanupICSResidueContext` + ctx；`WaitDNSReady` 薄包装注明生产用 Abort |
| sessionmgr | 删 `ErrAccountAlreadyOnline` 别名；`register_grace.go` / `register_lan.go` |
| netstack ICS | `ics_egress_windows.go` + `ics_enable_windows.go`（原 `ics_nat_windows.go`） |
| 文档 | architecture 删第 12～25 轮长段；FAQ/CODEMAP/internal README/记忆/traffic-routing/hardening 对齐 |

### 未做

- GUI 记住密码 User DPAPI（仍 yaml 明文 + ACL）
- 家庭版 ICS EnableSharing 抖链路（平台残留，仅文档标明）
- field gate / 重启自连（现场验收，非本轮代码）

### 验证

```text
go test ./internal/netutil/... ./internal/safeutil/... ./internal/winnet/... ./internal/netstack/... ./internal/sessionmgr/... ./internal/clientapp/... ./internal/clientgui/... -count=1 -skip TestWaitDNSReady
go test ./... -count=1 -skip TestWaitDNSReady
```

---

## 2026-09-01 · 分流过滤抽公共（netutil / safeutil）

### 动机

越权目的双层过滤落地后，客户端 `dstAllowedForUpload` 与服务端 `dstAllowed`、噪声判定、WARN CAS 限频在多包重复；另有死 `224–239` 分支与冗余 `IsLimitedBroadcast` OR。

### 改动

| 项 | 做法 |
|----|------|
| netutil | `IPInAnyNet` / `VPNIPOrInNets` / `IsTUNNoiseDst` / `IsTUNNoiseForLog`；修 `IsLimitedBroadcast` godoc |
| safeutil | `AllowEvery`；sessionmgr 双 WARN、transport queue full、tun quiesce 改用 |
| 清理 | 删死组播尾支、冗余 OR、本地 `dstAllowedForUpload` |
| 行为 | 不变：LL-unicast 仍仅服务端日志降 DEBUG，客户端不上送早丢不加 |

### 验证

```text
go test ./internal/netutil/... ./internal/safeutil/... ./internal/clientapp/... ./internal/sessionmgr/... ./internal/transport/... -count=1 -skip TestWaitDNSReady
```

---

## 2026-09-01 · 重连后「丢弃越权目的 IP」刷屏

### 现场

握手完成后约十余秒（`StateConnected` 放开上送）服务端狂刷 WARN：`丢弃越权目的 IP … dst=223.5.5.5/CDN/192.168.31.1`；客户端业务正常。

### 根因

服务端 `dstAllowed` **正确**丢弃 AllowedIPs 外目的（非漏洞）。缺口：客户端 `shouldUploadTUN` 只验源不验目的 → OS 经 TUN 注入的 DNS/公网噪声上送；越权 WARN **无限流**（伪造源已有 10s 限流）。

### 改动

| 项 | 做法 |
|----|------|
| 客户端 | `allowedNets` + `shouldUploadTUN` 要求 dst∈AllowedIPs（或本机 VPN IP） |
| 服务端 | `shouldWarnDstOverreach` 10s 限频 + `drops=` |
| DNS | `MergeDNSIntoAllowedIPs`：策略 DNS /32 并入下发 AllowedIPs（会话+客户端） |

### 验证

```text
go test ./internal/clientapp/... ./internal/sessionmgr/... ./internal/tunnel/... ./internal/vpnaccount/... ./internal/netutil/... -count=1 -skip TestWaitDNSReady
```

---

## 2026-09-01 · 在线改 VPN IP 卡死（双路径：同 IP 保 / 变 IP 换）

### 现场

管理台改账号 VPN IP（如 `10.88.0.2`→`10.88.0.4`）后 soft/Hard/退出登录都修不好，须杀进程；`ics_src_diag` 新旧 IP 并存；`RestoreDNS servers=[旧 VPN]`；Connected 后刷 `send queue full`。同 IP 重连已快——**不是缺判断**，是变 IP 分支落地不全。

### 根因

Wintun `Close` 保留适配器（同 IP 要快，合理）；`assignIPv4` 只 Create / 「已有则跳过」、不删其它 IPv4 → 双地址。PreferVPN 仅 SkipAsSource 掩盖；DNS 快照可把旧 VPN IP 写回。

### 改动

| 项 | 做法 |
|----|------|
| 配 IP | `winnet.ReplaceInterfaceIPv4`：删 ≠ want，前缀不对重建；`assignIPv4` 一律 Replace；日志 `assign_ip replace removed=` / `tun_addrs_before/after` |
| PreferVPN / ICS | 非 vpn 非 137 → Remove；`RemoveICSAddressesKeepVPN` / `CleanupICSResidue` 删全部 ≠ vpn |
| DNS | `RestoreDNS(..., poisonIPs)`：相交则 `dns_restore poisoned→dhcp`；`clearRoutesLockedWithDNSPoison` |
| 队列 | `noteSendQueueFull` 限频 5s + drops（不加大队列当修） |

### 验证

```text
go test ./internal/winnet/... ./internal/tun/... ./internal/netstack/... ./internal/clientapp/... ./internal/transport/... -count=1 -skip TestWaitDNSReady
```

现场：改 VPN IP → `dataplane_clear reason=vpn_ip_change`；`removed` 含旧 IP；`ics_src_diag` 仅新 IP±137；无持续 queue full；Hard/logout 无需杀进程。

---

## 2026-09-01 · 启动耗时二轮（15.6s 后剩余：孤儿/空 DNS/already_paired）

### 现场（一轮后）

双 LAN `policy_elapsed≈15.6s`（已&lt;30s）。仍见：`prepare_orphan≈4.3s`（无孤儿仍 PS）、`dns_snapshot iphlp≈0.84s`（空列表）、`ics_enable≈11s`（可能重复 Enable）。

### 改动

| 项 | 做法 |
|----|------|
| 孤儿 | `HasWintunOrphanAdapters`；无孤儿 `prepare_orphan skipped reason=no_orphan` |
| DNS | 新 TUN 首次快照 `method=skip_empty`（dhcp 还原），免 GAA |
| ICS | 已 public/private 配对则 `ics_enable action=already_paired`，跳过 Disable/Enable |
| 日志 | PreferVPN 只用脚本 `wait_ms`；汇总 `open=session adapter=` |

### 验证

```text
go test ./internal/winnet/... ./internal/netstack/... ./internal/clientapp/... ./internal/tun/... -count=1 -skip TestWaitDNSReady
```

对照：冷启无孤儿应 skipped；`dns_snapshot method=skip_empty`；二次连期望 `already_paired`。

---

## 2026-09-01 · 启动耗时全量治理（DNS/egress/PreferVPN/route_del）

### 现场慢点（治理前）

单 LAN `policy_elapsed≈25s`、双 LAN≈39s；Stop 时 RestoreDNS→关 wintun 间隙≈7s。主因：N×PS 出站、PreferVPN 第二次冷启、DNS 快照 netsh、route DELETE 子进程；ICS COM 本体仍可能数秒。

### 改动

| 项 | 做法 |
|----|------|
| 日志 | `first_policy` / `mode=open adapter=`；`dns_snapshot`/`ics_egress`/`ics_enable`/`ics_prefer_vpn`/`route_del` |
| DNS | `QueryInterfaceDNS`（GAA）优先，失败回退 netsh |
| 路由删 | `DeleteIpForwardEntry2` 优先 |
| 出站 | `CollectEgressSnapshot` 一次采集，多 LAN 内存匹配 |
| PreferVPN | `PSSnippetPreferVPNAfterICS` 嵌 ICS Enable 同 PS；回退 `*Context` |
| ICS | 无残留跳过开头全机预清；Pair 后无残留跳过 DisableAll（打日志钉死） |

### 验证

```text
go test ./internal/winnet/... ./internal/netstack/... ./internal/clientapp/... ./internal/tun/... -count=1 -skip TestWaitDNSReady
```

现场对照：单 LAN `policy_elapsed`<30s；`dns_snapshot method=iphlp`；`ics_prefer_vpn embedded=true`。

### 文档

troubleshooting / deploy / architecture / codebase-guide / internal/README / 记忆 已对齐「根因=sync + 启动耗时周边已砍、COM 残留」。

---

## 2026-09-01 · local_lans 复盘：根因=sync，假因退去、纵深留下

### 复盘结论

| 层级 | 内容 |
|------|------|
| **已确认根因** | 客户端 post-auth `lan_registry_sync` → 服务端 Replace/Prune/Kick 与数据面交织；去掉后现场恢复。`2cdc5e6` 本无此帧 |
| **误判曾堆的** | `defer_dns`、peer_silent 门、hbPause 不 touchHB、PreferVPN#2 —— **不是**本因（已回退/删除） |
| **保留纵深** | Conn 绑定、Done 排空、Decrypt Open 后 commit、`tunUploadReady`、SharedAccess 按需启动、PreferVPN 仅 ICS 内一次、服务端 sync 兼容+勿 Kick via 自己 |

### 本条清理

- 删除死 API：`PeerSilentLongerThan` / `HeartbeatInterval()`；收窄 `hb_pause_test`。
- 文档统一「根因=sync」：troubleshooting / 记忆 / hardening / CODEMAP。

### 验证

```text
go test ./internal/transport/... ./internal/crypto/... ./internal/sessionmgr/... ./internal/clientapp/... ./internal/tunnel/... -count=1 -skip TestWaitDNSReady
```

---

## 2026-09-01 · 去掉 post-auth lan_registry_sync（对齐 2cdc5e6）

### 现场（08:27）

冷启动已对齐：`dns_applied`（ICS 前）→ ICS → 装路由 →「隧道握手成功」；无 decrypt/replay。但仍见 `lan_registry_sync sent`（`skipped=[]`，与握手同 CIDR，零收益却走 prune/Kick）。

### 改动

- 客户端删除 `registrySync*` / `sendLANRegistrySync` / `lan_registry_sync.go`；注册表**仅握手**上报。
- 服务端保留 `applyLANRegistrySync` 兼容旧客户端（限速 + 勿 Kick via 自己）。
- `mergeConnectWarns` 留在 `connect_warn.go`（原随 sync 文件删除而丢失）。

### 验证

```text
go test ./internal/clientapp/... ./internal/tunnel/... ./internal/sessionmgr/... -count=1 -skip TestWaitDNSReady
```

现场：不应再出现 `lan_registry_sync sent`；服务端握手仍有 `lan_registry_reported`。

---

## 2026-09-01 · local_lans 以 2cdc5e6 为可工作基线回退 WT 尾部

### 纠正

用户确认发布提交 **`2cdc5e6`（0.1.3）** 配 `local_lans` **可用**。先前把「`defer_routes` 未同步 `defer_dns`」写成 0.1.3 回归根因 —— **作废**。

| 基线 | 冷启动顺序 |
|------|------------|
| **可工作 `2cdc5e6`** | TUN → `defer_routes` → **DNS（ICS 前）** → ICS → 装路由 → Connected（无 post-auth `lan_registry_sync`） |
| **WT 曾引入（嫌疑）** | `defer_dns` 晚装；Connected **前** `lan_registry_sync`；`peer_silent` 门；hbPause 恢复不 touchHB |

### 本次对齐

| 项 | 动作 |
|----|------|
| DNS | 恢复 ICS 前 `ApplyDNS`（去掉 `defer_dns`） |
| lan_registry_sync | 先延后 Connected；**随后整段删除**（见上条） |
| peer_silent 门 | 去掉 |
| hbPause 恢复 | 恢复 **touchHB**（2cdc5e6） |
| 保留 | TUN Connected 前静默上送；服务端 Conn 绑定 / Done / Decrypt commit；ICS SharedAccess 按需启动 |

### 验证

```text
go test ./internal/transport/... ./internal/clientapp/... ./internal/crypto/... ./internal/tunnel/... ./internal/sessionmgr/... -count=1
```

现场期望：`dns_applied` 在 ICS **之前**；**无** `lan_registry_sync sent`。

---

## 2026-09-01 · ICS 配网末尾 decrypt/replay：健康门 + 去 PreferVPN#2

> **作废（假因实验）**：不 touchHB / peer_silent 门已回退删除。真根因见「复盘」条（post-auth sync）。PreferVPN 仅 ICS 内一次、decrypt 带 counter、勿 Kick via 自己仍保留。

---

## 2026-09-01 · ICS SharedAccess 按需启动（减 soft 重连根因抖）

### 改动

默认不再 `Restart-Service SharedAccess -Force`：已 Running → `ics_sharedaccess action=already_running`；未跑 → `Start-Service`。仅 EnableSharing 两轮都失败后才 Restart 再试（`action=restart`）。

### 验证

```text
go test ./internal/winnet/... ./internal/netstack/... ./internal/clientapp/... -count=1
```

现场：ICS 成功路径应见 `already_running` 或 `start`，不应每次都 `restart`；仍可能因 EnableSharing 本身抖一下（`ics_link_risk`）。

---

## 2026-09-01 · ICS PreferVPN 去冗余 + 厘清 soft 重连根因

### 结论（对照现场 client 日志）

| 层次 | 是什么 | 不是什么 |
|------|--------|----------|
| **断链根因** | 家庭版 ICS：`Restart-Service SharedAccess` + `EnableSharing(public=WLAN)` 抖底层；隧道走同 WLAN 则 ICS 窗口易 soft 重连。未配 `local_lans` 不进 ICS → 无此问题 | PreferVPN / SkipAsSource / 强制 /32 |
| **错源问题** | ICS 挂 137 后本机可能用 137 作源 → ping AllowedIPs 不通；`PreferVPN` 设 137 skip / VPN 可源 | soft 重连本身 |
| **replay 症状** | 软重连后同钥新窗口竞态；或 WT 配网末 sync/Connected 交织 | 「0.1.3 defer_routes 本身必坏」（**已否定**：`2cdc5e6` 可用） |

执行顺序：`tun → defer_routes → DNS → ICS → PreferVPN#1 → 装路由 → Connected`（新客户端无 post-auth sync）。

### 清理

去掉 `via_exit` / 装路由后再 PreferVPN。ICS 启用后打 `ics_link_risk`。

### 验证

```text
go test ./internal/clientapp/... ./internal/netstack/... -count=1
```

---

### 目标

修复配 `local_lans`→ICS 软重连时服务端 `decrypt failed` + ascending `replay attack detected`；补齐注册表收缩后的活路由剪枝与限速；抽取 `PollUntil`、删死代码、文档对齐。User DPAPI **未做**（仍听安排）。

### 根因（现场已确认）

post-auth `lan_registry_sync`（已去）。纵深另见：同钥软重连旧 Conn 迟到包 / Decrypt 先 Mark 烧号。

### 代码

| 项 | 内容 |
|----|------|
| 入站 | `HandleInbound(userID, conn, …)` 须 `ps.Conn == conn` |
| 顶替 | `drainOldConn`：`SetOnData(nil)` + Close + 等 `Done`（超时 `old_conn_drain_timeout`） |
| Crypto | `Decrypt`：Open 失败回滚 Filter 快照，成功才 commit |
| 客户端 | `tunUploadReady`：非 `StateConnected` 不上送（`tun_upload_quiesced`） |
| 注册表 | 新客户端仅握手；旧 sync 兼容：`PruneViaRoutesAfterRegistry` + 勿 Kick via 自己；限速 |
| 抽取 | `safeutil.PollUntil` → DNS settle / GUI 单实例 / SCM 停等待 |
| 死代码 | 删 `ProbeICSLocalLANsHint`、`ics_hint.go`；Darwin/Linux `platform.Command` |

### 验证

```text
go test ./internal/crypto/... ./internal/sessionmgr/... ./internal/tunnel/... ./internal/clientapp/... ./internal/safeutil/... ./internal/netstack/... ./internal/netutil/... ./internal/clientgui/... -count=1
```

### 文档

troubleshooting（replay / registry 行）、security-hardening §4.3 + 重编号 §10 DPAPI、architecture FAQ、codebase-guide 调用链、internal/README、记忆.md。

---

## 2026-09-01 · local_lans 格式硬校验 + ICS 出站三档

### 目标

非法 `local_lans` 登录前挡过；ICS 出站保持简单：配置网卡 → 本机同网段/专用路由 → 默认网关。客户端 via 可读 `windows.outbound_interface`（仅 ICS）。

### 代码

| 项 | 内容 |
|----|------|
| 校验 | `netutil.ValidateLocalLANsList`；`ClientConfig.Validate` 硬挡并规范化写回 |
| 出站 | `findOutboundInterface`；默认网关打 `lan_egress default_route` |
| via | `via_exit` 传入 `Windows.OutboundInterface` → `OutboundIf` |

### 验证

```text
go test ./internal/netutil/... ./internal/config/... ./internal/netstack/... ./internal/clientapp/... -count=1
```

---

## 2026-09-01 · ICS 提示改连接后 + 注册表纠正

### 目标

登录页不再预检 ICS（此时不知 WinNAT/ICS）。确定走 ICS 后再提示；异网卡跳过的网段不得留在服务端注册表误导托管路由。

### 代码

| 项 | 内容 |
|----|------|
| GUI | 去掉登录页 `icsHintLbl` / Probe |
| Setup | `NATSetupOutcome` + Stack `UsedICS`/`ICSPlan`；`via_exit_setup` 打 `active_lans`/`skipped_lans` |
| 提示 | `ics_multi_nic` + `LastError`（主窗状态，不挡连接） |
| 注册表 | ICS 后 Handshake `type=lan_registry` → `ReplaceClientLANRegistry` + `ReloadExitLANs` |

### 验证

```text
go test ./internal/netstack/... ./internal/clientapp/... ./internal/clientgui/... ./internal/tunnel/... ./internal/sessionmgr/... -count=1
```

---

## 2026-08-31 · 多网段 / 多网卡 ICS：首网卡生效 + 提示清楚

### 目标

Windows ICS 只能一对共享；修复旧版 per-LAN 反复 Enable 互相覆盖。同首出站网卡的多段一并生效，异网卡跳过并提示（不挡登录）。WinNAT 仍一条 VPN 子网覆盖多目的 LAN。

### 代码

| 项 | 内容 |
|----|------|
| 决策 | `netstack/ics_plan.go`：`PlanICSByOutbound*`、`FormatICSLocalLANsHint`（日志=GUI 同文案） |
| Setup | `setupNATForLANs` → WinNAT 一次或 `setupICSForLANs` 整表一次 Enable；键 `ics_multi_nic` / `ics_enable once` |
| 提示 | **已修订**：见上条「连接后提示 + lan_registry 纠正」（不再登录预检） |
| 测 | 同 NIC / 异 NIC / Preferred outbound / 文案快照 |

### 验证

```text
go test ./internal/netstack/... ./internal/clientapp/... ./internal/clientgui/... -count=1
```

### 文档

troubleshooting / deploy / traffic-routing / architecture 写清 ICS 单出口语义。

---

## 2026-08-31 · 架构解耦第 25 轮（抽取 · 可取消 · 安全 · 文档）

### 目标

高内聚低耦合收口：补全 Stop/HardRestart 可取消链路、WinNAT PS 转义、公共 abort/PS/GUI Stop 辅助、清死代码、补测、文档对齐。明文密码写文档本轮不改。

### 代码

| Phase | 内容 |
|-------|------|
| 1 | `nat_windows.go`：`New-NetNat`/`Remove-NetNat` 一律 `EscapeSingleQuoted`；`formatNewNetNatPS` 单测 |
| 2 | `safeutil.IsCanceled` / `Check`；删死导出 `GoSafeCtx`；`applyPolicy`/`Setup`/`via_exit` 去重 |
| 3 | `RunPSBestEffortContext`；ICS 探测/Disable/Cleanup 可 Kill；删死别名 `RunPS`；日志 `ps_kill`/`ics_abort` |
| 4 | `Stack.Setup(ctx)` / `Teardown(ctx)` 取代 `Config.AbortCtx`；Teardown 正常路径用 `Background` 以免跳过 ICS 清理 |
| 5 | `WaitDNSReadyAbort`（settle 中可 abort）；HardRestart 中段 abort 测例；`setupViaExitLocked` 取消测例 |
| 6 | `stopEnginesSerial`；`engineOpQueue` 纯状态机 + `decideHardRestartFinish`；login_fail busy→pending logout |
| 7 | 删 `ShutdownWindows`/`winnet.Shutdown` 空钩；`DisableICSSessionContext`（Pair→残留→All） |

### 验证

```text
go test ./... -count=1   # 通过
```

关键单测：`nat_escape_windows_test`、`safeutil/context_test`、`ps_context_test`（BestEffort/DisableAll 取消）、`setup_abort_test`、`WaitDNSReadyAbortMidSettle`、`HardRestartAbortDuringDNS`、`engine_op_queue_test`、`FinishLoginFailureBusyQueuesLogout`。

### 文档

architecture CODEMAP / internal README（FAQ 短表）/ codebase-guide / troubleshooting / security-hardening / 记忆 — 对齐第 25 轮；去掉 AbortCtx、ShutdownWindows、不可中断 WaitDNSReady 等过时表述。

---

## 2026-08-31 · ICS 取消误判 forward_only 成功（现场 22:57）

### 日志问题

ICS 中点重连已见 `ICS 启用已取消`，但随后：
`netstack NAT 失败: context canceled`（ERROR）→ `forward_only` 吞成「无 SNAT 成功」→ `via_exit_setup ok snat=false` → 误装路由 → 再 `stop_during_policy`。

### 修复

- `Stack.Setup`：abort/`context.Canceled` **立即返回 error**，禁止走 forward_only 成功分支。
- `via_exit_setup`：取消用 Info，不打 ERROR 堆栈。
- 单测：`TestSetupAbortNotForwardOnlySuccess`。

### 期望日志

`ICS 启用已取消` → `netstack setup aborted` / `via_exit_setup aborted` → `policy_apply aborted reason=engine_stop`（勿再 `via_exit_setup ok snat=false`）。

---

## 2026-08-31 · 现场日志确认：HardRestart 竞态已修复 + ICS 可取消

### 现场日志（22:39）对比旧日志（22:30）

| 项 | 修复前 | 修复后 |
|----|--------|--------|
| HardRestart 耗时 | ~42s（空跑 ICS + soft 重连 + teardown） | **~3.0s** |
| policy | 跑完 via/ICS | `policy_apply aborted stage=before_via` |
| soft 重连 | `dataplane_keep` / `将重连` | **无** |
| Stop | 后 cancel | `engine_stop begin` 先于 abort |

### 残留补强

ICS **已开始**时旧版仍须等 PowerShell 结束。现：`Stack.Setup(ctx)` + `RunPSOneShotContext`（Kill powershell）；ICS 后 1.5s 等待可打断。第 25 轮起探测/Disable 亦 Context 可 Kill。

### 验证

- 现场日志如上；`go test ./internal/winnet/ ./internal/netstack/ ./internal/clientapp/ ./internal/clientgui/ -count=1`
- `TestRunPSOneShotContextCanceled` / `CancelDuringSleep`

---

## 2026-08-31 · 日志实证：连接中 HardRestart 与 policy/ICS 竞态

### 日志时间线（摘要）

`gui_reconnect`/`hard_restart begin` 发生在首次 `policy_apply` 的 DNS 之后、via/ICS 完成之前；随后仍跑完 ~20s ICS → `session_abandoned disconnected_during_policy` → `dataplane_keep`/`将重连` → 再 `dataplane_clear reason=stop`（HardRestart 总耗时 ~42s）。

### 根因（已用代码路径验证）

1. `Engine.Stop` 旧顺序：先 `rc.Stop`（关 Conn 并等 onConnect）**后** `cancel` → `applyPolicy` 看不到取消，ICS 空跑。
2. policy 结束后因 `activeConn` 已清走 soft 重连分支（`dataplane_keep`），与 HardRestart 叠在一起。
3. reconnect loop 在 `Done` 后仍打「将重连」再被 stop 打断。

### 修复

- `Stop`：先 `stopping`+`cancel(runCtx)`，再清 activeConn、`rc.Stop`、清数据面；日志 `engine_stop begin/done`。
- `applyPolicy(ctx)`：`before_dns`/`before_via` 检查 cancel；abort 不走 `dataplaneFailed`。
- `session_abandoned`：`isStopping` → `stop_during_policy`，禁止 soft 重连。
- reconnect：`Done` 后若已 stop 直接 return，不打「将重连」。

### 验证

- `go test ./internal/clientapp/ ./internal/transport/ ./internal/clientgui/ -count=1`
- 单测：`TestApplyPolicyAbortedAtStart`、`TestStopCancelsRunContext`、`TestIsStoppingSetByStop`

---

## 2026-08-31 · GUI 连接中重连/退出抢占与状态机收口

### 问题

连接/HardRestart 进行中再点「重新连接」：旧清理与新拨号观感并行、二次重连无效；busy 时「退出登录/退出」被直接拒绝；HardRestart 新引擎未挂 `OnDataplaneFailed`；`eng==nil` 时重连静默 return。

### 修复

- `opGen` + `pendingIntent`（logout/quit/reconnect）：busy 时排队；reconnect bump gen。
- `HardRestart(..., abort)`：Stop/DNS/Start 间隙可中止 → `ErrHardRestartAborted`。
- `finishHardRestartUI`：supersede/abort/失败/成功后 pending 统一经 orphan Stop，禁止未挂载 eng 泄漏。
- 登录与 HardRestart 共用 `attachDataplaneHook`；重连允许 `eng==nil`（失败清理后）。

### 验证

- `go test ./internal/clientgui/ ./internal/clientapp/ -count=1`
- 日志：`gui_reconnect deferred` / `gui_logout deferred` / `gui_hard_restart aborted|superseded|mounted`

---

## 2026-08-31 · GUI 托盘/登录状态机修复（审查收口）

### 问题

1. 退出登录后主界面已回登录页，托盘 tip 仍「正在断开…」。
2. 登录连不上服务端时：登录钮/文案卡「正在连接」，托盘已红并提示失败。

### 根因与修复

- tip：`engOpBusy` 时强制断开文案；登出回调先 `applyTray` 后 `endEngineOp` 且 end 不刷 tip → tip 锁死。改为先 end 再回登录；`endEngineOp` 必 `refreshTrayTooltip`。
- 登录失败：`finishLoginFailure` 立刻红字+`trayStickyErr`；再 `beginEngineOp` 串行 Stop（禁未清完就 NewEngine）；busy 时 sticky **优先于**「正在断开」。
- 审查补洞：HardRestart Start 失败须 Stop 返回的 eng（禁 setEngine 僵尸）；SaveClient 失败仍进主窗；数据面失败经 `pendingLogoutMsg` 回登录保留原因；`applyTray` 无桌面托盘也更新 `trayKind`；超时文案勿盖真实 LastError。

### 验证

- `go test ./internal/clientgui/ ./internal/clientapp/ -count=1`
- 关键日志：`gui_login_fail` / `gui_login_fail cleanup_done` / `gui_hard_restart_fail cleanup_done`

---

## 2026-08-31 · 文档治理（对齐 0.1.3 / 第 24 轮）

### 目标

修正活文档中与代码不符的过时表述、重复 FAQ、页脚漂移；**不改运行时行为**。

### 改动摘要

- 根 README CSP：`style-src 'self'`（删「样式仍可内联」）。
- architecture FAQ：ICS/Shutdown/Configure 改为 `netstack` 门面；合并 listen_tun / TUN 预热重复行；修复表内空行；历史 CSP tip 标注已被第 22 轮取代；记住密码债单链 hardening §8。
- codebase-guide 调用链与 vpnaccount/`winnet_facade`；记忆压缩「当前阶段」并对齐 VERSION **0.1.3**；internal/README 去掉「第二十二轮」标题并补 warmup/hard_restart。
- docs/README 必读顺序：architecture → codebase-guide；deploy/hardening/troubleshooting 页脚统一。

### 验证

- 人工对照 import 边界与 FAQ 符号；本条仅文档。

---

## 2026-08-31 · 架构解耦第 24 轮（高内聚低耦合收口）

### 目标

收紧分层依赖、补齐 TUN/PS/静态路径安全校验、peer 写路径进 vpnaccount、统一领域错误；保持功能兼容。**跳过** GUI 记住密码 User DPAPI（仍听安排）。

### 改动

- **边界**：`clientapp.WarmupTun` / `SaveServiceCredentials`；`netstack` Windows 门面（`winnet_facade.go`）；切断 `clientgui→tun|credentials`、`clientapp→winnet`。
- **安全**：`config.ValidateTunName`；`winnet.EscapeRegex`；`handleStatic` path.Clean；去掉 `PreferServerCipherSuites`。
- **领域**：`vpnaccount/peer_write.go`；`writeDomainError` 覆盖 vpnaccount/auth/probedefense 哨兵。
- **可维护**：`WaitDNSReady`→`RetryN`；三种 escape 方言交叉注释。
- **文档**：architecture / internal/README / codebase-guide / security-hardening / troubleshooting / deploy / 记忆。

### 验证

- `go test` 相关包与 `./...`：除 `singleinstance`（本机已有客户端占用锁，环境干扰）外全部通过
- `.\scripts\build-local.ps1` 通过（server/client/gui）

### 未做（听安排）

- GUI 记住密码脱离 yaml 明文 / CurrentUser DPAPI。

---

## 2026-08-31 · 架构解耦第 23 轮（高内聚低耦合收口）

### 目标

收敛 PS/ICS/Wintun 重复模板、把手动重连契约收口到 `clientapp`、路由部分失败可观测；保持功能兼容。

### 改动

- **winnet/ps_snippets.go**：`PSSnippetAssignAdapterIf` / ICS Disable / `BuildPrepareWintunOrphanScript`；address/resolver/ics/tun/netstack 改用模板。
- **clientapp**：`WaitDNSReady` + `HardRestart`；GUI 仅 `fyne.Do` 挂载；禁止第三套重连编排。
- **路由**：`route_install ok/fail`；期望非空且零成功 → applyPolicy 硬失败；部分失败 → `LastError`；Del 失败 Warn；Stop 清 `sessionPriv` 单测。
- **TrimLower** 收口 sku/tls_policy；`Shutdown` 文档化为空挂点。
- **未做（听安排）**：GUI 记住密码脱离 yaml 明文 / User DPAPI。

### 验证

- `go test ./internal/winnet/... ./internal/tun/... ./internal/netstack/... ./internal/clientapp/... ./internal/clientgui/... ./internal/security/...` 通过
- `go test ./...`：除 `singleinstance`（本机已有客户端占用锁，环境干扰）外全部通过
- `.\scripts\build-local.ps1` 通过（server/client/gui）

---

## 2026-08-31 · 退出防连点 + ICS 靶向关共享

### 问题

- 退出 ~10s，其中 ~8s 全机 `DisableAllICS`；合理但偏慢。
- `beginEngineOp` 已防重入，按钮未 Disable，用户以为还能点。

### 修复

- 主窗/登录操作钮：`setEngineOpBusyUI` 随 begin/end 灰掉/恢复。
- `RememberICSPair` + `DisableICSPair`；残留再 `DisableAllICS`。

### 验证

- `go test ./internal/clientgui/ ./internal/winnet/ ./internal/netstack/`

---

## 2026-08-31 · 删除 ps_resident + PreferVPN 一进程收口

### 问题

- ICS 后 `PreferVPNSourceWithICS` 仍走常驻 → IEX「意外的 }」熔断再回退一进程。
- `ps_resident` 设计目标（加速 ICS/WinNAT）已被 OneShot/IP Helper/`sku_home` 替代，开 true 只有攻击面与噪声。

### 修复

- 删除 `ps_resident_*` 实现与配置字段；`RunPS`≡一进程；address/resolver/wintun 全部 `RunPSOneShot`。
- 旧 yaml 含 `ps_resident` 键：Unmarshal 忽略，无影响。

### 验证

- `go test ./internal/winnet/ ./internal/netstack/ ./internal/tun/ ./internal/config/ ./internal/clientapp/ ./internal/clientgui/`

---

## 2026-08-31 · Tip 预算 63 + WinNAT/ICS OneShot + 家庭版快路径

### 问题

- tip「连接自: 20」：按 127 拼装，fyne systray 未 SETVERSION → Windows 只显示 ~64，日期被砍成残片。
- `ps_resident` handshake ok 后空等 20s：首条严格 RunPS 是 Get-NetNat（常驻 + 脚本 `exit`），与常驻主机不兼容；家庭版仍白试 WinNAT。

### 修复

- tip：预算 **63**；行序品牌→IP→连接自（短日期）→主机；整行原子。
- `RunPSOneShot`；NetNat/ICS 一律一进程；去掉 `exit`；exchange 日志 `op_hint`。
- `IsWindowsHomeSKU` → `WinNAT skip reason=sku_home` 直进 ICS。

### 验证

- `go test ./internal/clientgui/ ./internal/netstack/ ./internal/winnet/`

---

## 2026-08-31 · 托盘 tip/状态机 + ps_resident BestEffort 余量

### 问题

- tip 末行只剩「分配」：Windows 127 UTF-16 盲截；长 hostname 砍掉 IP。
- 鉴权后 GUI Connected，但 Engine 等 applyPolicy 才写 vpnIP → tip 长时间无 IP/像「正在连接」。
- 登出/退出 Stop 期间 tip 仍像连接中。
- `ps_resident` handshake ok 后 ICS/Remove-NetNat 空等 90s。

### 修复

- **tip**：行序品牌→IP→主机→时间；按预算 ellipsize 主机。
- **状态机**：`trayKindDisconnecting`；logout/quit/reconnect Stop 立刻刷「正在断开」；engOpBusy 覆盖。
- **Engine**：鉴权后早写 vpnIP 等；StateConnected/connectedAt 仍等数据面；Connecting+IP →「正在配置网络…」。
- **PS**：`RunPSBestEffort` 只走一进程；resident 交换超时 20s。

### 验证

- `go test ./internal/clientgui/ ./internal/clientapp/ ./internal/winnet/`

---

## 2026-08-31 · 托盘悬停气泡 + ps_resident 熔断重写

### 问题

- 托盘悬停无 OpenVPN 式「已连接至 / 连接自 / 分配 IP」。
- `ps_resident: true`：隐藏窗口下旧 Console 管道主机不回 PSOK → 60s timeout → 反复启停卡住。

### 修复

- **托盘**：`formatTrayTooltip` + `systray.SetTooltip`；`Engine.ConnectedSince()`。
- **ps_resident**：`-EncodedCommand` 主机、`PSREADY` 握手、行通道同步、失败一次熔断回退一进程。

### 验证

- `go test` clientgui / clientapp / winnet
- 手工：悬停气泡；`ps_resident: true` 见 handshake ok 或一次 `disabled reason=` 后无风暴

---

## 2026-08-31 · Fyne.Do 真正生效 + 自动连接重叠 + 路由/DNS/WinNAT 余量

### 问题（家里机完整日志）

- 仍打 Fyne「not migrated」：`FyneApp.toml` 未被 `go build` 加载。
- UI 空等～5s：`tun_warmup wait done` 后才 `gui_auto_connect`（Wait 串行）。
- 预热 handoff / `assign_ip method=iphlp` 已好；`route_add method=route_exe`、DNS netsh、家庭版每次先试 WinNAT 仍慢。

### 修复

- **Fyne**：`Invoke-GoBuildGui -tags migrated_fynedo`；`fyne_meta.go` SetMetadata；TOML ID=`com.haovpn.client`。
- **auto_connect**：删 `waitTunWarmup`；`warmup_overlap=true` 立即拨号；后台预热保留。
- **路由**：`MibIpForwardRow2` + `ConvertInterfaceIndexToLuid`；fail 打 Warn。
- **DNS**：`SetInterfaceDnsSettings`（GUID 调用约定按架构）；失败回退 netsh。
- **WinNAT**：会话缓存不可用后跳过重复 New-NetNat，直接 ICS。

### 验证

- `go test` clientgui / winnet / netstack / tun
- 重建 GUI 后：无 Fyne migrated 警告；先 `gui_auto_connect begin warmup_overlap=true`；期望 `route_add/dns_set method=iphlp`

---

## 2026-08-31 · Fyne.Do + Windows IP Helper + 可选常驻 PS

### 问题

- 公司机热路径：`assign_ip_probe≈5s` / `wait≈14s` / `HasICSResidue cache≈9s`——根因是 Go `net.InterfaceByIndex`+`Addrs` 每次全表 GAA，非 O(1)。
- Fyne「not migrated to fyne.Do」；手动重连若把 DNS settle 塞进 UI 会卡死。
- netsh/route/dns 子进程税仍在；常驻 PS 只能削 powershell 冷启，不能修 GAA。

### 修复

- **Fyne**：`FyneApp.toml` `fyneDo=true`；reconnect settle 在后台；`stopEngineAsync`/`Start` 经 `fyne.Do`；`clientgui/doc.go` 线程规则。
- **config**：`windows.use_ip_helper`（默认 true）、`ps_resident`（默认 false）；`NewEngine`→`winnet.Configure`；`Stop`→`Shutdown`。
- **读**：`GetUnicastIpAddressTable` → `InterfaceHasIPv4` / ICS by_index；`method=iphlp|net_fallback`。
- **wait**：真正尊重 deadline；配置已提交失败仅 Warn。
- **写**：配 IP / 分流路由优先 IP Helper，失败回退 netsh/route；`method=` 日志；DNS 仍 netsh 并打 method。
- **ps_resident**：stdin 协议 + Job Object；默认关；失败回退一进程一脚本。

### 验证

- `go test` winnet / tun / netstack / clientapp / clientgui / config + `./...`
- 公司机期望：probe/ICS `method=iphlp` 亚秒；`assign_ip_wait` 不再十余秒

### 文档

- troubleshooting / architecture / codebase-guide / internal/README / security-hardening / 记忆 / winnet·clientgui doc

---

## 2026-08-31 · 配 IP 探测加速（禁 Interfaces 全表）

### 问题

- 预热/`from_warmup`/`已复用` 已生效，但仍见 `assign_ip≈42s`：session→netsh 空白 ~32s + wait ~10s。
- 根因：`interfaceHasIPv4ByIndex` 每次 `net.Interfaces()` 全表；`InterfaceHasIPv4` 在 ByIndex 未命中 IP 时还 fallback ByName。

### 修复

- ByIndex 改 `net.InterfaceByIndex`；有 ifIndex 不再 ByName。
- 非 reused 跳过配前 probe；reused 打 `assign_ip_probe elapsed`；wait 轮询 Debug。
- HasICSResidue cache：`by_index`/`addrs` Debug 微埋点。

### 验证

- `go test` winnet / tun
- 公司机期望：`assign_ip_probe` 亚秒；assign 总时接近 netsh+短 wait

---

## 2026-08-31 · 预热 Close 卸适配器 + 心跳暂停 + 手动重连 DNS

### 问题

- 预热 `CreateAdapter` 后 `Adapter.Close` 卸掉系统适配器 → 登录 `Element not found` 再 Create；仍 `reused=false`。
- `assign_ip` 约 66s 触发 `heartbeat timeout` → `session_abandoned`（随后 noop 重连能好）。
- 手动重连：Stop 后 `lookup i/o timeout` + `SetFailFast(true)` 首败停 loop，再点一次才连上。

### 修复

- **预热**：`warmedAdapter` 持有句柄禁止 Close；Open `take` → `reuse from_warmup`；auto_connect Wait 预热。
- **心跳**：`Conn.SetHeartbeatTimeoutPaused` 包住 `applyPolicy`。
- **配 IP**：`assign_ip_netsh|ps|wait` 子阶段；netsh 失败先探测地址再决定是否 PS。
- **ICS**：有 LUID 缓存只查 ifIndex；`stage=cache|…`。
- **手动重连**：去掉 FailFast；`reconnect_dns_settle` 短等 LookupHost；`RestoreDNS elapsed`。

### 验证

- `go test` tun / transport / clientapp / clientgui / winnet
- 公司机：`tun_warmup held=true` → `from_warmup`；手动重连无需再点

---

## 2026-08-31 · 手动重连死循环修复 + 公司机慢登录加速

### 问题

- 手动「重新连接」后死循环：`lookup … i/o timeout`、握手 `invalid character '\x00'`；退出登录再连却正常。
- 公司机首次登录 ~2min：冷 `CreateAdapter`（~70s）+ PowerShell `HasICSResidue`（~33s）+ route/netsh 子进程。

### 修复

- **R1/R2**：`ReconnectClient.Stop` 等 loop、Dial 后 stop 门闩、可中断 Sleep；`Engine.Stop` 等 rc 后再 `rt.close`。
- **R3**：`Conn.SetOnHandshake`；Data 不再当握手 JSON。
- **R4**：`clearRoutesLocked` 先 `RestoreDNS`（Warn）再删路由。
- **R5**：手动重连 `SetFailFast(true)`（成功后 `markAuthOK` 关闭）。
- **P0/P3**：`tun_open` / `policy_apply stage=` / `route_add` / `dns_apply` / `HasICSResidue elapsed` 分段埋点。
- **P1**：`HasICSResidue` 优先 Go/net+LUID；仅无网卡才 `ps_fallback`。
- **P2**：GUI 启动 `tun.WarmupAdapter`；登录走「已复用」；Open/Create 串行化。

### 验证

- `go test` transport / tunnel / clientapp / winnet / netstack / tun / clientgui
- `go test ./...`（singleinstance 占锁属环境）
- 手工：手动重连无死循环；公司机看 `method=native` 与登录 `已复用`

---

## 2026-08-31 · 架构解耦第二十二轮（抽取 · PS 收口 · CSP · 文档）

### 问题

- `api`/`clientgui` 各有 `ToLower+TrimSpace`；GUI 子网 hint 与 `InferGatewayFromVPNIP` 同假设却散落。
- `netstack` raw powershell 缺 Bypass，与 `winnet.RunPS` 双轨；ICS/NAT `_ = Run()` 静默失败。
- `ParseDNSShowOutput` 在 netstack；`NormalizeKillPrefixes` 薄封装。
- `winnet/netsh_windows.go`、`route_windows.go`、`killswitch_windows.go` 过胖。
- CSP `style-src 'unsafe-inline'` 文档债；FAQ 对 ICS API 文件位置漂移。

### 修复

- **叶子**：`netutil.TrimLower`、`InferVPNSubnetHint`；删私有副本与 `NormalizeKillPrefixes`。
- **PS**：`RunPSBestEffort`（失败 Warn）；netstack/winnet 一律 Bypass；无 raw powershell。
- **DNS**：`ParseDNSShowOutput` → `winnet/dns_parse.go`。
- **拆分**：winnet → `ps_`/`address_`/`dns_netsh_`/`ics_windows.go`；netstack → `forward_`/`nat_`/`ics_nat_`/`route_ops_` + `killswitch_wfp_*.go`。
- **CSP**：外置 style；`style-src 'self'`；`HaoVPN.setVisible`/`setOverlayOpen`；测试钉死无 `style=`。
- **依赖规则 26～27**；architecture / internal/README / codebase-guide / hardening / troubleshooting / 记忆。

### 验证

- 分步：`go test` netutil / winnet / netstack / api / clientgui / security
- `go test ./...`（`singleinstance` 本机客户端锁占用失败属环境，与既往一致）
- `.\scripts\build-local.ps1` 通过

---

## 2026-08-31 · 空 local_lans 智能跳过 ICS 清理

### 问题

- 公司客户端未配 `local_lans` 时，每次登录仍无条件 `DisableAllICS`（PowerShell COM，常 ~10–20s）。
- via 开→关：`Teardown` 已关 ICS，空路径再 `DisableAllICS` 二次付费。
- `via_exit` 在 `Setup` 后再调 `PreferVPNSourceWithICS`（ICS 路径 Setup 内已做）。

### 修复

- `winnet.HasICSResidue`：便宜探测 TUN 上 `192.168.137.*`。
- `winnet.CleanupICSResidue`：有残留时一次 PS（Disable + 清 137）。
- `cleanupTUNAfterViaDisabled(hadVia)`：无残留跳过；hadVia 只清地址；否则 CleanupICSResidue。
- 删除 Setup 后重复 PreferVPNSource；unchanged/no-residue 日志改 Debug。
- **不做**：YAML 开关、收窄 Disable 为仅本适配器、改 Setup sleep/多次 Disable、服务端 NAT ICS。

### 验证

- `go test ./internal/winnet/... ./internal/clientapp/... ./internal/netstack/...`
- `go test ./...`（singleinstance 本机占锁属环境）

---

## 2026-08-31 · 架构解耦第二十一轮（薄封装清零 + GoSafe + 文档对齐）

### 问题

- 违反依赖规则 9 的薄 re-export：`clientapp.IsIPBannedDialError` / `IsAccountAlreadyOnline`、`probedefense`/`tunnel` 的 `ErrSourceDenied`、`tunnel.CheckTunnelSourceIP`。
- 生产路径仍有裸 `go`（transport Conn/Listen、sessionmgr 关旧连接、logstore writer、singleinstance accept）。
- dialerr banner Line/Bytes 双份前缀；哨兵 Error() 中英文混用；autherr Classify 与 Is* 子串双份维护。
- `doHandshake` ~165 行混阶段，阅读成本高；过时注释仍写 `classifyHandshakeReject`。

### 修复

- **删薄封装**：调用方直接 `autherr`/`dialerr`/`netutil`；源 IP 测试迁入 `netutil`。
- **GoSafe 收口**：上述生产路径全部 `safeutil.GoSafe`。
- **叶子去重**：`matchRejectBannerPrefix`；哨兵中文 Error()；autherr 共用子串表；`ExpBackoff` 测试对齐。
- **握手文件簇**：`server_handshake_auth.go`（1～3）+ `server_handshake_session.go`（4～7）；`doHandshake` 仅编排。
- **依赖规则 24～25**：GoSafe 强制；源 IP 禁止薄包装。

### 文档

- architecture CODEMAP/FAQ/第二十一轮要点；internal/README；codebase-guide；security-hardening / troubleshooting / 记忆。

### 验证

- 分步：`go test` 覆盖 dialerr/autherr/netutil/safeutil/transport/tunnel/probedefense/clientapp/sessionmgr/logstore
- `go test ./...`（`singleinstance` 本机客户端锁占用失败属环境）
- `.\scripts\build-local.ps1`

---

## 2026-08-31 · 架构解耦第二十轮（公共抽取 + 缺陷清零）

### 问题

- 双 `ErrSourceDenied`（autherr 中文 vs transport 英文）；`autherr → transport` 反向依赖。
- 握手失败只传 `error` 字符串，客户端 `fmt.Errorf("%s")` 丢掉哨兵，靠中文子串兜底。
- `netutil.CheckSourceIPAllowed` 注释声称可 `errors.Is`，实际不 wrap；`tunnel` 重复实现。
- `tunnel` 硬 import `probedefense`（违反 ProbeRecorder 窄接口）。
- `ReconnectClient` 裸 `go`、100ms 轮询、双 Start 无防护；TLS bad-record 子串两处重复。

### 修复

- **叶子包 `internal/dialerr`**：banner 常量、拨号哨兵、`IsFatalDialError`、`ClassifyTLSHandshakeErr`、`IsTLSBadRecordMsg`。
- **`autherr`**：只依赖 `auth`+`dialerr`；`HandshakeCode`/`FromHandshakeCode`；`ErrSourceDenied` 与 dialerr 同一枚。
- **握手 `code`**：`EncodeHandshakeErrCode`；客户端优先还原哨兵；`reportFirstFailure(error)` 保留 wrap。
- **源白名单**：`CheckSourceIPAllowed` wrap `dialerr.ErrSourceDenied`；`CheckTunnelSourceIP` 一行委托。
- **`ProbeRecorder.OnHandshakeReject`**：切断 `tunnel→probedefense`。
- **`safeutil.ExpBackoff`**；`ReconnectClient`：`GoSafe`、CAS 防双 Start、`Conn.Done()` 等待。
- **clientapp**：拨号 UX 只依赖 autherr+dialerr；`doc.go` 文件簇边界。

### 文档

- architecture CODEMAP/FAQ/依赖规则 21–23；internal/README 胖包文件簇；codebase-guide 修正 serverapp/api 表述；security-hardening / troubleshooting / 记忆。

### 验证

- `go test ./internal/dialerr/... ./internal/autherr/... ./internal/netutil/... ./internal/safeutil/... ./internal/transport/... ./internal/tunnel/... ./internal/probedefense/... ./internal/clientapp/...`
- `go test ./...`（`singleinstance` 本机客户端锁占用失败，属环境，非回归）
- `.\scripts\build-local.ps1` 通过

---

## 2026-08-31 · 封禁 banner 链路加固（误判 / 延迟 / 状态机）

### 问题

- 客户端 TLS 前 peek 过长时，**每次成功连接**都会空等（服务端在 ClientHello 前不发字节）。
- `EOF` / TLS「first record does not look like…」被直接当成 `ErrIPBanned`，连错端口也会提示「已封禁」。
- `RecordBanHit` 同步写库曾阻塞 banner 写出；源白名单拒绝无 banner。
- 已上线后遇封禁：`ReconnectClient` 停 loop，但 Engine 仍停在「重连中」。

### 修复

- **peek ≈ 250ms**；成功路径 `TestTLSRoundTrip` 断言 Dial &lt; 1.5s。
- 哨兵分离：`ErrIPBanned` / `ErrSourceDenied` / `ErrPlaintextBeforeTLS` / `ErrClosedBeforeTLS`。
- 服务端 `WriteRejectBanner`；`CheckAccept` 记库经 `safeutil.GoSafe`；源拒绝发 `HAOVPN:SOURCE_DENIED`。
- `ReconnectClient` + `Engine.onDialError`：致命拨号错误停重连并置 `StateIdle` + `LastError`。
- `FormatDialError` 中文双因提示；单测覆盖 banner / 晚到明文 / 空关闭。

### 验证

- `go test ./internal/transport/... ./internal/clientapp/... ./internal/probedefense/... ./internal/autherr/...`

---

## 2026-08-31 · 架构解耦第十九轮（审计整改 + 公共抽取续）

### 安全与正确性

- **TLS Accept 探针**：`transport/server.go` 握手失败调用 `Probe.OnTransportReadError`；`server_probe_test.go`。
- **手动封禁**：`probedefense/manual_ban.go`（`ManualBanStore` 始终检查豁免）；无 Guard 时 API 503。
- **豁免 YAML 导入**：非法 CIDR 跳过 + WARN。
- **`api.listen_tun`**（默认 true）：可关闭 TUN 网关管理口绑定；`audit/tun_listen.go`。
- **HSTS / Secure Cookie**：`security/RequestIsHTTPS`；可信反代 `X-Forwarded-Proto: https`。
- **GUI 记住密码**：勾选时显示明文存储警告（行为不变）。

### 公共抽取

- **`internal/autherr`**：`Classify` / `IsIPBanned` / `IsFatalAuth`；probedefense/clientapp 共用。
- **netutil**：`MergeDedupTrimNonEmpty`、`CheckSourceIPAllowed`、`source_ip.go`。
- **paginate**：`ParseOnlyEnabled`。
- **api**：`validateWebSession`；`writeDomainError`；`persist/peer_access_errors.go` 哨兵。
- **health**：`CheckDirWritable` 替代重复 `EnsureParentDir`。

### 文档

- 更新 architecture、internal/README、codebase-guide、deploy、security-hardening、troubleshooting、记忆。

### 验证

- `go test ./...`（`singleinstance` 若本机锁占用可跳过）
- `.\scripts\build-local.ps1`

---

## 2026-08-31 · 架构解耦第十八轮（审计整改 + 公共抽取）

### 缺陷修复

- **CIDR 豁免 DELETE**：`handler_security_exempts.go` 移除对 `/` 的误判，改用 `ValidateIPOrCIDR`；补 `TestSecurityExemptCIDRDelete`。
- **`enabled` / `record_events` 语义**：`RecordReject` 落库看 `record_events`，auto-ban 计数看 `enabled`；`guard_record_test` 覆盖四组合。
- **`tunnel/source_ip.go` 注释**：Accept 侧白名单不依赖 probe `enabled`。

### 公共抽取与降耦

- `audit/public_bind.go`：`LogPublicBindEnabled`；`serverapp/boot_persist.go` 不再 import `api`。
- `netutil/validate_ip.go`：`ValidateIPOrCIDR`；`ValidateCIDRList` / guard / API 共用。
- `probedefense`：拆 `signatures.go`、`classify_tls.go`、`classify_handshake.go`、`errors.go`、`auto_ban.go`。
- `api/session_context.go`：`requireAuth` 注入 Session；安全 handler 统一 `actorFromRequest`。
- `api/handler_security_*.go`：按 events/blocks/exempts/common 拆分。
- `tunnel/handshake_reject.go`：握手拒绝 + 探针记录。
- `clientapp/route_view.go` + `clientgui/tray_routes.go`：GUI 不再 import `tunnel`。
- Manual ban 无 Guard 时直写 `ip_blocks`；API 错误文案中文化；`requireMethod` 补全。

### 文档

- 新增 [docs/codebase-guide.md](codebase-guide.md)。
- 更新 architecture、internal/README、security-hardening、记忆、docs/README。

### 验证

- `go test ./...`：除 `singleinstance`（本机已有客户端实例占用锁，环境问题）外全绿。
- `.\scripts\build-local.ps1` 通过。

---

## 2026-08-31 · 封禁豁免白名单 + 客户端封禁友好提示

### 动机

自测手动封禁后客户端仅显示 `tls handshake: forcibly closed`，无法区分封禁与网络故障；需可配置/动态维护「永不封禁」名单（与 `tunnel_allowed_source_ips` 接入白名单语义分离）。

### 改动

- **豁免**：`probe_defense.ban_exempt_ips` + 表 `ip_ban_exempt`；Guard `IsBanExempt`/`ReloadBanExempt`；API `/api/v1/security/exempts`；WebUI「封禁豁免」卡片。
- **客户端**：TCP 接入后 TLS 前写 `HAOVPN:IP_BANNED`；`transport.ErrIPBanned`；`clientapp.FormatDialError` 中文提示并 fatal 停重试。
- **transport**：`ListenTLS` 改为 TCP Accept → CheckAccept → TLS Handshake。

### 验证

- `go test ./internal/probedefense/... ./internal/api/... ./internal/transport/... ./internal/clientapp/... -count=1` 通过。
- 重建 server + client 后：封禁 IP 登录窗应显示友好文案；豁免 IP 不可封且已封记录无效。

---

## 2026-08-31 · WebUI：favicon + 手动封禁可选时长

### 动机

管理端浏览器页签无 icon；探针页手动封禁只能填 IP/原因，时长固定为服务端 `ban_duration_sec`（默认 1 小时），无法选永久或自定义。

### 改动

- **Favicon**：`scripts/gen-icons.go` 生成 `web/static/favicon.ico` / `favicon-32.png`；9 个模板 `<head>` 增加 `rel="icon"`；`webui_csp_test` 回归。
- **后端**：`ManualBan(ip, reason, durationSec)` — `-1` 用配置默认、`0` 永久、`>0` 指定秒；`POST /api/v1/security/blocks` 可选 `duration_sec`；审计 metadata 记录时长。
- **探针 UI**：预设 1 小时～5 年 / 永久 / 自定义（默认 **1 周**）；封禁列表增「封禁时间」列；事件行「封禁」预填 IP；Toast 反馈。
- **测试**：`probedefense/manual_ban_test.go`、`api/security_test.go` `TestSecurityBlocksManualBanDuration`。

### 验证

- `go test ./internal/probedefense/... ./internal/api/... -count=1` 通过。
- 改 static 后须 `.\scripts\build-local.ps1` 重建 server 后浏览器验收 favicon 与封禁表单。

---

## 2026-08-31 · 文档治理：去冗、纠偏、浅分层

### 动机

`docs/` 平铺堆发版草稿与存档/蓝图，入口与路径易乱；`release-notes-*-DRAFT` 与 `VERSION`/`dev-log` 双源；architecture 第十五轮仍写 CSP 含 script unsafe-inline（已不符）。

### 改动

- **删除** `release-notes-0.1.1-DRAFT.md`、`release-notes-0.1.2-DRAFT.md`；[versioning.md](versioning.md) 规定发版只写 VERSION + dev-log +（可选）GitHub Release。
- **浅分层**：活文档仍在 `docs/` 根；`meta-plan` → [archive/meta-plan.md](archive/meta-plan.md)；`mobile-client-plan` → [plans/mobile-client-plan.md](plans/mobile-client-plan.md)。
- **索引**：重写 [README.md](README.md) 放置规则；更新记忆 / 根 README / development-principles / deploy / troubleshooting / `.cursor/rules`。
- **纠偏**：[architecture.md](architecture.md) 第十五轮 CSP 改为 `script-src 'self'` + 禁 HTML `onclick=`。

### 验证

全仓无 `docs/meta-plan.md` / `docs/mobile-client-plan.md` / `release-notes-0.1.*` 现行死链（历史 dev-log 条目内旧路径保留为当时记录）。

---

## 2026-08-31 · WebUI：CSP 拦截 onclick= 导致封禁等按钮失效

### 动机

第十七轮收紧 `script-src 'self'` 后，模板仍残留 `onclick="banIP()"` 等内联事件；浏览器直接拦截，探针页「封禁」无 Network 请求。同类问题波及退出登录与多页按钮。

### 改动

- 全部 `templates/*.html` 去掉 `onclick=`；侧栏退出用 `data-action="logout"`（`app.js` 绑定）。
- 各页 `static/*.js` 用 `addEventListener` 绑定；账号列表改为 `data-act` 委托。
- 回归：`TestEmbeddedTemplatesNoInlineEventHandlers`、`TestEmbeddedStaticJSNoOnclickHTMLLiteral`、`TestWebUIButtonIDsBoundInPageScript`、`TestWebUIAuthPagesExternalScripts`、`TestSecurityPageExternalScript`；hardening / troubleshooting / web README 注明。
- 加宽 CSP 回归：模板扫全部 `on*=`、static 禁 `onclick=` 字面量（注释除外）、`type=button` 的 id 须在页脚本出现、登录后多页 HTTP 烟测。

### 验证

`go test ./internal/api/ -run "CSP|Onclick|SecurityPage|Inline|WebUI|EventHandler|ButtonID|AuthPages|StaticJS" -count=1` 通过。须**重编并部署服务端**（embed）。

---

## 2026-08-30 · 文档治理（入口收口 / 去重）

### 动机

文档入口发散：记忆.md 进度表过长、docs/README 堆轮次摘要、规划存档与现行 CODEMAP 易混淆。

### 改动

- **记忆.md**：只保留阅读顺序 + 当前阶段；历史轮次指向本日志。
- **docs/README.md**：纯索引；发版说明分当前/历史；维护约定写清单一来源。
- **根 README**：快速开始与文档表对齐；去掉重复口号。
- **meta-plan**：强化规划存档头；docs 子树改为现行文件名。
- **architecture / internal README**：轮次细节回指本日志。
- **security-hardening**：账号节密码强度去重。

### 验证

人工核对入口链：记忆 → docs/README → architecture / deploy / hardening / troubleshooting / release-notes-0.1.2。

---

## 2026-08-30 · 架构解耦第十七轮（Cookie / PeerPolicyApplier / CSP / 叶子）

### 动机

审计发现：logout Cookie 属性与 Secure 不一致导致 HTTPS 删不掉；peer dirty/apply 仍沉在 api；WebUI 内联脚本阻塞收紧 CSP；Listen/关连接/viaIndex/脱敏/Windows ACL 与提权路径空格等边角需一次收口。

### 改动摘要

- **安全/正确性**：`setSessionCookie`/`clearSessionCookie` Secure/SameSite 对齐；Touch 重发 Cookie；`must_change` 可 GET CSRF；`transport.Conn.Close` 锁拷贝 `onClose`；viaIndex 重建稳定排序；`peer_access` 须已存在 VPN 用户；`decodeJSONBody` 1MiB；密码 ≤72；logger 脱敏 Authorization/`session=`；历史日志 API items 再脱敏；Windows `EscapeArg`、凭据 `RestrictToAdminsOnly`、`CheckWorldReadable` Everyone；`boot_api` peerDirty 内存 WARN。
- **结构**：`fileutil.EnsureDir`/`AbsPair`/`RestrictToAdminsOnly`；`safeutil.RetryN`；`netutil.StringSlicesEqualTrimmed`；**`vpnaccount.PeerPolicyApplier`**（dirty/apply 出 api）；管理页脚本外置 `web/static/*.js`，CSP `script-src 'self'`（style 仍 unsafe-inline）。
- **文档**：architecture / internal README / hardening / troubleshooting / deploy / 记忆 / meta-plan / web README / docs README / 0.1.1 草稿标历史。

### 验证

```powershell
go test ./internal/api/... ./internal/vpnaccount/... ./internal/safeutil/... ./internal/fileutil/... ./internal/auth/... ./internal/logger/... ./internal/security/... ./internal/platform/... -count=1
go test ./... -count=1
.\scripts\build-local.ps1
```

---

## 2026-08-30 · 架构解耦第十六轮（正确性 / 叶子 / 拆分 / 文档）

### 动机

审计发现：托管路由成员收窄未脏旧成员、应用生效 TOCTOU 清空脏集、`GetPeerRoute` 空指针风险、`local_lans` 可广告 VPN 网段绕过横向隔离；已有 helper 未统一采用；transport/peer_store/runtime/route/engine_boot 仍偏胖；peer DTO 留在 api；CODEMAP 略旧。一次收口，不留「下次」。

### 改动摘要

- **正确性/安全**：成员替换 dirty=旧∪新；apply 仅清成功 done；`GetPeerRoute` 判空；ExitLAN 禁与 `vpn.subnet` 重叠；via/成员须 VPN 账号；`host_id` 长度上限。
- **叶子收敛**：`paginate.ParseBoolQuery`；`httputil` writeItems/parse*Int64/requireMethod；`fileutil.Exists`/`AbsPair`/`CheckWorldReadable`；证书原子写；GUI `requireAdmin`；autostart unix AbsPair。
- **同包拆分**：transport config/conn_loops/server/mtu；persist peer_*；sessionmgr route_*；clientapp runtime_*；serverapp boot_*。
- **边界**：peer 视图→`readmodel/peers.go`；`api/doc.go` 写清 vpnaccount vs persist+sessionmgr；systemd ExecStart 空格引号。
- **文档**：architecture CODEMAP、internal README、hardening §4.3、troubleshooting、dev-log、记忆。

### 验证

```powershell
go test ./internal/api/... ./internal/netutil/... ./internal/persist/... ./internal/fileutil/... ./internal/autostart/... -count=1
go test ./... -count=1
.\scripts\build-local.ps1
```

---

## 2026-08-30 · 架构解耦第十五轮（SCM / 自启 / peer / health）

### 动机

审计发现：CLI 与 GUI 两套 Windows SCM 实现、非 Windows Disable 伪成功、`handler_peers.go` 过胖、公开 health 泄漏数据面、死 re-export、默认口令测试不一致。一次收口，不留「下次」。

### 改动摘要

- **autostart**：SCM install/start/stop/uninstall 唯一写路径；`DefaultServiceStopTimeout`；Linux XDG+systemd、macOS LaunchAgent/Daemon；`gen.go` 生成物单测；stub Disable 诚实报错。
- **clientapp**：`--service` 委托 autostart；Unix `service` 无界面入口。
- **叶子**：`tun.parseCIDR` 不导出；删 persist LAN 薄包装；`security.Redact` 防循环注释。
- **api**：拆 peer handlers；`writeOKWith`/`writePendingApply`；公开 health 仅 `ok`+`uptime_sec`。
- **安全**：测试口令对齐 `changeme12`；登录脚本 `web/static/login.js`；CSP 残留文档化。
- **文档**：architecture / internal README / deploy §5.3 / hardening / cmd README / 记忆。

### 验证

```powershell
go test ./internal/autostart/... ./internal/clientapp/... ./internal/api/... -count=1
go test ./... -count=1
.\scripts\build-local.ps1
```

---

## 2026-08-30 · 文档：开机自启平台说明 + README 去 AI 味

### 完成

- 标明：托盘两个开机自启 **仅 Windows 实现**；Linux/macOS 概念仍两套，托盘未接，须 systemd/launchd 手工配 CLI。
- 更新 deploy §5.2/5.3、troubleshooting、architecture、autostart/doc、internal README、根 README（压缩口号、按平台写清）。

### 关联

- `README.md`、`docs/deploy.md`、`docs/troubleshooting.md`、`docs/architecture.md`、`internal/autostart/doc.go`

---

## 2026-08-30 · GUI 托盘配置：自动连接 + 开机自启 + 退出不卡

### 动机

工控机重启后要自动连 VPN；退出因同步 ICS 假死；托盘需可配自动连接/无窗口/自启。

### 改动摘要

- **退出**：`quitApp` 异步 `stopEngineAsync`，提示「正在退出（清理网络）」；未 Setup via 不跑 DisableAllICS。
- **gui.***：`auto_connect` / `start_minimized`；SaveClient patch；托盘「配置」菜单。
- **autostart**：登录计划任务 HighestPrivileges；可选同一 GUI.exe 注册 Windows 服务；服务占用时 GUI 可接管。
- **文档**：deploy / troubleshooting / architecture / README / 记忆。

### 验证

```powershell
go test ./internal/config/... ./internal/autostart/... ./internal/clientgui/... -count=1
go test ./... -count=1
.\scripts\build-local.ps1
```

---

## 2026-08-30 · 托盘本机路由分栏 + README 品牌

### 动机

托盘「托管路由」只列 peer `managed_routes`，不列 `nat.allowed_lan_cidrs` / AllowedIPs，日志已加 `192.168.3.0/24` 时菜单却显示「无对端托管」，易误判。顺带 README/WebUI 品牌图与术语澄清。

### 改动摘要

- **Engine**：握手保存 `allowedIPs`/`vpnSubnet`；导出 `AllowedIPs()`/`VPNSubnet()`；Stop 清空。
- **托盘**：父项「本机路由」= 本机TUN（真实 subnet）+ 分流 AllowedIPs + 对端托管；Stale 标「失效」；`tray_routes` 单测。
- **Web**：仪表盘列「会话分流前缀」；登录/侧栏用 `static/logo.png`。
- **README**：`docs/assets/haovpn-logo.png`；压缩表述。
- **文档**：troubleshooting / architecture / deploy / internal·web README / 记忆。

### 验证

```powershell
go test ./internal/clientgui/... ./internal/clientapp/... -count=1
go test ./... -count=1
.\scripts\build-local.ps1
```

---

## 2026-08-30 · 发送队列可配 + WebUI 展示时区 + defaults 全注释

### 动机

大文件/电影下载易打满默认发送队列（256）刷 WARN；中国现场 WebUI 默认 UTC 与本地差 8 小时。队列与展示时区做成 YAML 可配，并补全 defaults 模板注释。

### 改动摘要

- **队列**：`vpn.send_queue_size` / 客户端 `server.send_queue_size`（默认仍 256；钳制 64～8192 + Warn）→ `transport.MaxQueueSize`；启动日志 `transport send_queue_size=`。
- **时区**：`api.display_timezone`（默认 UTC；`Asia/Shanghai` / `GMT+8` / `+08:00`）；`timeutil` + 嵌入 `tzdata`；`system/info` 下发；`HaoVPN.formatTime` 四页统一；LAN `updated_at` RFC3339。存库/API 仍 UTC。
- **defaults / 示例 yaml**：服务端与客户端模板逐项中文注释；`config/*_example.yaml` 同步。
- **文档**：deploy / architecture / troubleshooting / hardening / internal README / web README / 记忆。

### 验证

```powershell
go test ./internal/config/... ./internal/timeutil/... ./internal/transport/... ./internal/api/... -count=1
go test ./... -count=1
.\scripts\build-local.ps1
```

手工：`display_timezone: GMT+8` 审计页应为东八；`send_queue_size: 1024` 大文件 WARN 应减少。

---

## 2026-08-30 · 架构解耦第十四轮（叶子工具 + 安全硬化）

### 动机

审计发现：LAN/CIDR 纯函数仍经 `persist` 绕路、`clientapp` 为哨兵依赖 `sessionmgr`、公开 health 泄漏 `recent_errors`、可删末管理员、logout 未强制 POST、GUI `eng` 无锁等。一次收口，不留「下次」。

### 改动摘要

- **netutil**：`NormalizeLANCIDR`/`ValidLANCIDRs`、`NormalizeCIDRList`、`AppendCIDRUnique`；persist 薄包装；clientapp/tunnel 改调 netutil。
- **auth**：`ErrAccountAlreadyOnline`；sessionmgr 兼容别名；clientapp `fatal_auth` 仅依赖 auth。
- **安全**：health 公开无 recent_errors；`ErrLastAdmin` + `CountEnabledAdmins`；logout 仅 POST；`ValidateUsername` 下沉 Provision/EnsureAdmin；`writeInternalError`；`?online=1` 分页修正。
- **api**：`requireMethod`/`parseFormOrError`/`decodeJSONOrForm`/`parsePathID`；peers JSON∪表单走 parseRequestForm。
- **clientgui**：`getEngine`/`setEngine`/`takeEngine`/`clearEngineIf` 与 `engOpMu` 同锁。
- **文档**：architecture / internal README / hardening / 记忆 / 本条。

### 验证

```powershell
go test ./internal/netutil/... ./internal/auth/... ./internal/vpnaccount/... ./internal/clientapp/... ./internal/clientgui/... ./internal/api/... ./internal/health/... -count=1
go test ./... -count=1   # 若本机 HaoVPN 客户端占锁，singleinstance 失败可忽略
.\scripts\build-local.ps1
```

---

## 2026-08-29 · via/ICS 前推迟装分流路由

### 动机

`local_lans` 开启时旧顺序为「装 AllowedIPs → ICS Setup → 清路由再装」，第一次白做且 ICS 常冲掉路由。

### 改动

- `willViaSetupLocked` 预判本次会 Setup 则 `defer_routes`，Setup 成功后再 `clear`+全量安装一次。
- via 失败且尚未有路由时补装一次便于排障。
- 日志：`policy_apply defer_routes=...`。

### 验证

```powershell
go test ./internal/clientapp/... -count=1
```

---

## 2026-08-29 · GUI 登出/手动重连不卡 UI（ICS 后台清理）

### 动机

配置 `local_lans` 后，退出登录或手动重新连接会在 Fyne UI 线程同步 `Engine.Stop` → ICS `DisableAllICS`（PowerShell COM，常数秒），界面假死。多 LAN 时 Teardown 还对每条 LAN 重复关 ICS。

### 改动

- **clientgui**：`engine_stop.go` 后台 Stop；登出/手动重连/登录失败清理不阻塞 UI；忙状态防连点。
- **netstack**：`Teardown` 末尾 `disableICSPlatform` **只关一次** ICS。
- 耗时埋点：`DisableAllICS elapsed=`、`via_exit_teardown elapsed=`、`gui_engine_stop`。

### 验证

```powershell
go test ./internal/clientgui/... ./internal/clientapp/... ./internal/netstack/... -count=1
.\scripts\build-local.ps1
```

---

## 2026-08-29 · 客户端差分重连 + GUI 日志 300 行

### 动机

每次断线都 teardown ICS + 清光分流路由，再握手全量重装；via 机 ICS Setup 耗时长，配置未变时大量工作重复。GUI 日志区过长影响体验。

### 改动摘要

- **临时断线**：`protectForReconnect` 仅启杀开关（若配置），保留 TUN/路由/via/DNS；`Stop`/`dataplaneFailed` 仍 `protectThenClearRoutes` 全清。
- **增量 applyPolicy**：路由集合差分增删；via 指纹未变跳过 ICS；完全一致 `policy_apply mode=noop`；埋点 `dataplane_keep` / `dataplane_clear`。
- **policy_diff.go**：规范化 CIDR、routeSetDiff、viaFingerprint、dnsServersEqual。
- **GUI**：日志 UI 默认保留最近 300 行（磁盘 log 不受限）。

### 验证

```powershell
go test ./internal/clientapp/... ./internal/clientgui/... -count=1
go test ./...
.\scripts\build-local.ps1
```

重连日志期望（配置未变）：`dataplane_keep reason=reconnect` → `policy_apply mode=noop`（或 `dns_only`）；不应反复 `via_exit_teardown`/`via_exit_setup`。

---

## 2026-08-29 · 架构解耦第十三轮（netutil 收口 + ExitLAN 信任边界 + 管理面硬化 + 审计中文）

### 动机

第十二轮后仍有半抽取重复（CIDR/广播/远端主机）、ExitLAN 可被任意客户端滥用绕过横向隔离、管理面 HTTP 无超时与敏感 GET 下载、审计 WebUI 纯英文码难读。

### 改动摘要

- **netutil**：NormalizeCIDROrHost / ForbidDefaultRoute / ValidateAdvertisedLAN（RFC1918+≥/16）/ IsLimitedBroadcast / NormalizeRemoteHost / CIDRListContainsIP；调用方去重。
- **ExitLAN**：仅 viaIndex 命中的 via 可 ExitLAN→对端 VPN IP 旁路；过宽 local_lans 记 lan_cidr_reject。
- **管理面**：HTTP 超时；备份/导出 POST+CSRF；Dashboard requireAuthPage；登录统一文案；Accept 源白名单始终生效；会话滑动+绝对上限+prune；IsAdmin 重查；自动封仅计 rejected。
- **内聚**：decodeJSONBody、flushSessionStat、RouteOutbound 优先 byIP、route DELETE Warn。
- **审计 UI**：audit/labels.go；action_zh / target_username；展示 login（登录）、admin (#1)。

### 验证

```powershell
go test ./...
.\scripts\build-local.ps1
```

---
## 2026-08-29 · ICS SkipAsSource 根因修复（保留 ICS）

### 动机

此前用「客户端 via 禁 ICS」回避问题。真实根因：家庭版 ICS 在 TUN 挂 `192.168.137.1` 后，Windows 本机访 AllowedIPs 可能错选该地址为源 → 超时；关 ICS 只是糊弄。

### 改动

- **保留 ICS**（via / 服务端家庭版 NAT 回退照旧）。
- ICS 启用后：`PreferVPNSourceWithICS`——VPN IP 可作源，ICS 地址 `SkipAsSource=$true`。
- via Setup 后重装客户端 AllowedIPs 路由。
- 撤回 SkipICS。

### 验证

```powershell
go test ./internal/netstack/... ./internal/clientapp/... ./internal/winnet/...
.\scripts\build-local.ps1
```

重连日志应有 `ICS 已启用` + `SkipAsSource 非 VPN 地址`；`ping 192.168.3.1` 与 via SNAT 并存。

---

## 2026-08-29 · via 回程 ExitLANs + 伪造源刷屏治理

### 动机

via/ICS 把 `192.168.3.1` / `192.168.137.1` 灌进隧道；服务端只认 VPN IP → 狂刷 WARN，且 LAN 回程被丢，via 不通。本机 LAN 与服务端 NAT 同网段时还会抢路由。

### 改动

- 服务端：会话加载 `ExitLANs`；入站源=VPN IP 或 ExitLANs；ExitLAN→对端 VPN IP 直转；广播 DEBUG；伪造源 WARN 10s 限流。
- 客户端：TUN 上送过滤（仅 VPN IP / local_lans）；排障文档补充 ICS 与网段冲突。

### 验证

```powershell
go test ./internal/sessionmgr/... ./internal/clientapp/...
.\scripts\build-local.ps1
```

须同时更新 **server + client/gui**。家端 `local_lans` **勿**与服务端 AllowedIPs NAT 网段重叠，勿写 ICS `192.168.137.0/24`。

---

## 2026-08-29 · 断线「账号已在线」半死顶替 + 客户端持续重试

### 动机

ZT/链路黑洞后客户端已断，服务端会话仍占坑；`reject_second` 拒重连；旧客户端仅重试 5 次就停。

### 改动

- 服务端：同 IP grace 顶替保留；对端静默约 8～20s 允许异 IP 密码顶替（`PeerActivityConn`）；拒绝日志带 `same_host`/`stale_peer`。
- 客户端：曾连接或重连中 account_online **不停**；首次登录上限 40 次。

### 验证

```powershell
go test ./internal/sessionmgr/... ./internal/clientapp/... ./internal/transport/...
.\scripts\build-local.ps1
```

须同时更新 **server + client/gui**。

---

## 2026-08-29 · 本地网段注册 + 托管路由解耦 + via 出口

### 动机

家里 LAN 经 via 客户端共享：需临时注册广告、手工托管路由（访问方关系表）、via 侧 TUN→LAN 转发/SNAT；`local_lans` 空则整条关闭。

### 改动

- Schema：`client_lan_registry`；`peer_routes` 去 accessor + `peer_route_members`（`user_id=0`=全部）；旧库迁移 `migrate_peer_routes.go`。
- 握手上报 `local_lans`；断线清注册；策略跳过失效路由；握手下发 `vpn_subnet`。
- 客户端 `via_exit.go` 复用 `netstack.Stack`；GUI/YAML `local_lans`。
- `/peers` 注册表 + 失效展示；`GET /api/v1/lan-registry`。

### 验证

```powershell
go test ./internal/persist/... ./internal/vpnaccount/... ./internal/clientapp/... ./internal/tunnel/... ./internal/api/... ./internal/sessionmgr/... ./internal/config/...
.\scripts\build-local.ps1
```

（全仓 `go test ./...` 时若本机已开 GUI，`singleinstance` 单测会因互斥锁失败，属环境干扰。）

---

## 2026-08-29 · 客户端去网关 /32 冗余 + GUI 重连可恢复性

### 动机

有 `10.88.0.0/24` 时仍装 `10.88.0.1/32` 冗余；GUI `failFast` 登录成功后未关，断线/踢线后再失败即停重连。

### 改动

- `gatewayHostRouteNeeded`：AllowedIPs 已覆盖网关则跳过 `/32`。
- 鉴权成功 `markAuthOK` 关 failFast；`reportFirstFailure` / `onDialError` 仅登录阶段停循环。
- `disconnected_during_policy` 改 `StateReconnecting` 不停循环；曾连接后 `account_online` 持续重试。

### 验证

```powershell
go test ./internal/clientapp/...
.\scripts\build-local.ps1
```

---

## 2026-08-29 · 互访 hub 直转 + 去冗余 /32 + 「应用生效」

### 动机

互访 ping 不通：白名单单向 + 放行后仍 `writeTUN`（无 hairpin）；托管页保存同步踢线卡死；客户端多余 `peer/via /32`。

### 改动

- 入站横向放行 → `sendToAccount` 对端会话。
- `ResolveClientPolicy`：已被 CIDR 覆盖的 peer/via `/32` 不下发；会话仍记 PeerAccess/ViaPeers。
- 增删路由/白名单只写库；`POST /api/v1/peers/apply` 踢脏账号；互访默认双向。
- `/peers` 黄条 + 应用生效按钮；排障文档澄清 on-link。

### 验证

```powershell
go test ./internal/sessionmgr/... ./internal/vpnaccount/... ./internal/persist/... ./internal/api/...
.\scripts\build-local.ps1
```

须重启 **haovpn-server**；改策略后点控制台「应用生效」。

---

## 2026-08-29 · Peer 路由 + 重连续算 + 托盘 Logo（对齐 ZeroTier）

### 动机

默认横向隔离正确，但缺「经对端转发」语义与托盘可见性；抖动重连易被 `account_online` 误杀；控制台无 Managed Routes。

### 设计

- **AllowedIPs** = 经服务端网关/NAT；**托管路由** = `dest via 客户端 vpn_ip`（`peer_routes`，`user_id NULL`=全员）。
- **互访**：`allow_all_vpn_peers` / `peer_access` / via 下一跳；删号级联 `peer_*`。
- **出站**：vpn_ip → via 索引；禁止用 AllowedIPs 错送。
- **重连**：`reconnect_grace_sec` 同 IP 顶替续算；客户端 account_online 有限重试。
- **托盘**：灰/黄/绿/红 + 托管路由只读子菜单；握手 `managed_routes`。

### 验证

```powershell
go test ./...
.\scripts\build-local.ps1
```

---

## 2026-08-29 · 架构解耦第十二轮 + 安全审计闭环

### 动机

审计发现重复工具分散、握手错误靠中文子串耦合、封禁在 `enabled=false` 时未挂 Accept、握手 OK 发送失败留僵尸会话、自改密无旧密码且不吊销 Session、Web/隧道共用 lockout 等。本轮一次做完：高内聚叶子工具 + 安全闭环 + 全局 CODEMAP。

### 改动摘要

- **叶子**：`netutil.SplitRemoteAddr`、`timeutil.Seconds`、`persist.fillIPBlock`；源 IP 统一 `IPMatchesRules`。
- **哨兵**：`auth/errors.go`；`clientapp.IsFatalHandshakeError` / 探针签名用 `errors.Is`；锁定文案对齐。
- **P0**：Probe 始终挂载；握手 OK 失败回滚；改密须 `old_password` + `LogoutAllForUser`；`requireAuth` 失败关闭。
- **P1**：明文钥默认拒绝（`allow_plaintext_private_keys`）；导出不解密；双 lockout；CSRF 常量时间；retention 解耦；sessionmgr 回调/路由加固；`dev-security-check.sh` 对齐 changeme12。
- **文档**：architecture / internal README / hardening / deploy / troubleshooting / web README / 记忆。

### 验证

```powershell
go test ./...
.\scripts\build-local.ps1
```

本轮相关包与 `go test ./...` 中除 `singleinstance` 外全部通过；`singleinstance` 失败因本机已运行 HaoVPN 客户端占用协调端口（环境干扰，同前次记录）。`build-local.ps1` 通过。

field 门禁仍待工控网：`.\scripts\dev-field-gate.ps1 -PlcHost <IP> -UseHomeConfig`（本轮不阻塞）。

---

## 2026-08-29 · 探针审计修复 + 特征中文 / 文档对照

### 动机

审计近期探针防御与登录体验改动：配置哨兵、超时误封、数据面失败无法回登录；安全事件页英文码难读；文档缺对照表。

### 修复

- `probe_defense` 的 `enabled`/`record_events`/`auto_ban` 改为 `*bool`：显式 `false` 不被 ApplyDefaults 改回。
- 读超时 / deadline / 已关闭连接不记探针、不自动封；transport 有 Probe 时由 Guard 单条 Warn（去双行）。
- `IsBlocked`/手动封禁不依赖 `Enabled`；Enabled 只管自动记录/自动封。
- `OnDataplaneFailed`：GUI TUN/路由失败回登录红字；删 `stopReconnectFatal`。
- `probedefense/labels.go` + API `*_zh`；探针页显示「中文（英文码）」。
- 文档：`security-hardening` 完整中英文对照；`deploy` / `troubleshooting` / `architecture` 交叉补全；defaults/example/schema 注释。

### 验证

`go test` 覆盖包通过（`config`/`probedefense`/`clientapp`/`api`/`transport` 等）；`.\scripts\build-local.ps1` 通过。若本机已开客户端，`singleinstance` 测可能因端口占用失败（环境干扰，非本改动）。

---

## 2026-08-29 · 探针防御 + 登录体验修复

### 动机

家里 DDNS 映射隧道口后公网扫描噪声（`GET `/AMQP/SSLv2 等）被记成 ERROR；同账号双端互踢重连循环；GUI 错密码仍进主界面；CLI 密码明文回显。

### 登录体验（A）

- `vpn.session_policy` 默认 `reject_second`：已在线则 `该账号已在其他设备在线`，不踢旧会话；可选 `kick_previous`。
- 握手拒绝经 `SendRawSync` 再 Close，保证客户端收到 `handshake_err`。
- 客户端 `IsFatalHandshakeError` → 停重连；GUI `WaitConnected` 成功后再 `showMain`。
- CLI `PromptPassword` → `golang.org/x/term.ReadPassword`（无回显）。

### 探针防御（B）

- 表：`security_events`、`ip_blocks`；包 `internal/probedefense`。
- Accept 后查封禁/源白名单；TLS/非法帧分类落库；窗口达阈值自动封禁（白名单与 `auth_failed` 不计入）。
- WebUI `/security` + API `/api/v1/security/events|blocks`；retention 清理事件与过期封禁。
- 非法帧由 ERROR+stack 降为带 `remote=` 的 WARN。

### 验证

`go test ./...` 通过；`.\scripts\build-local.ps1` 通过。

---

## 2026-08-28 · 第十一轮：架构收敛 + 审计闭环 + 文档治理 + 法律层授权

### 动机

第十轮后剩余 polish：IP 释放重复逻辑、审计测试补洞、文档漂移、发版授权流程完善（无运行时 license 校验）。

### 架构收敛

- `vpnaccount.releaseDynamicIP`：`DeleteAccount` / `ReleaseOnDisconnect` / 租约清理统一。
- `PlanVPNPatch`、`VPNPatchInput`、`VPNPatchPlan` 移至 `patch.go`；`delete.go` 仅删号。
- `users_export` / `users_vpn` 统一 `ErrAccountNotFound` → HTTP 404。
- `users_crud` `?online=` 复用 `onlineUserSet()`。
- `query_ext_test.go` → `query_page_test.go`。

### 审计闭环

| 项 | 修复/补测 |
|----|-----------|
| A1 form 解析 | 登录/改密/重置密码/CSRF form 失败 → 400 |
| A2 kick DB | `recordDisconnect` 失败打 WARN |
| S1 logs API | `TestLogsAPIRedactsSensitive` |
| S3 Cookie | `TestSessionCookieHttpOnlySameSite` |
| #2 public bind | `TestPublicBindWarnBanner`（security） |
| A1 回归 | `TestPasswordResetBadForm` |

### 文档与授权

- architecture / deploy（#3 网络隔离手工步骤）/ security-hardening §9 发版前。
- [docs/licensing.md](licensing.md) 发版前检查清单。
- NOTICE 与 go.mod 直接依赖一致。

### 验证

```powershell
go test ./...
.\scripts\build-local.ps1
.\scripts\dev-smoke-test.ps1
.\scripts\dev-security-check.ps1
```

---

## 2026-08-28 · 第十轮：架构收敛 + 全量审计 + 文档治理 + 商用授权（法律层）

### 动机

在第九轮基础上完成剩余重复逻辑收敛、安全审计闭环、全局文档治理与 LICENSE 法律层落地（无运行时 license 校验）。

### 架构收敛

- `persist/query_page.go`：`queryPageTotal`；删 `ListAuditLogs`。
- `serverapp/engine_boot.go`：分阶段启动；`engine.go` 仅串联。
- `api`：`writeMethodNotAllowed`、`dataplaneSnapshot`、`buildMonitorItems`；`vpnaccount.ErrAccountNotFound`。
- 删 `security.ClientTLSConfig`、`api/export_test.go`（重复）。

### 安全审计闭环

| 项 | 修复 |
|----|------|
| S1 日志 Redact | `logger.RedactSensitive` 写盘路径；`/api/v1/logs` 返回前脱敏 |
| S2 XFF 绕过 | 默认仅 `RemoteAddr`；`api.trusted_proxy_cidrs` 可选 |
| S3 Secure Cookie | `secure_cookies` 或 HTTPS 时设 Secure |
| S4 密码强度 | ≥8 + 字母数字；默认 admin 模板改为 `changeme12` |
| S5–S6 测试 | 锁定 5 次 + audit；禁用账号握手拒绝 |
| S7 form 解析 | 失败返回 400 |

### 文档与授权

- [README.md](../README.md) 独立部署指南；[docs/licensing.md](licensing.md)；[LICENSE](../LICENSE)、[NOTICE](../NOTICE)。
- 精简 [docs/README.md](README.md)；更新 [security-hardening.md](security-hardening.md)、[architecture.md](architecture.md)。

### 验证

```powershell
go test ./...
.\scripts\build-local.ps1
.\scripts\dev-smoke-test.ps1
.\scripts\dev-security-check.ps1
```

均通过。

---

## 2026-08-28 · 架构解耦第九轮（高内聚 · 低耦合）

### 动机

在第八轮基础上，消除 API 分页/成功信封样板、clientIP 与 HostFromAddr 漂移、魔法租约秒、死代码、胖文件与 retention 裸 goroutine，并同步全局 CODEMAP。

### 完成

**叶子工具**

- `paginate.ParseLimitOffset`；users/audit/monitor events 统一使用。
- `api.clientIP` → XFF + `netutil.HostFromAddr`（修 IPv6）；单测 `httputil_test.go`。
- 删除未使用的 `netutil.FormatListenAddrs`。
- `persist.DefaultIPLeaseSec`；provision/service/users/session_store/schema 注释对齐。

**HTTP 助手**

- 全面 `writeOK` / `writePage`；新增 `writeAttachment`（ZIP/YAML 导出）。
- 去掉 `handleLogs` 对 `parseLogTailQuery` 的双重 Clamp。
- `ip_lease_sec` 用 `ParseIntDefault`；CSRF 迁入 `auth_handlers` 并走 `sessionFromRequest`。
- `auth.setPassword` 私有复用；`ChangePassword` / `ResetPasswordByAdmin` 语义保留。

**安全与领域**

- `maintenance.StartRetentionLoop` → `safeutil.GoSafe("data-retention")`。
- `vpnaccount.ValidateAllowedIPs` 注释标明领域别名边界。

**同包拆分**

- `api/handler_listen.go`（监听生命周期）。
- `persist/query_users.go` / `query_audit.go` / `query_events.go` / `query_monitor.go`（删 `query_ext.go`）。

**文档**

- `docs/architecture.md`（第九轮 CODEMAP + 规则 16/17）、`internal/README.md`、`docs/README.md`、`记忆.md`、本日志。

### 验证

```powershell
go test ./...
.\scripts\build-local.ps1
```

均通过。

### 动机

在第七轮基础上，消除 api 对 persist/sessionmgr 的直接写操作、monitor N+1、重复 helper、胖文件与文档滞后，提升复用与可读性。

### 完成

**叶子工具**

- `paginate.ParseBoolQuery`；api users/monitor 统一使用。
- `timeutil`：`FormatRFC3339` / `ParseSinceRFC3339`；readmodel/api 迁移。
- `config.DefaultServerCertPath` / `ResolveServerCertPath`。
- 删除 `persist/timefmt.go` 垫片与 api `parseIntDefault`/`clampLimit`/`buildClientExportYAML` 薄包装。

**领域边界**

- `vpnaccount.ApplyVPNPatch`、`SetAccountEnabled`（含 OnKickUser）；api 薄 HTTP。
- `auth.MustChangePassword` / `ChangePassword` / `ResetPasswordByAdmin`。
- `readmodel.AuditLogView`、`ConnectionEventView`；persist JOIN 事件带 username。
- monitor online/accounts：`ListMonitorAccountRows(filter)` SQL 筛选，去掉 containsFold 与 N+1。

**同包拆分**

- clientapp：`engine_state` / `engine_lifecycle` / `engine_connect`；`PromptPassword` → credentials。
- api：`handler_routes` / `handler_ops`；`users_crud` / `users_vpn` / `users_export`。
- config：`yaml_node.go`；serverapp：`engine_shutdown.go`。

**其余**

- `health.DashboardMap`；clientgui 地址校验收敛到 `cfg.Validate`；api testutil 合并 `fetchCSRF`/`newTestAPI`。
- 14+ 包 `doc.go` 按 comment-style 补全上游/下游/并发/不变量。

**文档**

- `docs/architecture.md`（第八轮 CODEMAP）、`internal/README.md`（仅 FAQ）、`docs/README.md`、`meta-plan` 目录表、`记忆.md`、本日志。

### 验证

```powershell
go test ./...
.\scripts\build-local.ps1
```

均通过。

---

## 2026-08-28 · 架构解耦第七轮（高内聚 · 低耦合）

### 动机

在已有六轮解耦基础上，消除重复 helper、敏感写盘非原子、YAML 导出与模板漂移、胖文件阅读负担，并修正文档中 GUI/单实例路径漂移。

### 完成

**叶子工具**

- `fileutil.WriteFileAtomic` / `ExecutableDir`；配置、凭据、数据密钥、隧道私钥、wintun.dll 接入原子写。
- 新建 `timeutil`（SQLite UTC layout）；`persist`/`logstore` 共用；`maintenance` 补 `doc.go`。

**统一已有 helper**

- API：`parseSinceQuery`、`writeAPIError`；monitor live 合并抽 helper。
- `platform.CommandOutputError`（linux/darwin route/tun）。
- GUI：`safeutil.RunTickerStop`；`AlreadyRunningMessage` 回退文案。

**配置**

- `config.BuildClientExportYAML` / `ExportServerAddress`；`api/export` 薄封装。
- `DefaultRetentionDays` 从 `netutil` 迁至 `config`。

**同包拆分（导出 API 不变）**

- `auth`：service / password / login / tunnel_login / session / lockout。
- `persist`：store / users / audit_store / session_store。
- `sessionmgr`：manager / register / kick / route / stats。
- `clientapp`：补充阶段注释与 doc 关联（未强行切碎 engine）。

**文档**

- 更新 `docs/architecture.md`、`internal/README.md`、`cmd/README.md`、`记忆.md`。

### 验证

```powershell
go test ./...
.\scripts\build-local.ps1
```

---

## 2026-08-28 · GUI 托盘、登录窗居中、提示框样式

### 完成

- **提示框**：单实例等致命提示改为自定义小窗（无 dialog 套娃）；窗口标题 `HaoVPN`，内容区单独 headline。
- **登录窗**：`CenterOnScreen`；关窗 `Hide` 不退出。
- **托盘**：`tray.go` — 启动即 `installTray`；未登录菜单「显示登录窗口/退出」；已登录含重新连接/退出登录。
- **退出**：仅托盘「退出」与主窗「退出程序」调用 `quitApp`。

---

## 2026-08-28 · 单实例改 localhost TCP 协调（跨平台 / UAC）

### 问题

- 文件锁在 Windows UAC 场景下，非管理员未必能感知管理员实例 → 重复弹 UAC、提权后 Fyne 提示不可见、进程残留。

### 修复

- `singleinstance` 改为 **127.0.0.1 哈希端口 Listen/Dial**（跨平台；Windows 不受提权隔离影响）。
- GUI：`ClientAlreadyRunning` **UAC 前 + 提权后各探测一次**；通过则 `ShowFatalNotice` + `os.Exit`。
- 单测：`TestProbeBeforeUACScenario` 等覆盖 Probe/Acquire/Release。

---

## 2026-08-28 · GUI 单实例僵尸进程 + SaveClient 注释/废除 peer

### 完成

**单实例**

- **根因**：`ShowFatalNotice` 仅 `a.Quit()`，Windows 上 GLFW 线程残留；UAC 先于抢锁导致重复双击 spawn 多个 elevated 子进程。
- **修复**：`runFatalDialog` 后 `os.Exit(0)`；`ClientAlreadyRunning()` UAC 前快检；提权成功 `os.Exit(0)`。
- **单测**：`HAOVPN_GUI_SKIP_DIALOG=1` 供子进程跳过对话框；`gui_dup_test`（环境不可 exec 时 Skip）。

**SaveClient / peer**

- **SaveClient** 改 `yaml.Node` 局部 patch：保留未改段中文注释；只更新 `server.address` + `auth.*`；写盘时删除 legacy `peer` 键。
- **废除 client.yaml peer 段**：vpn_ip/allowed_ips/gateway/私钥均由握手下发；删 `ClientPeerSection`/`PreferGateway`；runtime 用 `netutil.ResolveGateway(握手, "", vpnIP)`。
- 旧 yaml 含 `peer:` → Load 忽略；GUI 连接写回时自动剥离。

### 验证

- `go test ./internal/config/... ./internal/singleinstance/... ./internal/clientapp/...` ✅
- GUI 重复启动 / 记密码注释：须 Windows 手工验收

---

## 2026-08-28 · GUI 记住密码；杀开关仅 yaml

### 完成

- **杀开关**：登录窗移除「断线阻断工控网段」勾选；仅 `client.yaml` → `security.kill_switch` 控制（须 Windows 管理员）。
- **记住密码**：登录窗新增勾选；true 时 `config.SaveClient` 将 `auth.password` 明文写回 yaml（0600），下次预填。
- **配置**：`ClientAuthSection.RememberPassword`；`internal/config/client_save.go` + 单测；扩展 `clientYAMLTemplate` 中文注释。
- **退出登录**：`RememberPassword=true` 时保留密码框内容；否则清空（与改前一致）。

### 安全提示

- 明文密码适合本机专用包；含密码的 `client.yaml` 勿提交 git，权限建议 0600。
- 「保存供 Windows 服务使用」（DPAPI）与「记住密码」（yaml）相互独立。

### 验证

- `go test ./internal/config/...` ✅
- GUI 手工：勾选记住密码 → yaml 含 password；取消勾选 → password 清空；`kill_switch: true` 时断线行为与改前 yaml 一致

---

## 2026-08-28 · GUI 单实例重复启动空白窗修复

### 完成

- **根因**：`client-gui` 单实例锁失败时 `ShowInformation` + `w.Show()`，对话框点确定后 parent 空白窗未关。
- **修复**：`internal/clientgui/notice.go` — `ShowFatalNotice` / `ShowFatalError`；`NewInformation` + `SetOnClosed` → `Quit`；parent 不 Show。
- **login 早期失败**：日志 `InitGlobal` 失败改 `showFatalErrorOnApp`，同样无空白窗。
- **CLI**：行为不变（stderr + exit 1）；新增 `internal/singleinstance/cli_dup_test.go` 子进程验证。
- **脚本**：`scripts/test-client-single-instance.ps1`。

### 验证

- `go test ./internal/singleinstance/... -run TestCLIAlreadyRunningExit` ✅
- `go test ./...` + `build-local.ps1` ✅
- GUI 双开：须手工确认点确定后无残留窗

---

## 2026-08-27 · 架构第六轮（Wintun · 改密 · 删迁移）

### 完成

- **Wintun 启动噪声**：`internal/tun/wintun_log_windows.go` 将 DLL 日志接入 logger，预期 Open 失败降为 Debug；`wintun_adapter_windows.go` 启动前清理 `haovpn0 1` 类孤儿网卡、固定 GUID Create、二次 Open 复用。
- **管理员改密**：`POST /api/v1/users/{id}/password` + WebUI「改密」模态；审计 `admin_reset_password`；单测 `users_password_test.go`。
- **删 DB 迁移**：移除 `migrate_v2/v3.go`、明文私钥启动迁移；`store.migrate()` 仅执行 `schema.sql`（未发布、无旧库）。
- **paginate**：`ParseIntDefault` 从 api 抽到 `paginate/parse.go`。
- **脚本**：`scripts/test-wintun-restart.ps1` 连续启停检查 live.log 与网卡孤儿名。

### 验证

- `go test ./...` ✅
- `build-local.ps1` ✅
- Wintun 重启实测：管理员运行 `.\scripts\test-wintun-restart.ps1`

### 关联

- `internal/tun/wintun_*`、`internal/api/users.go`、`internal/persist/store.go`、`web/templates/user_list.html`

---

## 2026-08-27 · 架构解耦第五轮（分页 · 删号 · 维护解耦）

### Part 1 — paginate 与 persist 辅助

- 新增 `internal/paginate`：`ClampLimit`、`ClampOffset`；api `httputil`、persist `query_ext`、logstore 共用。
- persist 拆分辅助：`query_ext.go`（ListUsersPage 等）、`scan.go`、`jsoncol.go`、`timefmt.go`。

### Part 2 — vpnaccount 删号与 api 解耦

- 新增 `vpnaccount/delete.go`：`DeleteAccount`（踢线、按 ip_mode 释 IP、删 users 行）。
- api `users.go` DELETE 改调 `vpnSvc.DeleteAccount`；生产代码不再 import `ippool`。

### Part 3 — maintenance 后台任务

- 新增 `internal/maintenance/retention.go`：审计/连接事件/历史日志保留。
- 从 api 迁出；`serverapp` 启动 `StartRetentionLoop`。

### Part 4 — 依赖边界

- **netstack → platform**：Windows route/NAT 子进程统一 `platform.Command`。
- **tunnel → tun**：`ServerHandler.TunDev` 类型为 `tun.Device`。
- **readmodel**：`monitor.go`（MonitorRowToItem、MergeLiveSessionStats）。

### Part 5 — clientgui / clientapp 收敛与注释

- 新增 `internal/clientgui`：Fyne UI 从 `cmd/client-gui` 抽出；`main.go` 仅 flag/UAC/单实例。
- 新增 `clientapp/bootstrap.go`（`RunCLI`）、`service_windows.go`（SCM 下沉）。
- 多包 doc.go 与 handler 注释加厚（P0–P5）；遵循 `docs/comment-style.md`。

### Part 6 — 文档

- 新建 `internal/README.md`（包索引 + 改 X 功能来哪）。
- 更新 `docs/architecture.md`、`docs/README.md`、`cmd/README.md`、`web/README.md`、记忆.md。

### 验证

- `go test ./...` ✅ 全绿
- `.\scripts\build-local.ps1` ✅ server/client/client-gui 构建通过
- 行为不变：无新功能；DELETE 账号现经 `vpnaccount.DeleteAccount` 同步释放 `ip_allocations`

### 关联

- `internal/paginate/`、`internal/persist/`、`internal/vpnaccount/delete.go`、`internal/maintenance/`、`internal/api/`、`cmd/client-gui/`、`docs/architecture.md`、`internal/README.md`

---

## 2026-08-27 · 架构解耦第四轮（高内聚 · 低耦合）

### Part 1 — netutil 公共能力收敛

- 新增 `addr.go`：`HostFromAddr`、`ParseHostIP`、`NormalizeIPv4`、`DedupTrimNonEmpty`。
- `SplitCIDR`、`ParseCIDRToV4Mask` 从 netstack 迁入 `cidr.go`。
- 单测 `internal/netutil/addr_test.go`；netstack 测试改调 netutil。

### Part 2 — 大文件拆分

- **api**：`users.go`、`auth_handlers.go`、`httputil.go`。
- **clientapp**：`runtime.go`。
- **transport**：`frame.go`、`reconnect.go`。

### Part 3 — 耦合边界

- **security**：`BuildClientTLSFromOptions`、`ClientTLSConfigWithRootCAs`。
- **sessionmgr**：`PacketConn`（`conn.go`）。
- **readmodel**：DTO 从 persist 剥离。

### Part 4 — fileutil 与注释

- **fileutil**：`EnsureParentDir`；替换 5 处 MkdirAll。
- doc.go 加厚；`tun_windows.go` 注释补全。

### Part 5 — 文档

- `docs/architecture.md` HTTP API 路由表；`cmd/README.md`；记忆.md / docs/README 更新。

### 验证

- `go test ./...` ✅
- `.\scripts\build-local.ps1` ✅

### 关联

- `internal/netutil/`、`internal/api/`、`internal/clientapp/`、`internal/transport/`、`internal/readmodel/`、`internal/fileutil/`、`docs/architecture.md`

---

## 2026-08-27 · 架构第三轮 + 全量注释补全

### Part 1 — 代码收敛（无功能变更）

- **Server ApplyDefaults**：`netutil.DefaultRetentionDays`；`ServerConfig.ApplyDefaults()`；Validate 去心跳字面量；`FromServerVPN` 假定已 ApplyDefaults。
- **网关 netutil**：`gateway.go`（InferGatewayFromVPNIP、ResolveGateway、IsLoopbackHost）；config/client 删 `ResolveGateway` 死代码；clientapp teardown 统一 PreferGateway。
- **删死代码**：删除 `api/account.go`；handler 直连 vpnSvc；删 `netstack.ParseCIDRs`（测试改 netutil.ValidateCIDRList）。
- **cmd 收敛**：CLI 默认 `-c` 用 `ResolveClientConfigPath`；`clientapp.ResolveCredentials`；export 用 `brand.DefaultTunName`。
- **security**：cert/keyenc 去掉重复 Package 声明。

### Part 2–3 — 注释规范与补全

- 新增 `docs/comment-style.md`；development-principles 链接注释规范。
- Tier1 核心（engine、handler、sessionmgr、tunnel、transport、persist 等）导出符号与字段中文 godoc。
- Tier2–4：netstack/winnet/netutil/config/security/cmd/web 等补全；transport/pool 英文 godoc 改中文。
- 新建 `web/README.md`；`web/static/app.js` 主要函数中文注释。

### Part 4 — 文档

- `docs/architecture.md` 增加「关键文件索引」表。
- `记忆.md` 进度表与阅读顺序更新。

### 验证

- `go test ./...` ✅
- `go build ./...` ✅
- `.\scripts\build-local.ps1` ✅

---

## 2026-08-27 · 架构解耦第二轮（无功能变更）

### 完成项（Phase A–G）

- **netutil 扩展**：`ResolveMTU`、`ReadBufferSize`、`IPMatchesRules`、`ValidateIPInSubnet`、`SplitHostPortLoose`；心跳/重连/MTU 常量单一来源。
- **config**：`ClientConfig.ApplyDefaults()`、`loadYAML` 泛型；删除 `validate_net.go`；GUI/服务统一 `ResolveClientConfigPath`。
- **transport**：`FromClientConfig` / `FromServerVPN`；clientapp/serverapp 删除手写 tcfg 映射。
- **清理 re-export**：删除 `api/util.go`；serverapp 直连 `netutil`；security 去掉 `ResolveListenAddrs` 转发。
- **export / vpnaccount**：导出 YAML 默认值对齐 `ApplyDefaults`；`ValidateManualIP` 委托 `netutil.ValidateIPInSubnet`。
- **doc.go**：28 个 `internal/*` 包补中文 `doc.go`；英文 Package 注释迁入 doc.go。
- **文档**：`docs/architecture.md` 扩展为 CODEMAP；`记忆.md` / README / docs/README / meta-plan 接入导航。

### 验证

- `go test ./...` ✅
- `go build ./...` ✅
- `.\scripts\build-local.ps1` ✅

### 说明

本轮仅优化代码结构与文档导航，**不改变 VPN 业务行为**。

---

## 2026-08-27 · GUI 发版策略：仅 Windows 含 client-gui

### 决策
- 曾评估 Gio + systray 重写以解决 Fyne 交叉编译；实测 **Linux/macOS GUI 仍无法从 Windows 主机 CGO=0 交叉编译**，与 Fyne 类似受平台图形栈限制。
- **回滚**未提交的 Gio 迁移，保留 **Fyne `cmd/client-gui`**。
- **release**：`build-release.ps1` 在 **windows/amd64、windows/arm64** zip 内额外打包 `haovpn-client-gui.exe`；Linux/macOS zip 仅 server + client（CLI）。
- `build-local.ps1` 仍本机构建 GUI；`build-release.sh` 不构建 GUI（须在 Windows 上用 ps1）。

### 验证
- `go test ./...`
- `.\scripts\build-local.ps1`
- `.\scripts\build-release.ps1`

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

