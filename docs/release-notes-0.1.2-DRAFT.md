# HaoVPN 0.1.2 发行说明草稿（可直接粘贴 GitHub Release）

> 相对 **0.1.1** → **0.1.2**（根目录 `VERSION`）。AI 不代为 tag/push。

## 提交信息摘要

```
release: 0.1.2 — 架构第十七轮安全闭环、PeerPolicyApplier、CSP 与文档治理
```

---

## 发行说明正文（复制区）

### 摘要

工控现场 VPN 小版本：架构解耦第十七轮安全/正确性闭环；`vpnaccount.PeerPolicyApplier` 承接 peer dirty/apply；WebUI 脚本外置与 CSP `script-src 'self'`；叶子工具与 Windows ACL/提权加固；文档入口收口与去重。

（自 0.1.1 以来已入库、本版一并交付：Windows GUI 托盘配置/跨平台自启、发送队列、展示时区、第十五～十六轮模块拆分与 peer 正确性。）

### 新功能与增强（含 0.1.1 之后已入库）

- Windows GUI 托盘「配置」：自动连接、无窗口、登录自启、可选服务；异步退出；服务接管
- 托盘「本机路由」分栏；`internal/autostart` 跨平台自启
- `vpn.send_queue_size` / 客户端队列；`api.display_timezone`；品牌 Logo / defaults 注释

### 正确性与安全（第十七轮为主）

- Session Cookie：登录/清除 Secure+SameSite 对齐；Touch 重发实现滑动续期；须改密可拉 CSRF
- transport `Close` 锁内拷贝 onClose；viaIndex 稳定排序
- peer_access 双方须存在且为 VPN 账号；JSON 体 1MiB；密码 ≤72
- 日志脱敏（Authorization / session=）；历史 logs items 出口再脱敏
- Windows：提权参数转义（含空格路径）；凭据 ACL 收紧；Everyone 可读检测
- CSP：管理页脚本外置；`script-src 'self'`（style 暂保留 unsafe-inline）
- peerDirty 仅内存：启动 WARN；重启后「待应用」清空

### 架构

- **`vpnaccount.PeerPolicyApplier`**：dirty/apply 出 api
- `fileutil` EnsureDir / AbsPair / Windows ACL；`safeutil.RetryN`；`netutil` 切片相等
- Web：`web/static/{index,user_list,peer_routes,tools,audit_log,security_probe,connection_detail}.js`

### 文档治理

- 记忆.md / docs/README 入口收口；进度唯一来源 = dev-log
- meta-plan 强化规划存档头；architecture / internal README 不堆轮次摘要

### 升级注意

- **须同时更新**服务端与客户端
- 有未应用变更时先点「应用生效」再重启服务
- via `local_lans` 勿与 NAT / VPN 池重叠

### 验证

```powershell
go test ./internal/api/ ./internal/vpnaccount/ ./internal/transport/ ./internal/sessionmgr/ ./internal/safeutil/ ./internal/fileutil/ ./internal/security/ -count=1
.\scripts\build-local.ps1
```

（全量 `go test ./...` 时若本机已有客户端占锁，`singleinstance` 可能失败，与本版逻辑无关。）
