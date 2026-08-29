# HaoVPN 0.1.1 发行说明草稿（可直接粘贴 GitHub Release）

> 相对 **0.1.0**（`release: 0.1.0`）→ **0.1.1**（当前 `VERSION`）。AI 不改 VERSION、不 commit/push。

## 提交信息草稿

```
refactor: 架构解耦第十四轮 — 叶子工具收口、安全硬化与 0.1.1 收尾

抽取 netutil LAN/CIDR 列表、切断 client→persist/sessionmgr 哨兵耦合；
修复公开 health 错误泄漏、末管理员保护、logout 仅 POST、用户名域校验、
online 分页与 GUI eng 竞态；同步 CODEMAP/hardening 与验收测。
```

（若一次提交含全部 WIP 功能，可用更宽标题，例如：）

```
release: 0.1.1 — via 出口、探针防御、差分重连与架构第十四轮

本地网段/via/ICS、探针与审计中文、客户端差分重连与 GUI 体验；
架构解耦第十四轮安全硬化与工具收口。详见发行说明。
```

---

## 发行说明正文（复制区）

### 摘要

工控现场 VPN 小版本：本地网段注册与 via 出口、探针防御、客户端差分重连与 GUI 体验，以及架构/安全收口。

### 新功能与增强

- **本地网段 / via 出口**：客户端 `local_lans` 上报注册表；托管路由经 via 转发；ExitLAN 回程；ICS SkipAsSource（保留 ICS）；via/ICS 前推迟装路由；差分重连保留数据面（配置未变跳过 ICS）
- **探针防御**：`security_events` / `ip_blocks`、中文特征对照、`reject_second` / 半死会话顶替、登录体验（GUI 等握手、CLI 无回显）
- **控制台**：托管路由 UI（应用 loading、访问方多选）、审计中文 labels、备份/导出 POST+CSRF、账号合一体验修补
- **客户端 GUI**：托盘 Logo、日志面板默认 300 行、登出/手动重连后台 Stop（不卡 UI）

### 架构与安全（第十四轮）

- 叶子工具：`netutil.ValidLANCIDRs` / `NormalizeCIDRList` / `AppendCIDRUnique`
- 哨兵：`auth.ErrAccountAlreadyOnline`（切断 client→sessionmgr 仅为错误类型的依赖）
- 公开 `/api/v1/health` **不再**返回 `recent_errors`（仅 Dashboard 需登录可见）
- 不可删除/禁用最后一个启用管理员；注销仅 POST；用户名格式在领域层强制
- 管理 API 500 对客户端稳定「内部错误」；GUI `eng` 指针与操作锁同锁

### 升级注意

- **须同时更新** 服务端与客户端（握手/注册表/via 语义）
- via 机配置 `local_lans`，且**勿与**服务端 NAT 工控网段重叠
- 改 peers 后控制台点「应用生效」；服务端重启后策略全量生效
- 现场建议跑：`.\scripts\dev-field-gate.ps1 -PlcHost <PLC> -UseHomeConfig`

### 自 0.1.0 以来主要提交（已入库）

- `feat: 本地网段注册、托管路由 via 出口与控制台体验`
- `feat: 探针防御、登录体验修复与安全事件中文对照`
- `refactor: 架构第十/十一轮 — 审计闭环、模块收敛与法律层授权`
- `refactor: 架构解耦第八/九轮 — 高内聚低耦合、模块拆分与 CODEMAP 对齐`
- `refactor: 架构解耦第七轮 — 原子写、timeutil、导出 YAML 与模块拆分`
- （本工作区未提交部分 + 第十四轮一并纳入本次发版）
