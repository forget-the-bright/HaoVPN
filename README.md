# HaoVPN

<p align="center">
  <img src="web/static/logo.png" alt="HaoVPN" width="96">
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

### Linux / macOS 开机自启

与 Windows 相同两条能力，由 `internal/autostart` 实现（托盘开关可写；无权限时中文报错，不伪成功）：

| 能力 | Linux | macOS |
|------|-------|-------|
| 登录后起 GUI | XDG `~/.config/autostart/*.desktop` | LaunchAgent `~/Library/LaunchAgents` |
| 开机无界面服务 | systemd（须 root） | LaunchDaemon（须 root） |

手工配 unit 亦可；`ExecStart` 须带 `service` 参数。示例见 [docs/deploy.md](docs/deploy.md)。发版 GUI 仍以 Windows 为主；Linux/macOS 分发包以 CLI 为主。

Windows 也可用 CLI：`haovpn-client.exe --service install`。

管理台：页面脚本在 `web/static/*.js`，CSP `script-src 'self'`（样式仍可内联）。

---

## 典型拓扑

```
工程师 ──TLS:8443──► frp/VPS ──► 现场 haovpn-server ──► 工控网
                         │
              管理台 :8080 仅本机/VPN（不要映射到公网）
```

上线前核对 [docs/security-hardening.md](docs/security-hardening.md)：`api.allow_public_bind` 保持 `false`，管理口勿经 frp 暴露。

可选：`vpn.send_queue_size`（大文件可调大）、`api.display_timezone`（仅 WebUI 展示时区，库内仍 UTC）。

---

## 构建

| | 命令 | 产物 |
|--|------|------|
| 本机 | `.\scripts\build-local.ps1` | `bin/` |
| 发版 | `.\scripts\build-release.ps1` | `dist/`（含 LICENSE、NOTICE） |

版本只改根目录 [VERSION](VERSION)。开发环境：Windows + PowerShell 7 + Go 1.26。

---

## 文档去哪看

| 文档 | 用途 |
|------|------|
| [记忆.md](记忆.md) | **接手入口**（阅读顺序 + 当前阶段） |
| [docs/README.md](docs/README.md) | 文档总索引 |
| [docs/architecture.md](docs/architecture.md) | 包边界 / CODEMAP |
| [internal/README.md](internal/README.md) | 改功能找哪个包 |
| [docs/deploy.md](docs/deploy.md) | 部署、自启、验收 |
| [docs/troubleshooting.md](docs/troubleshooting.md) | 排障 |
| [docs/dev-log.md](docs/dev-log.md) | 开发进度（唯一） |
| [docs/release-notes-0.1.2-DRAFT.md](docs/release-notes-0.1.2-DRAFT.md) | 0.1.2 发版说明草稿 |
