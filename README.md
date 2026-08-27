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

<img width="1763" height="784" alt="cb73137c2081732be81e5532aba43324" src="https://github.com/user-attachments/assets/47d97a5a-3302-4f29-b2ac-cededca1f132" />
<img width="1920" height="911" alt="5b58e6664ab2dbe9fbe8ab8cd0e6d983" src="https://github.com/user-attachments/assets/695ff1b1-fabc-477a-ae24-c4ae0c9ca1e7" />
<img width="1920" height="911" alt="3aa7b55047cbb9c92f70eccf75ab517e" src="https://github.com/user-attachments/assets/1ba9ac62-6ff7-4912-b301-0d4cdf1c4bad" />

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

## 构建与发版

| 场景 | 命令 | 产物 |
|------|------|------|
| 本机日常 | `.\scripts\build-local.ps1` | `bin/`（当前 Windows 架构） |
| 全平台发版 | `.\scripts\build-release.ps1` | `dist/`（6 平台 zip + manifest） |
| 公司机测试包 | `.\scripts\pack-company-client-test.ps1` | `dist/haovpn-company-client-test-*.zip` |

发版前由开发者手动改 [VERSION](VERSION)，详见 [docs/versioning.md](docs/versioning.md)。

### `dist/` 目录（`build-release`）

```
dist/
├── VERSION
├── manifest.json
├── HaoVPN-<ver>-linux-amd64.zip
├── linux-amd64/
│   ├── haovpn-server
│   └── haovpn-client
├── windows-amd64/
│   ├── haovpn-server.exe      # 内嵌 wintun，见下
│   ├── haovpn-client.exe
│   └── haovpn-client-gui.exe  # 仅 Windows zip
└── …（linux-arm64、windows-arm64、darwin-amd64、darwin-arm64）
```

- 默认 **6 平台 × 2 二进制 = 12 个文件** + 6 个 zip + `manifest.json`（平台列表见 `scripts/platforms.txt`）。
- **Windows zip**（`windows-amd64`）含 **`haovpn-client-gui.exe`**；`windows-arm64` zip 在本机 amd64 构建时不含 GUI（Fyne CGO 须同架构），ARM64 本机构建 release 时会带上。
- **Windows zip 内不再单独附带 `wintun.dll`**：已 `go:embed` 进 exe，首次创建 TUN 时释放到 exe 同目录。

### Windows 与 Wintun（单 exe 分发）

| 二进制 | 是否内嵌 Wintun | 说明 |
|--------|-----------------|------|
| `haovpn-client.exe` / `haovpn-client-gui.exe` | ✅ | 工程师 PC 拨号 |
| `haovpn-server.exe`（Windows） | ✅ | 服务端也要建 TUN/NAT，走同一套 `internal/tun` |
| Linux / macOS 任意二进制 | — | 使用系统 TUN/utun，无 wintun.dll |

- **分发**：拷贝单个 `.exe` 即可；**首次**连 TUN / 启动带 TUN 的服务端时，会在 **exe 所在目录** 写出 `wintun.dll`（目录须可写）。
- **构建机**：首次或升级 Wintun 前运行 `.\scripts\install-wintun.ps1`（`build-local` / `build-release` 会自动调用）；DLL 源文件在 `internal/tun/wintundll/`（已 gitignore，不入库）。

脚本细节见 [scripts/README.md](scripts/README.md)；部署见 [docs/deploy.md](docs/deploy.md)。

## 文档地图

| 文档 | 说明 |
|------|------|
| **[记忆.md](记忆.md)** | 接手顺序与当前进度表 |
| [docs/README.md](docs/README.md) | docs 目录索引 |
| [docs/development-principles.md](docs/development-principles.md) | 开发原则 |
| [docs/architecture.md](docs/architecture.md) | **CODEMAP**（包导航、依赖、HTTP 路由表） |
| [docs/comment-style.md](docs/comment-style.md) | **注释规范** |
| [cmd/README.md](cmd/README.md) | 三个二进制入口 |
| [docs/deploy.md](docs/deploy.md) | 部署、配置、验收 |
| [docs/troubleshooting.md](docs/troubleshooting.md) | 故障排障（现象→处理） |
| [docs/dev-log.md](docs/dev-log.md) | 开发日志（唯一进度来源） |
| [scripts/README.md](scripts/README.md) | 构建与测试脚本 |

规划存档：[docs/meta-plan.md](docs/meta-plan.md)（进度以 dev-log 为准）。

## 技术栈

Go 1.26 · TUN/Wintun/utun · wireguard-go 密码学 · TLS-TCP · SQLite · YAML

## 许可证

待定
