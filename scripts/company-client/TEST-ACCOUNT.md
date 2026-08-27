# 公司网测试账号（本包专用）

> 仅用于家里服务端 ↔ 公司电脑联调。测完可在家里 WebUI **禁用/删除**该账号。

| 项 | 值 |
|----|-----|
| 用户名 | `company_test` |
| 密码 | `CompanyTest@2026` |
| 角色 | 工程师（仅隧道登录；**不能**登 Web 管理端） |
| 建议 VPN 地址栏 | `192.168.196.17:8443`（家里 ZeroTier；若变了以 `client.yaml` 的 `server.address` 为准） |

## GUI 怎么填

1. 运行 `bin\haovpn-client-gui.exe`（可双击；非管理员会弹 UAC）。或：`bin\haovpn-client-gui.exe -c .\client.yaml`
2. 服务器地址：与 `client.yaml` 里一致（默认 ZeroTier `192.168.196.17:8443`）
3. 账号：`company_test`
4. 密码：`CompanyTest@2026`
5. 点「连接」

## CLI（可选）

```powershell
$env:HAOVPN_PASSWORD = "CompanyTest@2026"
.\bin\haovpn-client.exe -c .\client.yaml
```

## 注意

- 密码**不要**写进 `client.yaml`
- 若提示「须先修改密码」：家里 Web 用 admin 给该账号改密，或重新打包（本账号创建时已关闭 must_change）
- 若提示「用户名或密码错误」：确认家里服务端已启动且本账号已建（打包脚本会尝试自动创建）
