# HaoVPN

<p align="center">
  <img src="docs/assets/haovpn-logo.png" alt="HaoVPN" width="96">
</p>

面向工控现场与项目运维的自托管 VPN：工程师经 TLS-TCP 接入，策略由服务端握手下发，Web 控制台管理账号与审计。

许可见 [LICENSE](LICENSE)、[docs/licensing.md](docs/licensing.md)。

---

## 快速开始（服务端）

```powershell
.\scripts\build-local.ps1
mkdir home -Force
.\bin\haovpn-server.exe -c .\home\server.yaml
```

首次运行会生成带注释的 `server.yaml`、自签证书和 SQLite 库。默认：

| | |
|--|--|
| 隧道 | `:8443`（可走 frp） |
| 管理台 | `http://127.0.0.1:8080`（仅本机；TUN 起来后会加上 VPN 网卡 IP） |

用 `server.yaml` 里的 `admin` 登录后立刻改密，再在「账号」页开户并下载工程师 ZIP。完整部署见 [docs/deploy.md](docs/deploy.md)。

---

## 客户端

| 平台 | 怎么用 |
|------|--------|
| **Windows** | `haovpn-client-gui.exe`（推荐）或 `haovpn-client.exe` |
| **Linux / macOS** | 发行包目前以 **CLI** 为主：`haovpn-client -c client.yaml`（一般需 root / 相应权限） |

连接后 `vpn_ip`、`allowed_ips` 由握手下发，不必手写 peer。

### Windows GUI 托盘「配置」（已实现）

| 选项 | 作用 |
|------|------|
| 自动连接 | 启动后自动拨号（须「记住密码」） |
| 无窗口模式 | 只留托盘，可再「显示主窗口」 |
| 开机自启（登录后） | 计划任务，登录后最高权限拉起本 GUI（免每次 UAC） |
| 开机自启（服务） | Windows 服务，开机即连、**无托盘**；再开 GUI 可选择接管 |

工控有桌面时常用：记住密码 + 自动连接 + 无窗口 + 登录后自启（机器最好自动登录桌面）。

### Linux / macOS 开机自启（托盘未接）

概念上仍是「登录后有界面」和「开机无界面服务」两条路，但 **GUI 托盘里那两个开关尚未实现**（点了会提示去配系统单元）。请手工：

- 无界面常驻：`systemd` / `launchd` 跑 `haovpn-client`（示例见 [docs/deploy.md](docs/deploy.md) 服务端同类写法，客户端同理）
- 有桌面再谈 GUI 自启（当前发版 GUI 以 Windows 为主）

Windows 也可用 CLI：`haovpn-client.exe --service install`。

---

## 典型拓扑

```
工程师 ──TLS:8443──► frp/VPS ──► 现场 haovpn-server ──► 工控网
                         │
              管理台 :8080 仅本机/VPN（不要映射到公网）
```

上线前核对 [docs/security-hardening.md](docs/security-hardening.md)：`api.allow_public_bind` 保持 `false`，管理口勿经 frp 暴露。

可选配置：`vpn.send_queue_size`（大文件可调大）、`api.display_timezone`（仅 WebUI 展示时区，库内仍 UTC）。

---

## 构建

| | 命令 | 产物 |
|--|------|------|
| 本机 | `.\scripts\build-local.ps1` | `bin/` |
| 发版 | `.\scripts\build-release.ps1` | `dist/`（含 LICENSE、NOTICE） |

版本只改根目录 [VERSION](VERSION)。开发环境约定：Windows + PowerShell 7 + Go 1.26。

---

## 文档去哪看

| 文档 | 用途 |
|------|------|
| [记忆.md](记忆.md) | 接手顺序与当前进度 |
| [docs/deploy.md](docs/deploy.md) | 部署、客户端自启、验收 |
| [docs/troubleshooting.md](docs/troubleshooting.md) | 排障 |
| [docs/architecture.md](docs/architecture.md) | 包边界 |
| [internal/README.md](internal/README.md) | 改功能找哪个包 |
| [docs/dev-log.md](docs/dev-log.md) | 开发记录 |
