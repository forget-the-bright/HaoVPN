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
├── HaoVPN-0.1.0-dev-linux-amd64.zip
├── linux-amd64/
│   ├── haovpn-server
│   └── haovpn-client
├── linux-arm64/
...
```

`manifest.json` 含 version、commit、buildTime、各产物路径。

---

## 开发 / 测试工具

| 脚本 | 说明 |
|------|------|
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

`build-local` 会先调用 `install-wintun.ps1` 下载 Wintun 并 **go:embed 进 Windows 客户端**（单 exe 分发；首次连 TUN 时释放到 exe 同目录）。

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
- `go.mod` 与 `cmd/server`、`cmd/client` 已初始化（代码落地后）

---

## 注意事项

- **不要**在脚本或代码里写死版本号；只读 `VERSION` 文件。
- 新增/删减目标平台：编辑 `platforms.txt`，两个 release 脚本自动同步。
- bash 脚本的 zip 步骤依赖 `zip` 命令（Git Bash / Linux / macOS 通常自带）。
