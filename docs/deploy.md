# 部署文档

> 现场部署、开发联调、验收测试的完整指南。脚本用法见 [scripts/README.md](../scripts/README.md)。

---

## 1. 架构与拓扑

### 1.1 典型现场拓扑（frp 反向隧道）

```
                    ┌─────────────┐
  工程师笔记本       │  VPS / 公网  │
  (haovpn-client)    │  frps:7000   │
       │            └──────┬───────┘
       │ TLS-TCP:8443      │ frp 隧道
       └───────────────────┼──────────────────┐
                           │                  │
                    ┌──────▼──────┐    ┌──────▼──────┐
                    │  项目现场    │    │  工控局域网  │
                    │ haovpn-server│───►│ PLC/SCADA   │
                    │  frpc       │    │ 192.168.x.x │
                    └─────────────┘    └─────────────┘
```

- **隧道端口**（默认 `8443`）：可走 frp 暴露给工程师，承载 TLS-TCP VPN 数据。
- **管理端口**（默认 `8080`）：**默认仅本机 + VPN 网卡**；不要经 frp 暴露到公网。
- 管理访问方式：现场本机浏览器，或工程师 **先连 VPN** 后访问服务端 TUN IP。

### 1.2 端口清单

| 端口 | 协议 | 服务 | 暴露建议 |
|------|------|------|----------|
| 8443 | TLS-TCP | VPN 隧道 | 可经 frp 映射 |
| 8080 | HTTP | 管理 API / WebUI | **仅本机 / VPN 内** |
| 7000 | TCP | frps（若用 frp） | 公网 VPS |

---

## 2. 环境要求

### 2.1 服务端（项目现场）

| 平台 | 要求 |
|------|------|
| Linux | 内核支持 TUN；`CAP_NET_ADMIN` 或 root；iptables/nftables |
| Windows | 管理员权限；**`haovpn-server.exe` 内嵌 Wintun**（首次启动 TUN 时在 exe 同目录释放 `wintun.dll`，目录须可写） |
| macOS | 管理员权限；utun |

- 磁盘：≥100MB（含日志、SQLite）
- 内存：建议 ≥256MB

### 2.2 客户端（工程师）

| 平台 | 要求 |
|------|------|
| Windows 10+ | 管理员（创建 TUN）；**单 exe 分发**（内嵌 Wintun，首次连 TUN 释放 dll）；可选 Windows 服务 |
| Linux | root 或 `CAP_NET_ADMIN` |
| macOS | 管理员 |

---

## 3. 获取二进制

### 3.1 从 release 包（推荐）

```powershell
# Windows 开发者环境（pwsh 7 + Go 1.26）
# 1. 编辑 VERSION 设定版本号（仅开发者手动修改）
# 2. 全平台构建
.\scripts\build-release.ps1
```

```bash
# Linux / macOS
./scripts/build-release.sh
```

产物目录 `dist/`（**6 平台 × 2 二进制 = 12 个文件** + 6 个 zip + `manifest.json`）：

```
dist/
├── VERSION
├── manifest.json
├── HaoVPN-<ver>-linux-amd64.zip
├── linux-amd64/          # haovpn-server, haovpn-client
├── linux-arm64/
├── windows-amd64/        # haovpn-server.exe, haovpn-client.exe（内嵌 wintun，无单独 dll）
├── windows-arm64/
├── darwin-amd64/
└── darwin-arm64/
```

**Windows 说明**：release zip **不再**附带独立 `wintun.dll`；server 与 client 的 `.exe` 均在首次使用 TUN 时，将内嵌 DLL 释放到 **exe 所在目录**。拷贝到现场时只需对应平台的两个 exe（或其一）。

平台列表定义于 `scripts/platforms.txt`，可增删后重新构建。

仅构建部分平台：

```powershell
.\scripts\build-release.ps1 -Platform linux/amd64 -Platform windows/amd64
```

### 3.2 从源码构建

Windows 开发机构建前需准备 Wintun embed 源（`build-local` / `build-release` 会自动下载）：

```powershell
.\scripts\install-wintun.ps1
.\scripts\build-local.ps1
```

Linux / macOS 直接：

```bash
go build -o haovpn-server ./cmd/server
go build -o haovpn-client ./cmd/client
```

---

## 4. 服务端部署

### 4.1 Linux（systemd 推荐）

```bash
# 1. 创建目录
sudo mkdir -p /opt/HaoVPN/{data,logs,certs}
cd /opt/HaoVPN

# 2. 拷贝二进制
sudo cp haovpn-server /opt/HaoVPN/
sudo chmod +x /opt/HaoVPN/haovpn-server

# 3. 首次启动（生成 server.yaml / 证书 / 数据库）
sudo ./haovpn-server -c /opt/HaoVPN/server.yaml

# 4. 编辑配置后重启；浏览器打开 http://127.0.0.1:8080 强制修改 admin 密码
```

**systemd unit 示例**（`/etc/systemd/system/haovpn-server.service`）：

```ini
[Unit]
Description=HaoVPN Server
After=network.target

[Service]
Type=simple
WorkingDirectory=/opt/HaoVPN
ExecStart=/opt/HaoVPN/haovpn-server -c /opt/HaoVPN/server.yaml
Restart=on-failure
RestartSec=5

[Install]
WantedBy=multi-user.target
```

```bash
sudo systemctl daemon-reload
sudo systemctl enable --now haovpn-server
sudo systemctl status haovpn-server
```

### 4.2 Windows

```powershell
# 以管理员打开 PowerShell；目录须可写（首次启动会释放 wintun.dll 到 exe 同目录）
mkdir C:\HaoVPN
copy haovpn-server.exe C:\HaoVPN\
cd C:\HaoVPN
.\haovpn-server.exe -c .\server.yaml
# 首次运行后编辑 server.yaml，浏览器 http://127.0.0.1:8080 改密
```

> **Wintun**：Windows 服务端与客户端共用 `internal/tun`，**均内嵌** Wintun；无需单独拷贝 `wintun.dll` 安装包。若 exe 放在只读目录（如 `Program Files`），请用安装器写入 dll 或改到可写目录（如 `C:\HaoVPN`）。

可选：使用 `scripts/install-windows-service.ps1`（代码落地后）注册服务。

> **NAT 说明（Windows）**：专业版/已启用 Hyper-V 时优先 `New-NetNat`（WinNAT）。**Windows 家庭版**无 WinNAT（`0x80041010 Invalid class`），服务端会自动回退 **ICS**（Internet 连接共享）；live.log 应出现 `windows: ICS 已启用`。工控生产环境仍推荐 **Linux + iptables MASQUERADE**。

### 4.3 macOS

> **NAT 说明（v1.0）**：macOS 服务端在 `nat.enabled: true` 时**不会假装** pf SNAT 已配置，启动会失败并提示手工配置 pf。工控现场服务端请优先使用 **Linux** 或 **Windows（New-NetNat）**。若仅作客户端接入，可将 `nat.enabled: false`。

```bash
sudo mkdir -p /usr/local/HaoVPN
sudo cp haovpn-server /usr/local/HaoVPN/
cd /usr/local/HaoVPN
sudo ./haovpn-server -c ./server.yaml
```

**launchd 示例**（`/Library/LaunchDaemons/com.haovpn.server.plist`）：

```xml
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
  <key>Label</key>
  <string>com.haovpn.server</string>
  <key>ProgramArguments</key>
  <array>
    <string>/usr/local/HaoVPN/haovpn-server</string>
    <string>-c</string>
    <string>/usr/local/HaoVPN/server.yaml</string>
  </array>
  <key>WorkingDirectory</key>
  <string>/usr/local/HaoVPN</string>
  <key>RunAtLoad</key>
  <true/>
  <key>KeepAlive</key>
  <true/>
</dict>
</plist>
```

```bash
sudo chown root:wheel /Library/LaunchDaemons/com.haovpn.server.plist
sudo launchctl bootstrap system /Library/LaunchDaemons/com.haovpn.server.plist
sudo launchctl enable system/com.haovpn.server
```

launchd 配置历史见 `docs/dev-log.md`。

### 4.4 关键配置项（server.yaml）

首次启动自动生成，主要项：

| 配置 | 默认值 | 说明 |
|------|--------|------|
| `server.listen` | `0.0.0.0:8443` | 隧道监听（可走 frp） |
| `api.listen_hosts` | `["127.0.0.1"]` | 管理口绑定地址 |
| `api.allow_public_bind` | `false` | **危险**：绑 0.0.0.0 须显式 true |
| `api.port` | `8080` | 管理端口 |
| `vpn.subnet` | `10.88.0.0/24` | VPN 地址池 |
| `vpn.send_queue_size` | `256` | 服务端发送队列（帧条数）；大文件/电影可 `1024`；范围 64～8192，越界钳制 |
| `vpn.session_policy` | `reject_second` | 同账号第二端：拒绝（安全事件 `account_online`）/ `kick_previous` 踢旧 |
| `vpn.reconnect_grace_sec` | `60` | 同公网 IP 短窗顶替旧会话并续算 Rx/Tx；YAML `-1`→关闭 |
| `api.display_timezone` | `UTC` | **仅 WebUI 展示**时区（`Asia/Shanghai` / `GMT+8` / `+08:00`）；SQLite 与 API JSON 仍 UTC |
| `nat.allowed_lan_cidrs` | 工控网段 | 允许经**服务端 NAT**访问的现场网段（写入默认 AllowedIPs） |
| 客户端 `server.send_queue_size` | `256` | 客户端发送队列；与服务端可不同；上传大文件可加大 |
| `security.allow_all_vpn_peers` | `false` | 允许全部客户端互访对方 VPN IP；细粒度用控制台「托管路由」/白名单 |
| `security.probe_defense` | 见下表 | 公网探针识别、落库、温和自动封禁；详解与特征对照见 [security-hardening.md](security-hardening.md) |
| `security.allow_plaintext_private_keys` | `false` | 仅兼容旧库明文私钥；生产必须保持 false |
| `database.path` | `./data/haovpn.db` | SQLite 路径 |
| `database.audit_retention_days` | `90` | 审计日志保留天数 |
| `database.connection_events_retention_days` | `90` | 连接事件保留天数 |
| `log.history_retention_days` | `90` | 结构化历史日志（`logs.db`）；`-1` 关闭 |
| `log.history_db` | 同目录 `logs.db` | WebUI 分页检索用 |
| 客户端 `local_lans` | 未配/空 | **手动**填写本机后面的 LAN（如 `192.168.31.0/24`）；须为 **RFC1918** 且前缀 **≥ /16**（禁 `/0`～`/15` 与公网前缀）；非空则登录上报注册表并开 via 出口；空则整条关闭。勿写入账号 AllowedIPs；勿写 ICS `192.168.137.0/24` |

管理控制台：维护页读 live 日志；**探针**页（`/security`）；**托管路由**页（`/peers`：注册表 + Managed Routes + 互访）；滚动大文件仅读尾部块。

**四概念对照**：

| 配置/表 | 作用 |
|---------|------|
| 账号 AllowedIPs / `nat.allowed_lan_cidrs` | 经**服务端网关 NAT**访问工控网；客户端托盘「分流」栏展示 |
| 客户端 `local_lans` | 本机当 via 时广告并出口到家里/现场 LAN |
| `client_lan_registry` | 在线临时广告（断线清空）；alone 不转发 |
| `peer_routes` + `peer_route_members` | 管理员手工决定谁可走哪条 dest→via；客户端托盘「对端托管」栏 |

**via 出口现场步骤（家里共享 LAN）**：

1. 家里客户端 `client.yaml`（或 GUI「本地网段」）填写 `local_lans: ["192.168.31.0/24"]`，**管理员**运行连接。  
2. 日志应有 `lan_registry_reported`、`via_exit_setup ok`（失败看 WinNAT/ICS/提权）。  
3. 控制台 `/peers` → 注册表可见 →「创建托管路由」→ 选访问方 → **应用生效**。  
4. 公司客户端重连后 `ping` via 的 VPN IP 与 LAN 主机；回程依赖 via 侧 SNAT（握手下发 `vpn_subnet`）。

#### `security.probe_defense` 子项（摘要）

| 子项 | 默认 | 说明 |
|------|------|------|
| `enabled` | `true` | 自动记录/自动封；显式 `false` 不会被默认改回 |
| `record_events` | `true` | 写 `security_events` |
| `auto_ban` | `true` | 窗口达阈值写 `ip_blocks` |
| `ban_after_events` | `8` | 阈值 |
| `ban_window_sec` | `600` | 窗口秒 |
| `ban_duration_sec` | `3600` | 封禁秒；`0`=永久（整段未配置时才默认 3600） |
| `event_retention_days` | `30` | 事件保留天 |
| `ignore_signatures_for_ban` | `connection_reset` 等 | 不计入自动封（仍记事件）；默认含 `auth_failed` |

手动封禁与已有 `ip_blocks` **始终生效**，不依赖 `enabled`（服务端启动时有 Guard 即挂到 Accept）。完整说明与中英文对照见 [security-hardening.md](security-hardening.md)。

Web 自改密：`POST /api/v1/password` 须带 `old_password` 与 `new_password`；成功后吊销全部 Web 会话并重新登录。Web 与隧道登录锁定分表，互不影响。

完整示例见 [meta-plan.md](archive/meta-plan.md) YAML 章节。

### 4.5 frp 配置（现场无公网 IP）

现场 `frpc` 示例见 [scripts/frp-example.toml](../scripts/frp-example.toml)。

要点：

- 只映射 **8443**（隧道），**不要**映射 8080（管理口）。
- `haovpn-server` 的 `server.listen` 保持 `0.0.0.0:8443`，frpc 连本地 8443。

---

## 5. 客户端部署

### 5.1 获取配置包

1. 登录 WebUI → **账号** → 新建账号（默认 `fixed`；可选手动填写 VPN IP，留空则池自动分配）→ 下载 ZIP。
2. 「下载 ZIP」→ 得到 zip（含 `client.yaml` 凭证；**无** `vpn_ip`/`allowed_ips`，由握手下发）。
3. 改策略：账号页「策略」→ 可改 AllowedIPs / IP 模式 / **fixed 下的 VPN IP** → 保存后踢线重连。

### IP 模式说明

| 模式 | 开户 IP | 连接时 | 断线 |
|------|---------|--------|------|
| `fixed`（默认） | 可选手动指定，或留空自动分配 | 使用库内固定 IP | 不回收 |
| `dynamic_session` | 不可指定 | 握手从池分配 | 立即回池 |
| `dynamic_lease` | 不可指定 | 握手分配；租约内重连复用 | 写 `lease_until`，过期清理 |

公司机客户端：解压后改 `server.address`，运行新版 `haovpn-client.exe`（须支持无 `vpn_ip` 的 yaml）。

### 5.2 Windows

桌面 GUI（`haovpn-client-gui.exe`）**仅 Windows 发版包提供**；Linux/macOS 分发包以 CLI 为主。

#### Windows GUI 托盘「配置」与开机自启（已实现）

| 托盘项 | 行为 | 备注 |
|--------|------|------|
| 自动连接 | `gui.auto_connect`；须记住密码 | 会话内启动 GUI 后拨号 |
| 无窗口模式 | `gui.start_minimized`；仅托盘，可再唤起 | 关主窗=隐藏，不是退出 |
| 开机自启（登录后起本程序） | 计划任务 `HaoVPNClientGUI`，ONLOGON + Highest | 须用户登录（建议自动登录桌面）；免每次 UAC |
| 开机自启（服务，无托盘） | SCM 服务 `HaoVPNClient`（可用 GUI.exe 作入口） | 开机即连、无托盘；再开 GUI 可「接管」；与界面版互斥 |

```powershell
cd C:\haovpn-client
.\haovpn-client-gui.exe -c .\client.yaml   # 须管理员 / UAC
# 或 CLI：
.\haovpn-client.exe -c .\client.yaml
.\haovpn-client.exe --service install
.\haovpn-client.exe --service start
```

首次连接会在 **exe 同目录** 释放 `wintun.dll`（须可写）。GUI 可「记住密码」（明文写入 `client.yaml`，限制 ACL、勿提交 git）；杀开关在 yaml `security.kill_switch` 或托盘「配置」。断线自动重连时保留 TUN/via（未变则不重跑 ICS）；登出 / 手动重连仍全清。

### 5.3 Linux / macOS 客户端

发版以 **CLI** 为主（当前无正式 GUI 包）。日常：

```bash
chmod +x haovpn-client
sudo ./haovpn-client -c ./client.yaml
```

#### 开机自启：托盘可写（须 root / 用户目录）

与 Windows 相同两条能力，由 `internal/autostart` 实现：

| 能力 | Linux | macOS |
|------|-------|-------|
| 登录后起 GUI | `~/.config/autostart/haovpn-client-gui.desktop`（XDG） | `~/Library/LaunchAgents/com.haovpn.client.gui.plist` |
| 开机无界面 | `/etc/systemd/system/haovpn-client.service`（须 root；`ExecStart=… service`） | `/Library/LaunchDaemons/com.haovpn.client.plist`（须 root） |

托盘开关在 Linux/macOS 会真实写文件；无权限时返回明确中文错误（不再伪成功）。单元内容生成见 `autostart/gen.go`。

手工等价示例（Linux systemd，与 autostart 生成一致）：

```bash
# /etc/systemd/system/haovpn-client.service（路径按现场改）
[Unit]
Description=HaoVPN 客户端（开机无界面）
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
ExecStart=/opt/haovpn/haovpn-client-gui service
Restart=on-failure
RestartSec=5

[Install]
WantedBy=multi-user.target
```

```bash
sudo systemctl daemon-reload
sudo systemctl enable --now haovpn-client.service
```

macOS：LaunchDaemon 标签 `com.haovpn.client`，`ProgramArguments` 含 `service`；登录 GUI 用 LaunchAgent `com.haovpn.client.gui`。

---

## 6. 开发 / 内网联调

若需从**其他机器**访问本机 WebUI（仅开发测试）：

```yaml
api:
  listen_hosts: ["0.0.0.0"]
  allow_public_bind: true   # 必须显式开启，启动会有 WARN + 免责声明
```

生产交付前务必改回 `127.0.0.1` + `allow_public_bind: false`。详见 [security-hardening.md](security-hardening.md)。

本地快速自测可使用：

```bash
./scripts/dev-smoke-test.sh    # 代码落地后：起服务 + 健康检查
./scripts/dev-gen-certs.sh     # 手工生成测试证书
```

---

## 7. 测试验证步骤

### 7.1 部署后检查清单

| # | 检查项 | 命令 / 操作 | 期望 |
|---|--------|-------------|------|
| 1 | 进程运行 | `systemctl status` / 任务管理器 | Running |
| 2 | 配置已生成 | 存在 `server.yaml`、`data/haovpn.db` | 有 |
| 3 | 管理页本机可访问 | `curl http://127.0.0.1:8080/api/v1/health` | 200 |
| 4 | 管理页公网不可达 | 外网 curl 服务器公网 IP:8080 | 失败 |
| 5 | 已改 admin 密码 | WebUI 登录 | 非 changeme |
| 6 | `allow_public_bind` | 查看 server.yaml | `false` |

### 7.2 功能验收

| # | 场景 | 步骤 | 期望 |
|---|------|------|------|
| 1 | 开户 | WebUI 新建账号（一步） | 成功，有 vpn_ip |
| 2 | 客户端连接 | 导入 zip，启动 client | Dashboard 显示在线 |
| 3 | 工控访问 | 客户端 `ping 192.168.x.x` | 通 |
| 4 | 断线重连 | 断网 10s 后恢复 | ≤8s 重连 |
| 5 | 改策略踢线 | PATCH allowed_ips | 踢线 + 重连后新策略 |
| 6 | 禁用账号 | 禁用后 client 重连 | 握手失败 |
| 7 | 备份 | 下载 SQLite 备份 | 文件可打开 |

### 7.3 安全验收（必做）

见 [meta-plan.md](archive/meta-plan.md)「安全测试」9 项，或 [security-hardening.md](security-hardening.md)。

**meta-plan #3 网络隔离**（无 CI 自动化，须现场手工）：

1. 确认 `api.allow_public_bind: false`，`listen_hosts` 不含 `0.0.0.0`/`::`。
2. 从**非本机、非 VPN 网段**的机器执行：`curl -m 3 http://<服务端IP>:8080/api/v1/health`
3. 期望：**连接失败或超时**（管理口不可达）。

发版前授权核对见 [licensing.md](licensing.md)「发版前检查清单」。

快速脚本（代码落地后）：

```bash
./scripts/dev-security-check.sh
```

### 7.4 性能粗测（可选）

| 指标 | 目标 | 测量方式 |
|------|------|----------|
| 首次连接 | ≤10s | 客户端启动到 Dashboard 在线 |
| 重连 | ≤8s | 断网恢复计时 |
| 管理 API | ≤1s | `curl -w '%{time_total}'` health 接口 |

---

## 8. 升级与备份

### 8.1 备份

```bash
# WebUI：系统 → 下载数据库备份
# 或手动：
cp /opt/HaoVPN/data/haovpn.db /backup/HaoVPN-$(date +%F).db
```

建议同时备份 `server.yaml` 和 `certs/`。

### 8.2 升级

1. 停止服务。
2. 备份数据库与配置。
3. 替换二进制。
4. 启动服务，查看日志确认迁移成功。

**0.1.2 注意**：本小版本含握手/注册表/via、peer 应用生效与 Cookie/CSP 等管理面改动，**请同时更新服务端与客户端**（含 GUI）。仅升一端可能导致策略不同步或旧客户端行为差异。详见 [dev-log.md](dev-log.md) 第十七轮与 [architecture.md](architecture.md)。
### 8.3 回滚

1. 停止服务。
2. 恢复旧二进制 + 旧数据库备份。
3. 启动服务。

---

## 9. 脚本索引

| 脚本 | 用途 |
|------|------|
| [build-release.sh](../scripts/build-release.sh) | 交叉编译 release |
| [build-release.ps1](../scripts/build-release.ps1) | Windows 交叉编译 |
| [dev-gen-certs.sh](../scripts/dev-gen-certs.sh) | 开发用自签证书 |
| [dev-gen-certs.ps1](../scripts/dev-gen-certs.ps1) | Windows 生成证书 |
| [dev-smoke-test.sh](../scripts/dev-smoke-test.sh) | 本地冒烟测试 |
| [dev-smoke-test.ps1](../scripts/dev-smoke-test.ps1) | Windows 冒烟（build + E2E） |
| [dev-security-check.sh](../scripts/dev-security-check.sh) | 安全配置检查 |
| [dev-security-check.ps1](../scripts/dev-security-check.ps1) | Windows 安全配置检查 |
| [dev-full-test.ps1](../scripts/dev-full-test.ps1) | 全量验证（单元测试 + E2E） |
| [dev-e2e.ps1](../scripts/dev-e2e.ps1) | API 健康 E2E |
| [frp-example.toml](../scripts/frp-example.toml) | frp 参考配置 |

---

## 10. HaoVPN 首版迁移（自 MyVPN 旧名）

首版统一产品名为 **HaoVPN**，**不保留**旧 MyVPN 兼容：

| 旧 | 新 |
|----|-----|
| `myvpn-server.exe` 等 | `haovpn-server.exe` / `haovpn-client.exe` / `haovpn-client-gui.exe` |
| Go 模块 `go-vpn` | `haovpn` |
| TUN `myvpn0` | `haovpn0` |
| `myvpn.db` / `.myvpn-key` | `haovpn.db` / `.haovpn-key` |
| `%ProgramData%\MyVPN` | `%ProgramData%\HaoVPN` |
| 环境变量 `MYVPN_*` | `HAOVPN_USER` / `HAOVPN_PASSWORD` |

建议：清空或备份旧 `home/data`、重新生成证书（改 `cert_sans` 后删旧 crt/key 再启服务端）、公司机更新 `certs/server.crt` 与新客户端 zip。

---

## 11. 相关文档

- [security-hardening.md](security-hardening.md) — 生产加固
- [troubleshooting.md](troubleshooting.md) — 故障排障
- [meta-plan.md](archive/meta-plan.md) — 功能与验收演示脚本

---

*最后更新：2026-08-24*
