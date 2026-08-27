# HaoVPN

面向**工控现场 / 项目运维**的自托管 VPN：TLS-TCP 穿透防火墙，WireGuard 级加密，WebUI 管账号与连接，SQLite 持久化，YAML 配置开箱即用。

**设计优先级**：安全 > 简单 > 快

> 仓库目录名可能仍为 `go-vpn`（本地路径），Go 模块名为 **`haovpn`**。

## 环境要求（开发）

| 项目 | 版本 |
|------|------|
| 操作系统 | Windows（主开发环境） |
| PowerShell | **7+**（`pwsh`） |
| Go | **1.26** |
| Git | 仅开发者本人提交 |

## 快速开始

```powershell
.\scripts\build-local.ps1

# 服务端
.\bin\haovpn-server.exe -c .\home\server.yaml

# 客户端 CLI
.\bin\haovpn-client.exe -c client.yaml

# 客户端 GUI（推荐）
.\bin\haovpn-client-gui.exe
```

- 管理页面默认：`http://127.0.0.1:8080`
- 隧道端口默认：`8443`

进度与变更见 [docs/dev-log.md](docs/dev-log.md)。

## 版本

版本号由根目录 **[VERSION](VERSION)** 统一管理，**仅开发者手动修改**。

```powershell
.\scripts\build-release.ps1
```

详见 [docs/versioning.md](docs/versioning.md)。

## 文档地图

| 文档 | 说明 |
|------|------|
| **[记忆.md](记忆.md)** | 接手顺序与当前进度表 |
| [docs/README.md](docs/README.md) | docs 目录索引 |
| [docs/development-principles.md](docs/development-principles.md) | 开发原则 |
| [docs/deploy.md](docs/deploy.md) | 部署、配置、验收 |
| [docs/troubleshooting.md](docs/troubleshooting.md) | 故障排障（现象→处理） |
| [docs/dev-log.md](docs/dev-log.md) | 开发日志（唯一进度来源） |
| [scripts/README.md](scripts/README.md) | 构建与测试脚本 |

规划存档：[docs/meta-plan.md](docs/meta-plan.md)（进度以 dev-log 为准）。

## 构建平台

`build-release` 默认产出 6 平台 × `haovpn-server` + `haovpn-client`（Windows 另有 `haovpn-client-gui`），见 `scripts/platforms.txt`。

## 技术栈

Go 1.26 · TUN/Wintun/utun · wireguard-go 密码学 · TLS-TCP · SQLite · YAML

## 许可证

待定
