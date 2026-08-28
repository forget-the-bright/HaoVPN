# HaoVPN

面向**工控现场 / 项目运维**的自托管 VPN：TLS-TCP 穿透防火墙，WireGuard 级加密，WebUI 管账号与连接，SQLite 持久化，YAML 配置开箱即用。

**设计优先级**：安全 > 简单 > 快

> 仓库目录名可能仍为 `go-vpn`（本地路径），Go 模块名为 **`haovpn`**。

---

## 许可证与商用

- 个人/内网非商用：**可免费**使用官方源码与二进制。
- **禁止**未经许可修改源码、做衍生项目或再分发。
- **商用**须联系作者获取书面付费授权。详见 [LICENSE](LICENSE) 与 [docs/licensing.md](docs/licensing.md)。

---

## 5 分钟部署（服务端）

### 1. 获取二进制

从 [dist/](dist/) 发版包解压，或本机构建：

```powershell
.\scripts\build-local.ps1
```

产物在 `bin/haovpn-server.exe`（Windows）或对应平台二进制。

### 2. 首次启动

```powershell
mkdir home -Force
.\bin\haovpn-server.exe -c .\home\server.yaml
```

首次运行会自动生成：

- `server.yaml`（带中文注释）
- `./certs/` 自签 TLS 证书
- `./data/haovpn.db` SQLite 数据库

默认管理口：`http://127.0.0.1:8080`（仅本机；TUN 启动后会追加 VPN 网卡 IP）  
隧道端口：`8443`

### 3. 首次登录改密

1. 浏览器打开 `http://127.0.0.1:8080`
2. 默认账号 `admin` / 初始密码见 `server.yaml` 中 `admin.password`（模板默认 `changeme12`，**务必立即修改**）
3. 按提示修改管理员密码

### 4. 开户并导出工程师配置

1. WebUI → **账号** → 新建账号（自动生成密钥与 VPN IP）
2. 点击 **下载 ZIP**，发给现场工程师
3. 工程师解压后运行客户端（见下节）

详细拓扑、frp、Linux systemd 见 [docs/deploy.md](docs/deploy.md)。

---

## 工程师接入（客户端）

### GUI（Windows 推荐）

```powershell
.\haovpn-client-gui.exe
```

导入 ZIP 或填写 `client.yaml` 后连接。策略（`vpn_ip` / `allowed_ips`）由**握手下发**，无需手改。

### CLI

```powershell
.\haovpn-client.exe -c client.yaml
```

### Windows 服务（开机自连）

```powershell
.\haovpn-client.exe --service install
.\haovpn-client.exe --service start
```

---

## 典型现场拓扑（frp）

```
工程师 PC ──TLS:8443──► VPS/frp ──► 现场 haovpn-server ──► 工控网 PLC
                              │
管理页仅本机/VPN内 :8080 ◄────┘（不要经 frp 暴露管理口）
```

- **8443**：隧道，可走 frp 反向映射  
- **8080**：管理 API/WebUI，默认不暴露公网（`allow_public_bind: false`）

---

## 配置要点

| 项 | 生产建议 |
|----|----------|
| `api.allow_public_bind` | `false` |
| `api.listen_hosts` | `["127.0.0.1"]` + TUN IP |
| `api.trusted_proxy_cidrs` | 留空（防 XFF 绕过登录锁定）；反代后填反代 IP |
| `security.enforce_split_tunnel` | `true` |
| `admin.password` | 首次启动后立即修改 |
| TLS 证书 | 生产替换自签证书 |

上线前核对 [docs/security-hardening.md](docs/security-hardening.md)。

---

## 环境要求

| 角色 | 要求 |
|------|------|
| 服务端 | Linux/Windows/macOS；TUN 权限（Windows 管理员 / Linux CAP_NET_ADMIN） |
| 客户端 | 同上；Windows 单 exe 内嵌 Wintun |
| 开发 | Windows + **pwsh 7+** + **Go 1.26** |

---

## 构建与发版

| 场景 | 命令 | 产物 |
|------|------|------|
| 本机日常 | `.\scripts\build-local.ps1` | `bin/` |
| 全平台发版 | `.\scripts\build-release.ps1` | `dist/`（含 LICENSE、NOTICE） |

发版前请核对 [docs/licensing.md](docs/licensing.md) 与 [docs/security-hardening.md](docs/security-hardening.md) §9（LICENSE/NOTICE 随包分发、联系邮箱已填写）。

发版前由开发者手动改 [VERSION](VERSION)，详见 [docs/versioning.md](docs/versioning.md)。

### Windows 与 Wintun

- 客户端/服务端 Windows 二进制内嵌 Wintun；首次建 TUN 时在 exe 同目录释放 `wintun.dll`（目录须可写）。
- 构建前运行 `.\scripts\install-wintun.ps1`（`build-local` 会自动调用）。

---

## 文档地图

| 文档 | 说明 |
|------|------|
| **[记忆.md](记忆.md)** | 接手顺序与进度 |
| [docs/deploy.md](docs/deploy.md) | **完整部署与验收** |
| [docs/architecture.md](docs/architecture.md) | CODEMAP |
| [docs/licensing.md](docs/licensing.md) | 商用授权说明 |
| [docs/troubleshooting.md](docs/troubleshooting.md) | 故障排障 |
| [docs/dev-log.md](docs/dev-log.md) | 开发日志 |
| [internal/README.md](internal/README.md) | 包索引 |

---

## 技术栈

Go 1.26 · TUN/Wintun/utun · wireguard-go 密码学 · TLS-TCP · SQLite · YAML

---

## 故障与支持

常见问题见 [docs/troubleshooting.md](docs/troubleshooting.md)。  
进度与变更见 [docs/dev-log.md](docs/dev-log.md)。
