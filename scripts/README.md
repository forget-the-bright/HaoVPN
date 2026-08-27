# 脚本说明

主开发环境：**Windows + PowerShell 7 + Go 1.26**。日常开发优先用 **PowerShell 脚本**。

版本号统一来自根目录 **[VERSION](../VERSION)**（**仅开发者可改，AI 禁止修改**）。详见 [docs/versioning.md](../docs/versioning.md)。

---

## 平台矩阵（默认全量构建）

定义于 [platforms.txt](platforms.txt)：

| GOOS | GOARCH | 说明 |
|------|--------|------|
| linux | amd64 | 现场服务器 / VPS x64 |
| linux | arm64 | ARM 服务器、树莓派等 |
| windows | amd64 | 工程师 PC（常见） |
| windows | arm64 | ARM Windows 设备 |
| darwin | amd64 | Intel Mac |
| darwin | arm64 | Apple Silicon (M 系列) |

每个平台产出：`haovpn-server` + `haovpn-client`（Windows 带 `.exe`），共 **12 个二进制**。

本机 `build-local` 额外构建 **`haovpn-client-gui.exe`**（仅 Windows，未纳入 `build-release` 全平台矩阵）。

---

## Windows Wintun（内嵌单 exe）

Windows 上 **服务端与客户端** 都通过 `internal/tun` 创建 Wintun 网卡，因此 **都会** 在构建时 `go:embed` 对应架构的 `wintun.dll`：

| 二进制 | 内嵌 Wintun |
|--------|-------------|
| `haovpn-server.exe` | ✅ |
| `haovpn-client.exe` | ✅ |
| `haovpn-client-gui.exe` | ✅（仅 `build-local`） |

| 平台二进制 | Wintun |
|------------|--------|
| Linux / macOS server、client | 不使用 wintun.dll |

**运行时**：首次创建 TUN 时，将内嵌 DLL **释放到 exe 同目录**的 `wintun.dll`（与 WireGuard 官方加载方式一致）。分发 zip **不必**再带单独 dll 文件。

**构建时**（仅 Windows 开发机需要）：

```powershell
# 下载 Wintun 0.14.1 到 internal/tun/wintundll/{amd64,arm64}/（go:embed 源，已 gitignore）
.\scripts\install-wintun.ps1              # 默认 amd64+arm64
.\scripts\install-wintun.ps1 -Arch amd64  # 仅本机构建
```

`build-local` / `build-release` 会在编译前自动调用；**不可**在未下载 embed 源的情况下交叉编译 Windows 目标。

**现场注意**：exe 所在目录须**可写**（Program Files 等只读目录需安装器同时写入 dll，或改放到可写目录）。

---

## 发布打包

| 脚本 | 环境 | 说明 |
|------|------|------|
| **[build-release.ps1](build-release.ps1)** | **Windows pwsh7（推荐）** | 全平台交叉编译 → `dist/` |
| [build-release.sh](build-release.sh) | Linux / macOS / Git Bash | 同上 |
| [build-local.ps1](build-local.ps1) | Windows | 本机快速构建 → `bin/` |

### Windows 发版（开发者）

```powershell
# 1. 手动编辑 VERSION（AI 不得修改）
notepad VERSION

# 2. 全平台 release（6 平台 × 2 = 12 二进制 + 6 个 zip）
.\scripts\build-release.ps1

# 仅构建部分平台
.\scripts\build-release.ps1 -Platform linux/amd64 -Platform windows/amd64

# 只构建 server
.\scripts\build-release.ps1 -ServerOnly

# 不打包 zip
.\scripts\build-release.ps1 -NoZip
```

### 本机日常开发

```powershell
.\scripts\build-local.ps1              # windows/amd64 → bin/
.\scripts\build-local.ps1 -Arch arm64  # windows/arm64
```

### Linux / macOS

```bash
chmod +x scripts/*.sh
./scripts/build-release.sh
./scripts/build-release.sh -p linux/arm64 -p darwin/arm64
./scripts/build-release.sh --server-only
```

### 产物结构

```
dist/
├── VERSION
├── manifest.json
├── HaoVPN-<ver>-linux-amd64.zip
├── HaoVPN-<ver>-windows-amd64.zip
├── linux-amd64/
│   ├── haovpn-server
│   └── haovpn-client
├── windows-amd64/
│   ├── haovpn-server.exe    # 内嵌 wintun（首次启动 TUN 时释放 dll）
│   └── haovpn-client.exe
├── windows-arm64/
│   └── …
└── …（其余 4 个平台目录 + zip）
```

- 每个 zip 内为对应平台目录下的 **server + client 两个二进制**（无单独 `wintun.dll`）。
- `manifest.json` 含 version、commit、buildTime、各产物路径。

### 现场 / 公司测试打包

| 脚本 | 产物 |
|------|------|
| [pack-company-client-test.ps1](pack-company-client-test.ps1) | `dist/company-client-test/` + `dist/haovpn-company-client-test-<时间戳>.zip`（client + GUI + yaml + 证书 + VERIFY） |
| [pack-zt-field-test.ps1](pack-zt-field-test.ps1) | `dist/zt-field-test/` + `dist/HaoVPN-zt-field-test.zip`（本机 server/client + home 模板） |

公司包 **bin/** 下仅 exe，不含独立 wintun.dll。

---

## 开发 / 测试工具

| 脚本 | 说明 |
|------|------|
| [install-wintun.ps1](install-wintun.ps1) | 下载 Wintun → `internal/tun/wintundll/`（构建 embed 用，见上文） |
| [dev-gen-certs.ps1](dev-gen-certs.ps1) / [.sh](dev-gen-certs.sh) | 开发用自签证书 → `./certs/` |
| [dev-smoke-test.sh](dev-smoke-test.sh) | 冒烟：启动 server + health 检查 |
| [dev-smoke-test.ps1](dev-smoke-test.ps1) | Windows：build-local + dev-e2e |
| [dev-security-check.sh](dev-security-check.sh) | 检查 server.yaml 安全配置 |
| [dev-security-check.ps1](dev-security-check.ps1) | Windows 安全配置检查 |
| [dev-full-test.ps1](dev-full-test.ps1) | 全量验证（go test + E2E + 安全检查） |
| [dev-acceptance.ps1](dev-acceptance.ps1) | **smoke**（无管理员，`require_tun=false`）— 通过 ≠ 可交付 |
| [dev-field-gate.ps1](dev-field-gate.ps1) | **field 硬门禁**（TUN+NAT+PLC+服务，须 `-PlcHost`） |

```powershell
# smoke（开发日常）
.\scripts\dev-acceptance.ps1

# v1.0 交付硬门禁（须管理员 pwsh 7 + 批准 UAC + 可达 PLC）
.\scripts\dev-field-gate.ps1 -PlcHost 192.168.1.10 -UseHomeConfig
```

> **field 注意**：非管理员 shell 中 `sudo` 若未获 UAC 批准，`haovpn-server` 不会启动；脚本会报 `sudo/UAC 未批准`。请「以管理员身份运行」pwsh，或在弹窗中批准 UAC。

### E2E 冒烟（本机）

```powershell
.\scripts\dev-e2e.ps1              # API 健康检查（无需 sudo）
.\scripts\dev-e2e.ps1 -WithSudo      # sudo 启动（验证 TUN，本机已装 sudo）
sudo .\bin\haovpn-server.exe        # 完整服务端 + WebUI
```

`build-local` 会先 `install-wintun.ps1` 再编译，Windows 客户端/服务端 exe 均内嵌 Wintun。

---

## 参考配置

| 文件 | 说明 |
|------|------|
| [frp-example.toml](frp-example.toml) | frp 反向隧道示例 |
| [platforms.txt](platforms.txt) | 交叉编译目标列表（增删平台改此文件） |
| [lib/build-common.ps1](lib/build-common.ps1) | PS 构建公共函数（读 VERSION、注入 ldflags） |

---

## 前置条件

- **Go 1.26**（`go version` 确认）
- **PowerShell 7+**（`$PSVersionTable.PSVersion`）
- 交叉编译默认 `CGO_ENABLED=0`（纯 Go，免 CGO 工具链）
- **Windows 构建**：需能访问 [wintun.net](https://www.wintun.net/) 下载 embed 源（或本地已有 `internal/tun/wintundll/*/wintun.dll`）
- `go.mod` 与 `cmd/server`、`cmd/client` 已初始化

---

## 注意事项

- **不要**在脚本或代码里写死版本号；只读 `VERSION` 文件。
- 新增/删减目标平台：编辑 `platforms.txt`，两个 release 脚本自动同步。
- bash 脚本的 zip 步骤依赖 `zip` 命令（Git Bash / Linux / macOS 通常自带）。
