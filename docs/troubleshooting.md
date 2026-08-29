# 故障排障

> 按现象查找。更多背景见 [deploy.md](deploy.md)、[meta-plan.md](meta-plan.md)。

---

## 1. 服务端无法启动

| 现象 | 可能原因 | 处理 |
|------|----------|------|
| `bindcheck` / 拒绝启动 | `listen_hosts` 含 `0.0.0.0` 但 `allow_public_bind: false` | 改回 `127.0.0.1` 或显式 `allow_public_bind: true`（仅开发） |
| 证书错误 | 证书路径不对或过期 | 检查 `certs/`；重生成或 `tls.auto_generate: true` |
| TUN 创建失败 | 权限不足 | Linux sudo / `CAP_NET_ADMIN`；Windows 管理员运行 |
| 数据库错误 | 目录不可写 | 检查 `database.path` 父目录权限 |
| 配置校验失败 | YAML 字段错误 | 看启动日志指出的字段名 |

**日志位置**：`./logs/server.log`（可在 `server.yaml` 的 `log.file` 修改）

---

## 2. 客户端连不上

| 现象 | 可能原因 | 处理 |
|------|----------|------|
| TLS 握手失败 | 地址/端口错、证书不信任 | 见 [deploy.md § TLS 证书](deploy.md) |
| 连接超时 | frp 未通、防火墙拦 8443 | 检查 frp；测 8443 端口 |
| 认证失败 | 账号/密码错、账号禁用、IP 锁定、**须先改密** | 核对账号；须改密时先在 Web 改密再连隧道；锁定提示「登录失败次数过多，请稍后再试」——客户端应**停止自动重连**（`IsFatalHandshakeError`）；WebUI 探针页可见 `auth_failed` |
| 提示「该账号已在其他设备在线」 | `session_policy=reject_second` 且旧会话仍在；异公网 IP 第二端；或底层黑洞导致服务端半死会话未释放 | 同公网 IP：`reconnect_grace_sec` 顶替；半死静默约 8～20s 后亦可顶替（须升级服务端）；**曾连通过**的客户端持续重试；首次登录最多约 40 次。异设备先退旧端或改 `kick_previous`。查服务端日志 `grace 顶替` / `拒绝第二端 … same_host= stale_peer=` |
| GUI 断线后不再自动重连 | 旧版登录 `failFast` 成功后未关，或 account_online 仅重试 5 次即停 | 升级客户端：鉴权成功后关 failFast；曾连接/重连中 account_online **持续**重试；首次登录最多约 40 次；并升级服务端半死会话顶替 |
| 日志出现多余 `10.88.0.1/32` 路由 | 旧版始终加网关主机路由 | 新版：AllowedIPs 已含 VPN 子网时跳过网关 `/32` |
| 能 ping 网关 / AllowedIPs LAN，不能 ping 其他客户端 VPN IP | **默认设计**：横向隔离；或未点「应用生效」；或服务端未直转对端会话 | 控制台 `/peers`：开「全部互访」、加**双向**白名单，或配托管路由（via 下一跳在服务端放行）；改完后点 **应用生效** 踢线刷新；升级含 hub 直转的服务端 |
| 托管路由不生效 / ping LAN 得「来自 via：无法访问目标网」 | hub 已送到 via，但 via 未开出口或 SNAT 失败；或未配 `local_lans`；或托管路由「失效」 | 家里客户端配 `local_lans` 并以管理员连接；日志 `via_exit_setup ok` / `ICS 已启用` / `SkipAsSource`；控制台注册表有行；点「应用生效」；勿写 ICS 网段进 `local_lans` |
| 托管路由一直「失效」 | via 离线，或注册表无匹配 dest（未上报 / 已下线清空） | via 保持在线且 `local_lans` 含该 dest；换机后须重登上报 |
| 未配 local_lans 却想共享 LAN | 能力默认关闭 | 在 `client.yaml` 或 GUI「本地网段」填写 CIDR |
| 服务端狂刷 `丢弃伪造源 IP`（`192.168.137.1` / 家用 LAN） | via/ICS 把非 VPN 源灌进隧道；旧服务端只认 VPN IP | **升级服务端+客户端**：ExitLANs 放行已上报 `local_lans` 回程；客户端过滤非 VPN/非 local_lans 源；广播改 DEBUG。勿把 ICS `192.168.137.0/24` 写进 `local_lans` |
| 家/本机能连 VPN，ping 对端 VPN IP 通，但不通服务端 NAT（如 `192.168.3.1`），且开了 local_lans/ICS | ICS 在 TUN 挂 `192.168.137.1` 后 **Windows 错选发包源** | **升级客户端**：ICS 后对非 VPN 地址 `SkipAsSource`，并重装 AllowedIPs；日志 `本机发包源优先 10.88.x.x`。`Get-NetIPAddress` 看 137 地址应为 SkipAsSource |
| Windows 路由表「在链路上」且接口是本机 VPN IP | **预期**：进 haovpn0，不是把 via 配成自己 | 控制台 `via 10.88.x.x` 只在服务端选路；本机不必出现「下一跳=via」 |
| `/peers` 增删很卡 | 旧版保存时同步踢线抢 SQLite | 新版保存只写库；点「应用生效」再踢受影响账号 |
| 手动封禁 IP 仍能连上 | 旧版仅在 `probe_defense.enabled=true` 时挂 Probe | 升级服务端：有 Guard 即挂载，封禁表 Accept 始终生效；查 `/security` 与 `ip_blocks` |
| 提示「账号密钥须加密存储」 | 库内明文私钥且 `allow_plaintext_private_keys=false` | 重新开户/轮换密钥使私钥加密入库；临时兼容才开 `allow_plaintext_private_keys`（勿用于生产） |
| 反复断连 | 心跳超时、ZeroTier 等损耗链路抖动 | 客户端 `heartbeat_timeout_sec` 建议 60～90；默认已 90s；**先 `ping` 底层 ZT IP**（如 192.168.196.17），若底层也超时则属 ZeroTier/运营商问题，不是隧道逻辑 |
| 断线后「好久才连上」 | 旧版 TCP Dial 空等 10s + 退避到 8s，ZT 黑洞时体感约 30s | 新版默认 `dial_timeout_sec: 3`、`reconnect.max_sec: 3`；曾连上再断会立即重拨。**修的是探测节奏**，不能让 TCP 穿透 ZT 黑洞；ZT 仍抖时只能跟底层走 |
| ping 网关间歇丢包 | 同上：底层 ZT 丢包会连带 `10.88.0.1` 丢 | 对比双 ping；ZT 稳后再看 VPN |

**日志位置**：`./logs/client.log`

---

## 3. 能连上但 ping 不通工控设备

| 现象 | 可能原因 | 处理 |
|------|----------|------|
| ping 不通 **10.88.0.1（网关）** | Windows 客户端路由未绑 Wintun IF（/32 TUN + via gateway） | 升级含 on-link `IF` 路由的客户端；`route print` 看 `10.88.0.0` 是否指向 haovpn0 |
| ping 不通 PLC | AllowedIPs 未含工控网段 | 检查账号的 `allowed_ips` |
| 网关通、LAN 不通 | NAT/SNAT 未就绪 | 见 [deploy.md § NAT](deploy.md)；日志 `nat_ok=false` |
| 路由问题 | 客户端路由未写入 | 重启 client；Windows 管理员权限 |
| PLC 禁 ping | 设备本身不响应 ICMP | 用 `telnet 502` 等测实际端口 |

**相关日志**：
- 客户端大量 `隧道发送失败: not connected` → 旧版握手竞态；升级最新 client。
- ZeroTier 下约 20～30s 断一次 → 旧版 `heartbeat_timeout_sec: 30`；改为 90。
- 服务端组播 `224.0.0.x` / `239.255.255.250` 丢弃 → Windows 正常探测，已降为 DEBUG。

---

## 2.1 客户端 GUI（haovpn-client-gui）

| 现象 | 处理 |
|------|------|
| 重复启动提示后留空白窗 | 旧版单实例失败未 Quit parent Window | 升级含 `clientgui.ShowFatalNotice` 的版本；点确定后应自动退出 |
| 多个 gui.exe 僵尸进程 / 重复 UAC 后无窗 | 旧版文件锁 + UAC 隔离；或 Quit 未 Exit | 升级含 localhost TCP 单实例版本；临时 `Stop-Process -Name haovpn-client-gui -Force` |
| 已在运行提示 | 单实例锁占用（CLI/GUI/服务共用） | 托盘「退出」或结束已有 `haovpn-client`/`haovpn-client-gui` 进程 |
| 找不到配置 | 未传 `-c` 时按 **exe 同目录 `client.yaml`** → 当前目录；双击 exe 建议把 yaml 放在 `bin\` 旁 |
| TUN 失败 / 提示须管理员 | 非管理员会弹 UAC；拒绝则登录窗中文提示；同意后以提权实例继续 |
| 日志区字看不清 | 已用可读主题（深色字）；请用新版 GUI |
| 启动多一个黑控制台 | 新版 GUI 以 `-H windowsgui` 构建，应无控制台；CLI `haovpn-client.exe` 仍有控制台属正常 |
| 关窗后进程还在 | 正常：登录窗/主窗关窗仅 Hide，托盘仍在；须托盘「退出」或主窗「退出程序」 |
| 未登录无托盘 | 旧版托盘仅登录后出现 | 升级本版：启动即托盘，「显示登录窗口」可恢复 |
| 旧 TUN 网卡残留 | 网络连接中禁用旧 `myvpn0` 适配器 |
| 需重新登录 | 托盘或主窗「退出登录」；未勾选「记住密码」时密码框清空 |
| 记住密码 | 登录窗勾选后 patch 写回 `client.yaml`（保留其它段注释）；`auth.remember_password` + `auth.password` |
| client.yaml 注释消失 / 含 peer | 旧版全量 Marshal 覆盖 | 升级本版 SaveClient；legacy `peer:` 会在 GUI 写回时删除（策略由握手下发，勿手配） |
| 杀开关 | 仅 `client.yaml` → `security.kill_switch`；GUI 登录窗无此项 |
| TLS 证书 / SAN / CA | 见 [deploy.md § TLS 证书](deploy.md) |
| DNS / 服务凭据 | 见 [deploy.md](deploy.md) 对应章节 |

CLI 等效：`auth.username` + yaml 内 `auth.password` 或 `HAOVPN_PASSWORD` 或 GUI 输入密码。

---

## 4. 管理 WebUI 访问不了

| 现象 | 可能原因 | 处理 |
|------|----------|------|
| 本机 127.0.0.1 不通 | 服务未起、端口错 | `curl http://127.0.0.1:8080/api/v1/health` |
| VPN 内不通 | 未绑 TUN IP | 确认 TUN 已启动；看 `listen_hosts` |
| 登录失败 | 密码错 / 账户锁定 / **非 admin 账号** | 等锁定过期；工程师账号仅隧道登录，Web 须 admin |
| CSRF 错误 | 浏览器缓存 | 清缓存重登 |

---

## 5. 性能与稳定性

| 现象 | 可能原因 | 处理 |
|------|----------|------|
| 延迟高 | TCP-over-TCP、MTU 过大 | 降低 MTU（如 1420→1280） |
| 内存涨 | 连接泄漏 | 查 session 数；重启并看 dev-log |
| panic 日志 | 代码 bug | 看堆栈；提 issue / dev-log |

---

## 6. 常用诊断命令

```bash
# 健康检查
curl -s http://127.0.0.1:8080/api/v1/health

# 版本
./haovpn-server -version

# Linux：TUN 是否存在
ip link show | grep -E 'tun|HaoVPN'

# Linux：NAT 规则
sudo iptables -t nat -L -n

# Windows：路由表
route print

# 日志尾部
tail -f ./logs/server.log
```

---

## 6. Wintun 启动日志（Windows 服务端/客户端）

| 现象 | 说明 | 处理 |
|------|------|------|
| `Failed to find matching adapter name` / 0x490 | Wintun DLL 在 **OpenAdapter 未命中** 时的预期日志；新版已降为 Debug | 若仍为 ERROR 级 raw log，升级最新 build；见 `server.live.log` 中 `[DEBUG] wintun:` |
| `Removed orphaned adapter "haovpn0 1"` | Windows 因重名给旧网卡加后缀；启动时会清理并 Create | 连续重启后应减少；可跑 `.\scripts\test-wintun-restart.ps1`（管理员） |
| `windows wintun haovpn0 已复用` | 正常：第二次启动复用适配器 | 无需处理 |
| WinNAT / ICS 失败 + `forward_only` | Win11 家庭版常见；服务仍可隧道/ping 网关 | 见 [deploy.md § NAT](deploy.md)；工控跨网段需 Pro/Hyper-V 或手工 ICS |

---

## 7. 安全事件 / 探针页

WebUI「探针」`/security`；特征中英文对照见 [security-hardening.md §4.2](security-hardening.md)。

| 现象 | 含义 | 处理 |
|------|------|------|
| 大量 `http_get` / `tls_*` / `amqp` | 公网扫描撞隧道口 | 正常噪声；可看自动封禁；**勿映射管理口** |
| `auth_failed` | 错密尝试 | 查账号；默认不计入自动封 |
| `account_online` | 同账号第二端被拒 | 旧端先登出，或改 `vpn.session_policy: kick_previous` |
| 合法客户端被封 | 误封 / 扫描同出口 IP | 探针页解封；调大阈值或把特征加入 `ignore_signatures_for_ban` |
| 写了 `enabled: false` 仍拦已封 IP | 封禁表始终生效 | 预期行为；解封或清 `ip_blocks` |

---

## 8. 上报问题时请提供

1. `haovpn-server -version` / `haovpn-client -version` 输出
2. 脱敏后的 `server.yaml` / `client.yaml`（**去掉私钥和密码**）
3. 相关时间段日志（`server.log` / `client.log`）
4. 网络拓扑（是否 frp、端口映射方式）
5. 复现步骤

---

*最后更新：2026-08-29 · 探针事件排障*

