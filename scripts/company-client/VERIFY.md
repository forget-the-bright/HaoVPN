# 公司电脑 · HaoVPN 客户端验证说明

> 本包只含**客户端**。服务端在家里跑；公司机经 ZeroTier / 公网 / frp 连回家。
>
> **先看 [`TEST-ACCOUNT.md`](TEST-ACCOUNT.md)**（账号密码）→ 按下面步骤测 → 测完填 [`RESULT-TEMPLATE.md`](RESULT-TEMPLATE.md) 并跑收集脚本。

---

## 本包测试账号（抄这里）

| 项 | 值 |
|----|-----|
| 用户名 | **`company_test`** |
| 密码 | **`CompanyTest@2026`** |
| 服务器地址（默认） | **`192.168.196.17:8443`**（家里 ZeroTier；以 `client.yaml` 为准） |

详情与注意见 `TEST-ACCOUNT.md`。

---

## 0. 家里须已就绪（你出门前）

1. 服务端已启动：`.\scripts\run-server.ps1`
2. 账号 `company_test` 已存在（打包脚本会尝试自动创建）
3. 公司机能路由到 `client.yaml` 里的 `server.address`（ZeroTier 须两端在线）

---

## 1. 公司电脑操作

### 1.1 解压

例如解压到：

```text
C:\haovpn-company-test\
```

应看到：

```text
VERIFY.md                 ← 本说明
TEST-ACCOUNT.md           ← 账号密码
RESULT-TEMPLATE.md        ← 结果回填（测完填）
collect-client-info.ps1   ← 一键收集诊断
PACK-INFO.txt
client.yaml               ← 已预填 address + auth.username
bin\haovpn-client.exe          ← wintun 已内嵌，首次连 TUN 会在 bin\ 释放 wintun.dll
bin\haovpn-client-gui.exe      ← 推荐
certs\server.crt
logs\
```

### 1.2 确认配置

打开 `client.yaml`，确认类似：

```yaml
server:
  address: "192.168.196.17:8443"   # 若家里 ZT IP 变了再改
  tls:
    ca_file: "./certs/server.crt"
    insecure_skip_verify: false
  heartbeat_interval_sec: 15
  heartbeat_timeout_sec: 90

auth:
  username: "company_test"
  # 密码不要写在这里 → GUI 输入或 HAOVPN_PASSWORD
```

### 1.3 启动 GUI（推荐）

可直接双击 `bin\haovpn-client-gui.exe`（会自动找 **exe 同目录** 的 `client.yaml`；本包已把配置放到解压根时请用下面命令或把 `client.yaml` 拷到 `bin\`）。

非管理员会弹出 **UAC**；点「是」后以管理员继续。点「否」则登录窗提示须管理员（TUN/路由/杀开关）。

```powershell
cd C:\haovpn-company-test
# 显式指定配置（推荐）
.\bin\haovpn-client-gui.exe -c .\client.yaml
# 或把 client.yaml 放到 bin\ 后双击 haovpn-client-gui.exe
```

登录窗填写：

- 服务器：`192.168.196.17:8443`（或你改过的地址）
- 账号：`company_test`
- 密码：`CompanyTest@2026`
- 确认底部「配置: …」路径正确

点「连接」。可选勾选「断线阻断工控网段（杀开关）」。

**重连体感**：底层 ZeroTier 抖动恢复后，新版应在数秒内重拨（`dial_timeout_sec`/`reconnect.max_sec` 默认约 3s，断线立即重试）。若 `ping` ZT IP 仍超时，属底层问题，不是 VPN「握手慢」。

盯日志（另开窗口）：

```powershell
Get-Content .\logs\client.live.log -Wait -Tail 50
```

### 1.4 期望日志

- `隧道握手成功 vpn_ip=... gateway=...`
- `已应用服务端策略 ...`
- `客户端路由已添加: ... on-link`
- （若家里配了 DNS）`dns_applied ...`

### 1.5 连通性

```powershell
ping 10.88.0.1 -t
```

隧道存活时应有回复（ZeroTier 抖动可间歇）。家里 WebUI 应看到 `company_test` 在线。

网关通、工控不通：常见于家里 `nat_ok=false`（Win11 家庭版），与客户端无关。

---

## 2. 测完必须收集什么（带回 / 发给家里）

### 2.1 填结果

打开并填写 **`RESULT-TEMPLATE.md`**（勾选 + 失败原文）。

### 2.2 一键打包诊断

```powershell
cd C:\haovpn-company-test
pwsh -File .\collect-client-info.ps1
```

生成 **`diag-YYYYMMDD-HHMMSS.zip`**（内含 yaml、日志、系统摘要、`ping`）。

### 2.3 带回清单（缺一不可）

| # | 带回什么 | 说明 |
|---|----------|------|
| 1 | `diag-*.zip` | 脚本产物，整包 |
| 2 | 已填写的 `RESULT-TEMPLATE.md` | 可放进 diag 旁，或再拷一份 |

可选：截图 `ping 10.88.0.1`、家里 Dashboard「在线」。

**不要**带回明文密码；`client.yaml` 里也不应有密码。

---

## 3. 常见失败

| 现象 | 可能原因 |
|------|----------|
| `certificate is valid for 127.0.0.1, not …` | 家里证书 SAN 无该 IP：改 `cert_sans` 后删证重生并更新公司机 `certs\server.crt` |
| `用户名或密码错误` | 密码输错；或家里未建 `company_test` |
| `须先修改密码后再连接 VPN` | 账号仍 must_change；家里 Web 改密 |
| `登录失败次数过多，请稍后再试` | IP 锁定，稍后再试 |
| 握手超时 / dial 失败 | `server.address` 错、ZT 未通、公司防火墙拦 8443 |
| TUN 创建失败 | 未管理员运行；exe 目录不可写（无法释放内嵌 wintun.dll） |
| 杀开关启用失败 | 未管理员；失败时状态行有提示且不清路由 |
| 服务端刷 `丢弃非 IPv4` / `ver=6` | Windows IPv6 探测；已降 DEBUG；新客户端禁 TUN IPv6，可忽略 |
| 握手成功但 ping 间歇 / 频繁 reconnect | **先 ping 底层** `192.168.196.17`：若 ZT 也超时，属 ZeroTier 抖动，隧道只能跟着断 |
| ping 通网关不通 LAN | 家里 NAT/ICS（家庭版） |

---

## 4. 安全

- 密码只在 GUI / 环境变量输入，**不要**写入 `client.yaml`
- 测完可在家里禁用 `company_test`
- 本包含测试密码明文文档：仅限你自己的联调，勿外传
