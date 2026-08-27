# 代码全局审查记录（2026-08-24 第二轮）

> 原则：对照源码，不做「看起来能跑」的假设。区分 **致命 / 严重 / 已知短板**。

## A. 本轮新发现并已修复

| 级别 | 问题 | 证据 | 修复 |
|------|------|------|------|
| **致命** | 隧道加解密密钥双方不一致 | `deriveKey` 用 `priv XOR peer`；测试只测「自己加自己解」并注释称简化 | 改为 X25519 共享密钥 + SHA256 域分隔；`TestCrossSessionRoundTrip` 强制两端互通 |
| **致命** | 客户端握手竞态 | `SendRaw` 后再 `SetOnData`，应答可能先到导致超时 | 先注册回调再发送；`sync.Once` 关闭 channel |
| **严重** | macOS TUN 未配 IP | `tun_darwin` 开 utun 后无 ifconfig | `ifconfig inet ... up` |
| **严重** | Peer JSON 导出大写字段 | 无 json tag，WebUI 靠 `p.ID\|\|p.id` 凑合 | 加 `json` tag；私钥 `json:"-"` |

## B. 上一轮已修（仍有效）

| 问题 | 状态 |
|------|------|
| Windows netstack 空壳（只打日志） | 已改为真实转发 + New-NetNat / 失败报错 |
| Linux NAT 源网段写反 + eth0 硬编码 | 已改为 `-s VPNSubnet -d LanCIDR MASQUERADE` |
| Linux TUN 未 `ip addr` | 已修 |
| Windows 客户端 `route ADD ... IF name` | 已改为 `MASK` + gateway |
| 导出 `0.0.0.0`→`127.0.0.1` | 已改为 `REPLACE_WITH_SERVER_IP` |
| Wintun 空读当错误退出 | 已 WaitForSingleObject |
| Wintun 未 netsh 配 IP | 已配 |
| CSP 拦内联登录 JS | 已允许 unsafe-inline |

## C. 仍存在 / 诚实标注（未假装完成）

| 项 | 说明 | 是否挡 v1 本机+跨网隧道 |
|----|------|------------------------|
| Windows NAT 依赖 WinNAT | `New-NetNat` 失败会 Fatal（nat.enabled）；无 WinNAT 时需改配置 | 隧道可连；访问 LAN 可能需手工回程 |
| macOS 服务端 NAT | `nat.enabled=true` **返回 error**（不假装成功）；主推 Linux/Win | 客户端不受影响 |
| KDF 非完整 Noise_IK | X25519+SHA256 + 防重放窗口（非完整 WG 握手） | 对称可用 |
| ~~私钥明文存库~~ | ✅ AES-256-GCM（`enc:v1:`） | — |
| Darwin utun 实现较简 | connect 方式因内核而异，现场需验证 | 非 Windows 主路径 |

## D. 验收时看 live 日志（新二进制）

期望：

```text
windows TUN IP 已配置: 10.88.0.1/...
管理 API 已监听: 127.0.0.1:8080
（可选）管理 API 已监听: 10.88.0.1:8080
netstack: IP 转发已开启
NAT 成功或明确 ERROR（禁止再出现 “ICS/NAT setup” 空话）
```

## E. 第三轮补充（2026-08-24 续）

| 级别 | 问题 | 修复 |
|------|------|------|
| **严重** | `SetPeerEnabled` 有实现但无 HTTP/API/WebUI 入口，禁用 peer 只能删库 | `POST /api/v1/peers/{id}` `action=disable\|enable` + WebUI 按钮 |
| **严重** | 删除 peer 不踢线，隧道仍可收发 | `DELETE` 前先 `KickPeer` |
| **性能** | `HandleInbound` 每包 `UpsertSessionStat` | 5s 节流落库，内存计数不变 |
| **测试** | `TestTLSConcurrentEcho` 全量 `go test` 偶发 1/8 超时 | 错峰启动 + 超时 30s |

### 你当前 live 日志仍是旧二进制

`home/logs/server.live.log`（11:02）仍出现：

- `manual/route command may be needed`
- `ICS/NAT setup`
- **无** `windows TUN IP 已配置`

说明尚未用本轮编译的 `bin/haovpn-server.exe` 重启。新日志应出现 `New-NetNat` 或明确 ERROR，以及 `windows TUN IP 已配置`。

## F. 第四轮审计（2026-08-24 12:44）

### 新发现并已修

| 级别 | 问题 | 修复 |
|------|------|------|
| **P0** | `RegisterPeer` 持锁 `Close` 旧连接 → `onClose`→`RemovePeer` 死锁，客户端重连必挂 | 先解锁再 Close；`RemovePeerIfConn` 防旧 onClose 误清新会话；`TestHandshakeReconnectNoDeadlock` |
| **P1** | `tunnel_allowed_source_ips` 配置存在但未校验 | `CheckTunnelSourceIP` + `ServerHandler.AllowedSourceIPs` |
| **P1** | 服务端 shutdown 不关隧道 listener | 保留 `tunnelSrv` 并 `Close()` |
| **P1** | 客户端断线后仍用 stale `activeConn` 发包 | `SetOnClose` 清空指针 |
| **P1** | 客户端网关硬编码 `.1`，导出无 `gateway_ip` | 导出 `gateway_ip`；`peer.ResolveGateway()` |
| **P1** | Linux 客户端路由忽略 gateway | `ip route via gateway dev tun` |
| **P1** | 用户 disable/enable 吞 DB 错误仍返回 ok | 检查 `SetUserEnabled` 错误 |
| **P1** | 握手失败路径无 `handshake_err` 帧 | `rejectHandshake` 统一应答 |
| **P1** | 自动生成 client 配置 exit 0 | 返回 error 非零退出 |
| **P1** | TUN 未就绪静默丢包 | WARN 日志 |
| **P1** | TLS ServerName 写死 localhost | 从 address 推导或可配置 `server_name` |
| **P2** | `RouteOutbound` 持 RLock 加密发送 | 查找后解锁再 send |
| **P2** | 断线不更新 session_stats | `recordDisconnect` 写 last_heartbeat |

### 仍诚实标注

| 项 | 说明 |
|----|------|
| ~~私钥明文存库~~ | ✅ 已 AES 加密；非完整 Noise_IK 仍为 X25519+SHA256 对称会话（meta-plan 内层密码学已满足互通+防重放） |
| macOS NAT | `nat.enabled=true` 时返回 error，需手工 pf；服务端主推 Linux/Win |
| WinNAT 依赖 | New-NetNat 失败会 Fatal（nat.enabled）；无 WinNAT 需改配置或手工回程 |
| 开发冒烟 `require_tun: false` | E2E/acceptance 专用；`home/server.yaml` 默认 `require_tun: true` |
| step11 实机 | sudo TUN / PLC / Windows 服务重启仍须管理员手工验收 |

## G. 第五轮补齐（2026-08-24 12:49）

| 级别 | 问题 | 修复 |
|------|------|------|
| **P1** | TUN/NAT 失败仍假装可跑 | `require_tun: true` 默认；`nat.enabled` 时 Setup 失败 Fatal |
| **P1** | health 不反映 TUN/NAT | `/api/v1/health` 增加 `tun_ok`/`nat_ok`，`ok` 综合判定 |
| **P1** | 配置语义未校验 | subnet/gateway/CIDR/listen 格式校验 + 单测 |
| **P2** | AuditEntry JSON 无 tag | 补 `json` tag |
| **P2** | RouteOutbound 随机 map 顺序 | 按 peer ID 排序匹配 |
| **P2** | 客户端 Send 静默丢包 | 失败打 WARN |
| **P2** | Windows 服务 Start 吞错 | 检查 `Start()` 返回 |
| **维护** | manager.go 异常空行 | 重写规范化 |

## H. v1.0 真实收尾（2026-08-24）

对照 `meta-plan.md` 补齐此前误标为「可后置」的项：

| 项 | 状态 |
|----|------|
| 私钥 AES 落库 | ✅ `keyenc` + 启动迁移 |
| 防重放窗口 | ✅ counter nonce + `wireguard/replay` |
| IP 池持久化 / 重启不撞 IP | ✅ |
| Peer VPN IP 横向隔离 | ✅ |
| zip 导出 + 日志快照 API | ✅ |
| 文件权限 WARN / reconnect_count / macOS NAT 诚实 | ✅ |
| ProbeMTU / panic 存活单测 | ✅ |
| `build-release` 6 平台 | ✅ |
| `build-release` 6 平台 | ✅ |
| field 硬门禁脚本 | ✅ 已编写，待实跑 |
| 管理员 field 实跑 | ⏳ 开发者执行 |

## I. 硬门禁（2026-08-24）

| 项 | 说明 |
|----|------|
| smoke vs field | `dev-acceptance` = smoke；`dev-field-gate` = 交付门禁 |
| 假 WithSudo | 已移除（原 require_tun=false 路径） |
| 硬断言单测 | AES API、logs API、migrate、ProbeMTU、health、reconnect_count |
| 未完成 | field 0 FAIL 须开发者本机 sudo + PlcHost |

## J. B 后原则审计（2026-08-26）

| 级别 | 问题 | 状态 |
|------|------|------|
| P0 | 隧道未强制 `must_change_password` | ✅ `VerifyTunnelLogin` 硬拒绝 + 单测 |
| P0 | 杀开关 Enable 失败仍 clearRoutes 泄漏 | ✅ 失败禁止清路由；`LastError`/`KillSwitchOK` |
| P0 | WFP 仅删内存 filterId，残留清不掉 | ✅ 按子层 GUID 枚举删除 |
| P1 | 生产 goroutine 未 GoSafe | ✅ lease/TUN/GUI/API/服务 |
| P1 | 非 Win DNS 文案含糊 | ✅ 明确「仅支持 Windows」 |
| P1 | 锁定等对外英文错误 | ✅ 中文 |
| — | Win11 家庭版 NAT / field 实跑 / API HTTPS | 非缺陷，诚实标注 |

过时提示：旧「advfirewall / CurrentUser DPAPI / 缺证引导 skip-verify」均已废止；以 `docs/dev-log.md` 最新条为准。
