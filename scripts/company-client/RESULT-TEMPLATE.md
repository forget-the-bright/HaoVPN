# 公司机验证结果回填（测完填好 + 连同 diag 包带回）

填写日期：__________　　填写人：__________

## 本包测试账号（对照用，勿改密码后忘写）

| 项 | 本包默认 | 实际使用（若不同请改） |
|----|----------|------------------------|
| 账号 | `company_test` | |
| 密码 | （见 `TEST-ACCOUNT.md`，此处勿抄密码） | 已用 / 已改：____ |
| server.address | `192.168.196.17:8443` | |

## 环境

| 项 | 填写 |
|----|------|
| 公司电脑系统 | Win10 / Win11 / 其它： |
| 客户端类型 | GUI (`haovpn-client-gui.exe`) / CLI |
| 是否**管理员**运行 | 是 / 否 |
| 出网方式 | 公司网直出 / ZeroTier / 手机热点 / 其它： |
| 是否开公司代理或安全软件 | 是（名称：____）/ 否 |
| 分配的 VPN IP（日志 `vpn_ip=`） | |

## 结果（打勾）

- [ ] 客户端能启动（GUI 登录窗或 CLI 无配置错误）
- [ ] 日志出现「隧道握手成功」
- [ ] 日志出现「已应用服务端策略」且 gateway≈`10.88.0.1`
- [ ] `ping 10.88.0.1` 在隧道存活时有回复（可间歇）
- [ ] `ping` 工控目标通（目标 IP：________；不通可注明家里 `nat_ok=false`）
- [ ] 家里 Dashboard 显示 `company_test` 在线
- [ ] GUI：托盘 / 重连 / 退出登录 正常（若测）
- [ ] （可选）勾选杀开关后断线，状态行/日志有 `killswitch enabled (WFP)`

## 失败时（原文粘贴）

启动命令：

```
```

最后 30～50 行日志（优先 `logs\client.live.log`）：

```
```

`ping 10.88.0.1` 摘录（间歇超时请注明）：

```
```

## 必须带回的附件（缺一不可）

按顺序做：

1. 填写本文件并保存  
2. 在解压目录执行：

```powershell
cd C:\haovpn-company-test
pwsh -File .\collect-client-info.ps1
```

3. 把下面两样一起拷回 / 发给家里：

| 附件 | 说明 |
|------|------|
| `diag-YYYYMMDD-HHMMSS.zip` | 脚本自动生成：yaml、日志、系统摘要、`ping` 结果 |
| 本文件 `RESULT-TEMPLATE.md`（已填写） | 人工勾选与失败原文 |

### 自检：diag 包里应有

- [ ] `client.yaml`（无密码明文）
- [ ] `log-*-client.log` 或 `log-*-client.live.log`
- [ ] `system-summary.txt`（含 OS、是否管理员、10.88 路由、`ping 10.88.0.1`）
- [ ] `PACK-INFO.txt`（若有）
