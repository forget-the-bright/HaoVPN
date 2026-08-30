# Go自研VPN｜单版本完整交付规划（v1.0‑all‑in‑one）

> **【规划存档 · 勿当现行手册】**  
> - 本文保留 v1.0 **功能意图与 step 顺序**，便于对照「当初要交付什么」。  
> - **当前进度、目录树、包名、配置键以** [dev-log.md](dev-log.md)、[architecture.md](architecture.md)、[deploy.md](deploy.md) **为准**。  
> - 文中部分目录树 / `peers` 表术语 / docs 文件名是历史快照；发现冲突时**改代码侧文档，不要默默改规划正文冒充现行**。  
> 产品名：**HaoVPN**（模块 `haovpn`）。

> 目标：**一个版本交付「安全默认 + 极简部署 + 快速接入」的工控现场 VPN**——底层底座、多会话、账号审计、SQLite、YAML 全自动配置全部内置。
> 设计优先级：**安全 > 简单 > 快**（不为快牺牲安全）。
> 前提：底层可靠性模块（日志、panic捕获、buffer池、帧重组、心跳断线重连、MTU探测、优雅关闭）**全部内置在v1.0，不后置**。
> 允许：第一版功能全部存在，但性能、边缘容错可以偏弱；后续小迭代只做优化，不重构架构。
> 业务场景：项目现场服务端，可frp反向TLS‑TCP；多工程师客户端接入；访问工控局域网；管理接口默认仅本机/VPN网卡；**全平台**（Linux / Windows / macOS）服务端与客户端；服务端统一管理用户账号密码，每个账户可查看连接详情。
> 技术底座：Go + Wintun(TUN) + wireguard‑go密码学（不使用它的UDP socket） + TLS‑TCP自定义帧传输 + SQLite + YAML。

## 三大设计原则（所有功能都按这个排优先级）

> **安全 > 简单 > 快**。可以为了安全牺牲一点便利，可以为了简单砍掉花哨功能，但**绝不为「快」或「好看」削弱安全**。

| 原则 | 对外怎么说 | v1.0 硬指标 |
|------|------------|-------------|
| **安全** | 默认就是最安全配置，不用用户懂安全也能用对 | 管理面**默认**不暴露公网；双层加密；账号/隧道双鉴权；全审计；禁用户即踢线；敏感信息不落日志 |
| **简单** | 拷一个文件、点几下网页就能用 | 首次启动自动生成配置；WebUI 不超过 **5 个主页面**；开户到导出 **≤3 次点击**；工程师侧解压即用 |
| **快** | 部署快、连得快、断线恢复快 | 现场 **5 分钟**跑通；客户端拨号到可用 **≤10s**；断线 **≤8s**内自动重连；单二进制无依赖安装 |

### 安全优先：v1.0 安全架构（必须写进代码，不是文档空话）

```
┌─────────────────────────────────────────────────────────────┐
│  第 1 层 · 暴露面最小化                                      │
│  管理 API 默认只绑 127.0.0.1 + TUN IP │ 隧道端口与管理端口分离   │
│  绑定 0.0.0.0/:: 须显式 allow_public_bind: true（用户自担风险）│
│  未勾选该选项却配了公网监听 → 拒绝启动（防误配，非禁止能力）    │
├─────────────────────────────────────────────────────────────┤
│  第 2 层 · 传输与隧道加密                                    │
│  外层 TLS 1.2+（禁明文、禁弱套件）│ 内层 WireGuard 密码学     │
│  证书校验默认开启 │ 客户端默认不允许 insecure_skip_verify    │
├─────────────────────────────────────────────────────────────┤
│  第 3 层 · 身份与准入                                        │
│  WebUI：bcrypt 密码 + Session + CSRF + 登录限流 + 首次强改密  │
│  隧道：账号公钥白名单 + enabled 校验 + 可选源 IP 白名单         │
│  禁用账号 → 立即踢线 + 拒绝新握手                             │
├─────────────────────────────────────────────────────────────┤
│  第 4 层 · 权限与最小授权                                    │
│  AllowedIPs 握手下发（服务端权威，分流，非全隧道）            │
│  入站校验：src 必须=VPNIP，dst 必须∈AllowedIPs；横向隔离      │
│  NAT 仅放行配置的工控网段，不做「全网放行」                   │
│  配置导出、备份下载 → 必须登录 + 写审计                       │
├─────────────────────────────────────────────────────────────┤
│  第 5 层 · 可追溯与防泄露                                    │
│  管理操作全量 audit_logs │ 密码/私钥/会话 token 禁止写日志    │
│  响应头：X-Content-Type-Options、X-Frame-Options、CSP 基础版  │
│  SQLite/配置文件权限提示：Linux 600、Windows 仅管理员可读     │
└─────────────────────────────────────────────────────────────┘
```

### 简单优先：能少一步就少一步

- **服务端**：`./haovpn-server` → 自动生成 `server.yaml` + 证书 + 数据库 → 浏览器改密码 → 完成。
- **开户**：WebUI「新建账号」一步完成（密钥+IP+策略）→「下载 ZIP」→ 发给工程师，**无需手写 YAML**。
- **客户端**：解压 zip → `haovpn-client.exe`（或装服务）→ 握手后按服务端策略配 TUN/路由，**无需装运行时、无需 Docker**。
- **WebUI**：不做复杂前端框架；页面固定 5 个：Dashboard / **账号** / 连接详情 / 审计 / 登录。
- **配置**：所有可调项都在 YAML 里，但 **90% 场景用默认值即可**；客户端策略以握手应答为准。

### 快优先：省时间的地方要极致

| 场景 | 目标 | 实现手段 |
|------|------|----------|
| 现场首次部署 | ≤5 min | 单二进制 + 自动生成配置/证书/DB + 自检通过即监听 |
| 工程师接入 | ≤3 min | zip 配置包一键导入；Windows 服务开机自连 |
| 隧道建立 | ≤10 s | TLS 会话复用（可选）；并发热身心跳；buffer 池减 GC |
| 断线恢复 | ≤8 s | 指数退避 1→2→4→8s；会话状态 SQLite 持久化，重连不断户 |
| 管理操作反馈 | ≤1 s | SQLite 本地读写；WebUI 无重型前端；API 轻量 JSON |

## 产品定位与核心卖点（v1.0 就要能讲清楚）

**一句话**：**安全默认、极简部署、快速接入**——面向工控现场的自托管 VPN；**安全不是可选项，是出厂设置**。

### 六大核心卖点（对外宣传用）

| 卖点 | 用户痛点 | 我们怎么解决 |
|------|----------|--------------|
| **安全默认** | VPN 管理口被扫、弱口令、不知道谁连过 | **五层安全架构**；管理面**默认非公网**；误配 **0.0.0.0 须显式勾选**；双层加密；**禁用即踢线**；**全操作审计** |
| **极简部署** | 装 OpenVPN/WG 要半天，配置看不懂 | **单文件**拷过去就能跑；**自动生成** YAML/证书/数据库；WebUI **1 步开户**；注释写满 |
| **快速接入** | 工程师到现场还要远程指导配路由 | **zip 配置包解压即用**；自动分流路由；断线 **8s 内**重连；Windows **服务开机自连** |
| **穿透力强** | 现场封 UDP，WireGuard 直连经常废 | **TLS‑TCP** 过防火墙；天然适配 **frp**；现场无需公网 IP |
| **工控开箱即用** | 不会配 NAT/AllowedIPs | WebUI 勾选工控网段 → 自动 SNAT + 分流；导出即可 ping PLC |
| **可追溯运维** | 出事说不清谁干的 | 按账户看连接/流量/历史；审计日志**不可删**（v1.0 仅追加）；日志可导出给售后 |

### 与常见方案差异（销售/选型对照）

| 维度 | 官方 WireGuard | OpenVPN | Tailscale 等组网 SaaS | **本项目 v1.0** |
|------|----------------|---------|----------------------|-----------------|
| **安全默认** | 无管理面，但无审计/账号 | 易误配暴露 | 数据经第三方 | **默认非公网管理 + 显式勾选才可暴露 + 全审计** |
| **部署难度** | 手工配 peer | 复杂 | 注册即用 | **单文件、自动生成配置；账号合一** |
| **接入速度** | 快但常被 UDP 拦 | 较慢 | 快 | **TLS‑TCP 穿透 + 8s 重连** |
| 传输 | UDP（常被拦） | TCP/UDP | 依赖厂商中继 | **TLS‑TCP，自控链路** |
| 工控 NAT | 需自行 iptables | 需自行配置 | 按产品能力 | **内置 NAT + 分流** |
| 离线/内网 | 可以 | 可以 | 依赖外网注册 | **完全离线可运行** |
| **全平台** | 各平台工具分散 | 各平台工具分散 | 有客户端 | **Linux/Win/macOS 服务端+客户端同一套代码** |

## 全平台支持（v1.0）

| 平台 | 服务端 | 客户端 | TUN 实现 | 路由/NAT |
|------|--------|--------|----------|----------|
| **Linux** amd64/arm64 | ✅ | ✅ | `/dev/net/tun` | netlink + iptables/nftables |
| **Windows** amd64 | ✅ | ✅ | Wintun | Win32 路由表 + ICS/NAT |
| **macOS** amd64/arm64 | ✅ | ✅ | utun | route 命令 / Network Extension 薄封装 |

- `Makefile` `release` 目标调用 `scripts/build-release`，一次产出 **6 平台**（linux/win/darwin × amd64/arm64）。
- 配置、WebUI、API **各平台行为一致**；仅 TUN/路由/NAT 走平台实现文件（`tun_*.go`、`route_*.go`）。
- Windows 客户端支持 `--service`；Linux 提供 systemd 示例；macOS 提供 launchd 示例（写入 `docs/deploy.md`）。

## 完整项目目录（v1.0一次性全部落地）
```
HaoVPN/
├── go.mod
├── go.sum
├── cmd
│   ├── server                 # 服务端入口（项目现场）
│   │   └── main.go
│   └── client                 # 客户端入口（工程师侧）
│       └── main.go
├── internal
│   ├── logger                 # 分级日志、滚动日志、堆栈打印【核心底座】
│   │   └── logger.go
│   ├── safeutil               # GoSafe安全goroutine、信号处理、优雅关闭工具【核心底座】
│   │   └── goroutine.go
│   ├── tun                    # TUN 全平台抽象：Linux / Windows(Wintun) / macOS(utun)
│   │   ├── tun.go
│   │   ├── tun_linux.go
│   │   ├── tun_windows.go
│   │   └── tun_darwin.go
│   ├── transport              # TLS‑TCP传输层：帧协议、分片重组、buffer池、心跳、断线回调、重连逻辑【核心底座】
│   │   ├── frame.go
│   │   ├── pool.go
│   │   └── transport.go
│   ├── crypto                 # wireguard‑go密码学薄封装，只做加密解密会话密钥
│   │   └── wg_crypto.go
│   ├── ippool                 # IP地址池，分配回收VPN虚拟IP
│   │   └── pool.go
│   ├── sessionmgr             # 多会话管理器，账号会话、AllowedIPs 报文路由、入站 src/dst 校验
│   │   └── manager.go
│   ├── vpnaccount             # VPN IP 模式（fixed/dynamic_session/dynamic_lease）与策略解析
│   │   └── service.go
│   ├── tunnel                 # 握手协商下发 vpn_ip/allowed_ips/mtu/policy_ver
│   │   └── handshake.go
│   ├── netstack               # 跨平台路由、NAT、iptables/nftables/win32网络配置
│   │   ├── nat.go
│   │   └── route.go
│   ├── config                 # YAML配置加载、校验、首次启动生成默认配置文件（含详细注释）
│   │   ├── config.go
│   │   ├── server.go
│   │   ├── client.go
│   │   └── defaults.go
│   ├── persist                # SQLite：VPN 账号（users 合一，无 peers 表）、IP 池、连接事件、会话统计
│   │   ├── store.go
│   │   ├── schema.sql
│   │   └── migrate_v2.go
│   ├── auth                   # 用户账号认证：密码哈希校验、管理API登录鉴权、会话token、登录限流
│   │   ├── user.go
│   │   ├── password.go
│   │   └── session.go
│   ├── audit                    # 管理操作审计：谁、何时、做了什么
│   │   └── audit.go
│   ├── health                   # 启动自检、运行状态、就绪探针
│   │   └── health.go
│   ├── version                  # 编译版本、构建信息（-version 输出）
│   │   └── version.go
│   ├── security                 # 安全策略集中：TLS 套件、监听校验、敏感信息脱敏、响应头
│   │   ├── tls_policy.go
│   │   ├── bindcheck.go         # 管理口绑定校验：0.0.0.0/:: 须 allow_public_bind:true
│   │   └── redact.go            # 日志/API 响应脱敏（密码、私钥、token）
│   └── api                    # HTTP管理API层；默认只绑 lo+TUN IP；公网绑定须显式勾选
│       ├── handler.go
│       ├── router.go
│       ├── middleware.go        # 鉴权、CSRF、登录限流
│       ├── user_handler.go    # 用户账号增删改查
│       ├── monitor_handler.go # 连接详情、流量统计、在线状态、强制踢下线
│       ├── dashboard_handler.go # 总览：在线数、流量、告警摘要
│       └── backup_handler.go  # SQLite 备份导出
├── web
│   ├── embed.go               # go:embed 打包静态资源，单二进制分发
│   ├── static                 # js/css
│   └── templates              # html template极简webui
│       ├── index.html         # Dashboard 总览
│       ├── login.html
│       ├── user_list.html     # VPN 账号管理（合一）
│       ├── connection_detail.html  # 账号连接详情监控
│       └── audit_log.html     # 审计日志
├── scripts                    # 打包 & 开发测试脚本（见 scripts/README.md）
│   ├── platforms.txt          # 交叉编译目标：6 平台 amd64+arm64
│   ├── lib/build-common.ps1
│   ├── build-release.ps1 / .sh
│   ├── build-local.ps1        # Windows 本机快速构建
│   ├── dev-gen-certs.ps1 / .sh
│   ├── dev-smoke-test.sh
│   ├── dev-security-check.sh
│   └── frp-example.toml
├── VERSION                    # 唯一版本号（仅开发者维护，AI 禁止修改）
├── Makefile                   # 可选：调用 scripts/build-release
├── README.md                  # 项目简介、快速用法、文档索引
├── 记忆.md                    # AI/新人接手：阅读顺序与当前进度
├── docs
│   ├── README.md                  # 文档索引（现行导航）
│   ├── development-principles.md  # 开发原则
│   ├── comment-style.md           # 注释规范
│   ├── versioning.md / licensing.md
│   ├── architecture.md            # CODEMAP（现行包结构）
│   ├── meta-plan.md               # 本文件：v1.0 规划存档
│   ├── deploy.md / troubleshooting.md / security-hardening.md
│   ├── dev-log.md                 # 唯一进度日志
│   └── release-notes-*-DRAFT.md   # 发版说明草稿
└── config                     # 参考示例；运行时配置由首次启动自动生成
    ├── server_example.yaml
    └── client_example.yaml
```

## 📌v1.0全部功能清单（全部实现，不分版本）
### 🔴底座可靠性组（优先级最高，先写，所有模块依赖）
1. logger：TRACE/DEBUG/INFO/WARN/ERROR/FATAL；文件滚动日志；error打印堆栈；网络/TUN/密码学全链路详细日志。
2. safeutil.GoSafe：统一goroutine包装，捕获panic，打印堆栈，执行资源清理，**不允许单goroutine崩溃搞垮整个进程**。
3. 系统信号处理SIGINT/SIGTERM：完整优雅关闭流程；依次关闭transport会话、释放tun设备、释放资源、等待全部goroutine退出。
4. transport传输层：
   - 自定义帧 `[4字节大端长度][payload]`，完整粘包半包重组。
   - sync.Pool buffer池复用，减少GC压力。
   - TLS‑TCP服务端/客户端；**TLS 1.2+ 最低版本，禁用弱 cipher**（`security/tls_policy.go` 统一配置）。
   - 心跳帧、超时断连；状态机 Connected/Disconnecting/Closed。
   - 客户端断线自动重连，指数退避(1‑2‑4‑8s，上限8s)；每一次断开、重连、失败都打详细日志。
   - MTU探测；出站队列限流，防止缓冲区无限堆积；写超时，规避TCP‑over‑TCP恶性堆积。
   - **单 peer 最大并发连接数 = 1**（新连接踢旧连接，防账号共享，安全默认）。
5. tun抽象层：Linux TUN / Windows Wintun / **macOS utun** 统一接口；MTU设置；原始IP报文读写；错误全日志输出。
6. crypto/wg_crypto：复用wireguard‑go密码学、防重放窗口；不使用原生UDP socket；对外只暴露加密/解密/密钥生成接口；解密失败、重放攻击完整日志输出。
7. version：`-version` 输出语义化版本、git commit、构建时间；API `/api/v1/system/info` 同步返回，便于售后定位现场版本。

### 🟠网络业务组（底座完成后开发）
8. ippool：VPN虚拟IP池，支持网段配置；IP分配、离线回收；持久化保存已分配IP。
9. sessionmgr会话管理器：
   - 多peer会话并发管理；每个peer绑定公钥、VPN虚拟IP、transport连接、AllowedIPs规则。
   - IP报文路由逻辑：TUN读取出来的IP包，根据目标IP匹配AllowedIPs，分发到对应peer会话；对端报文解密后写入本地TUN。
   - 会话超时清理；会话上线/下线事件回调，日志记录；同步更新连接监控统计（写入 SQLite）。
   - **隧道准入**：握手时校验账号公钥、enabled；禁用账号**拒绝新连接**并踢掉已有会话。
   - **管理员强制踢下线**：API/WebUI 一键断开指定账号，记录审计日志。
   - **握手下发策略**：`vpn_ip` / `allowed_ips` / `mtu` / `ip_mode` / `policy_ver`；客户端以应答为准。
   - **入站加固**：src 必须等于会话 VPNIP；dst 必须∈AllowedIPs；禁止横向访问其他账号 VPN IP。
10. netstack路由&NAT模块：
   - Linux：netlink操作路由；iptables/nftables配置SNAT，允许VPN客户端访问现场物理工控网段。
   - Windows：Win32 API操作路由表；开启ICS/NAT共享。
   - **macOS**：`route` 子进程 / 系统 API 增删路由；NAT 按平台能力文档说明（工控访问以 Linux/Win 服务端为主）。
   - 接口：SetupNat、TeardownNat、AddRoute、DelRoute；执行前后打印日志，记录执行结果。

### 🟡管理层（业务层完成后开发）
11. persist持久化store（SQLite）：
    - 数据库文件默认 `data/haovpn.db`（路径可在 server.yaml 配置）。
    - 表结构：**users**（Web+隧道身份合一）、ip_allocations、connection_events、session_stats、**audit_logs**。**无 peers 表**（v1→v2 启动自动迁移）。
    - users 隧道字段：public_key、private_key_enc、vpn_ip、allowed_ips、ip_mode、ip_lease_sec、policy_ver。
    - 启动时自动执行 schema 迁移；写操作事务落盘；变更打日志。
    - **备份**：API 导出 SQLite 快照（或文件级 copy）；文档说明定期备份策略。
12. auth / VPN 账号模块：
    - **1 用户 = 1 隧道身份**：新建账号同事务生成密钥对、按 ip_mode 处理 IP、默认 AllowedIPs。
    - 密码存储使用 bcrypt 哈希，**明文不落盘**；私钥 AES 加密存库。
    - IP 模式：`fixed`（开户分配）/ `dynamic_session`（断线立即回收）/ `dynamic_lease`（租约内复用）。
    - 管理 API / WebUI 登录鉴权（session cookie + CSRF）；**首次登录强制改默认 admin 密码**。
    - **登录防暴力**：同 IP 连续失败 N 次临时锁定；密码最小长度校验。
13. audit审计模块：
    - 记录所有敏感管理操作：登录/登出、账号 CRUD、改密、策略变更踢线、配置导出、数据库备份。
    - 字段：操作者、动作、目标、IP、时间、结果；WebUI 分页查询，SQLite 持久化。
14. 连接监控（按账号维度）：
    - 每个账号可查看：当前在线状态、VPN 虚拟 IP、最近上线/下线时间、累计在线时长。
    - 连接详情：远端地址、TLS 连接建立时间、最近心跳时间、收发字节数、当前 AllowedIPs、断线重连次数。
    - 历史连接事件列表（可分页）；sessionmgr 实时更新统计，定期刷入 SQLite。
15. health健康检查与启动自检：
    - 启动前检查：配置文件可读、SQLite 目录可写、TLS 证书存在或触发自签生成、TUN 权限（Linux `CAP_NET_ADMIN` / Windows 管理员）。
    - 运行态：`/api/v1/health` 就绪探针；Dashboard 展示 uptime、在线账号数、DB 状态、最近错误摘要。
16. api HTTP管理接口：
    - **默认**：只监听 `127.0.0.1` + TUN 虚拟网卡 IP（安全出厂设置）。
    - **公网/全接口绑定（0.0.0.0 / ::）**：允许，但必须同时满足：
      1. `api.listen_hosts` 含 `0.0.0.0` 或 `::`；
      2. **`api.allow_public_bind: true`**（显式勾选，默认 `false`）；
      3. 启动时打印 **WARN 横幅 + 写 audit_logs**，声明「用户自行配置、自行承担暴露风险，与软件无关」。
    - **未勾选 `allow_public_bind` 却配置 0.0.0.0/::** → **拒绝启动**（防手滑误配；开发测试时手动勾选即可）。
    - 鉴权：除登录接口外，所有管理接口需已登录。
    - 接口列表：
        - Dashboard 总览：在线人数、今日流量、最近上下线、服务健康
        - 登录 / 登出 / 修改自己的密码
        - 账号：列表、创建（自动密钥/IP）、禁用/启用、删除、`PATCH /users/{id}/vpn`、踢线
        - 监控：按账号查询连接详情、在线列表、流量统计、连接事件历史
        - 导出：`GET /api/v1/users/{id}/export.zip`（凭证为主；策略握手下发）
        - 审计日志查询；SQLite **备份下载**
        - 系统信息（版本、构建信息）、日志快照（最近 N 行）
17. WebUI：go html/template极简页面，无复杂前端；
    - **Dashboard 首页**；登录页（含首次改密引导）；**VPN 账号管理**（创建/策略/导出/踢线）；连接详情；审计日志。
    - 静态资源 **go:embed 打进二进制**，现场只拷贝一个 server 可执行文件即可带 WebUI。
    - 只有现场本机、或者VPN接入后的客户端可以访问页面。
18. YAML配置文件（服务端 + 客户端）：
    - **首次启动**：若配置文件不存在，自动生成带完整默认值的 `server.yaml` / `client.yaml`，并写入**逐行详细中文注释**说明每个字段含义、单位、示例、注意事项。
    - 配置路径可通过命令行 `-c` 覆盖；默认服务端 `./server.yaml`，客户端 `./client.yaml`。
    - 服务端主要项：监听地址、**`allow_public_bind` 危险开关**、TLS 证书、VPN 网段、IP 池、MTU、心跳超时、工控允许网段、SQLite 路径、管理 API 端口、默认 admin 账号、日志级别与滚动策略。
    - 客户端主要项：服务端地址、TLS 校验、本地 TUN 名称、MTU、重连退避、私钥；**vpn_ip/allowed_ips 以握手应答为准**（yaml 中若残留仅 WARN）。
    - 使用 `gopkg.in/yaml.v3` 解析；加载后校验必填项与取值范围，错误信息明确指出字段名。
19. TLS 证书首次自签（v1.0 内置，卖点「开箱即用」）：
    - 若 `server.yaml` 中 cert/key 路径不存在且 `tls.auto_generate: true`（默认开启），自动生成 **10 年自签证书** 到 `./certs/`。
    - 日志明确提示：生产环境建议替换为正式证书；导出客户端配置时附带 CA 或跳过校验说明。
20. 交付物与文档（v1.0 一并交付，不是后置）：
    - `README.md`：项目简介与文档索引。
    - `记忆.md`：AI/新人接手指南与阅读顺序。
    - `docs/development-principles.md`：开发原则（不敷衍、不留坑、必须验证）。
    - `docs/deploy.md`：部署拓扑、安装、配置、验收测试、脚本说明。
    - `docs/dev-log.md`：项目开发历程与踩坑记录。
    - `docs/security-hardening.md`：生产环境安全检查清单。
    - `docs/troubleshooting.md`：常见故障排障。
    - `docs/versioning.md`：VERSION 文件与发版流程（仅开发者改版本）。
    - `scripts/`：全平台构建（6 目标 × server/client）、本机构建、开发工具脚本。
- 首次启动若无 `client.yaml`，自动生成带详细注释的默认配置文件。
- **单二进制**：客户端同样 embed 必要资源（若有）；`-version` 输出版本信息。
- TLS 拨号服务端；断线指数退避重连；使用配置中的服务端地址与 TLS 参数。
- 本地创建 TUN/Wintun 网卡；退出时**自动清理路由**（避免残留路由表）。
- 根据服务端下发的 AllowedIPs 自动配置本机路由（**分流**，不全隧道）。
- peer 密钥等身份配置写在 `client.yaml` 的 peer 段（或由服务端 **zip 配置包** 覆盖）。
- **运行模式**（v1.0 至少支持）：
  - 前台运行（默认，方便调试，控制台打印连接状态）。
  - Windows：**`--service install/start/stop`** 注册为系统服务，开机自连（工程师笔记本场景）。
  - Linux：**systemd unit 示例**写入 `docs/deploy.md`（unit 文件本身可后置，但文档 v1.0 要有）。
- 本地简易状态：CLI 打印当前连接状态、虚拟 IP、最近错误；日志文件滚动。

### 🔵安全与合规（v1.0 底线，优先级最高）

**设计信条：不安全，不发布。** 以下每一条都是 v1.0 必测项，不是「后续加固」。

#### 暴露面控制
- 管理 API **默认**只监听 `127.0.0.1` + TUN 虚拟网卡 IP（`allow_public_bind` 默认 `false`）。
- **`bindcheck.go` 规则**（防误配，非剥夺用户选择权）：
  - `listen_hosts` 含 `0.0.0.0` / `::` 且 **`allow_public_bind: false`** → **FATAL 拒绝启动**，日志提示：「若确需公网监听，请显式设置 `api.allow_public_bind: true`」。
  - `allow_public_bind: true` → **允许启动**，但启动时：控制台 + 日志 **WARN 横幅**、写入 `audit_logs`（动作 `management_public_bind_enabled`），文案含免责声明。
- **免责声明（写进代码注释、YAML 注释、启动日志）**：
  > 开启 `allow_public_bind` 表示您已知悉管理口将对所有网卡开放，可能遭受公网扫描与攻击；**该风险由配置者自行承担，与软件无关**。
- 隧道端口（如 8443）与管理端口（如 8080）**严格分离**；文档建议生产现场管理口不要经 frp 暴露。
- 未登录访问管理 API → **401**；已登录无权限 → **403**；不泄露内部堆栈给前端。

#### 开发 / 测试场景
- 本地或内网联调需从其他机器访问 WebUI：在 `server.yaml` 中设置 `listen_hosts: ["0.0.0.0"]` + **`allow_public_bind: true`** 即可。
- **禁止**在代码或测试中硬编码公网监听；测试用例应覆盖「未勾选拒绝 / 勾选后放行」两条路径。
- 生产交付检查清单（`security-hardening.md`）要求：`allow_public_bind` 必须为 `false`。

#### 加密与传输
- **双层加密**：外层 TLS + 内层 WireGuard 密码学；隧道 payload 明文传输 **禁止**。
- TLS：**最低 1.2**；优先 TLS 1.3；禁用 NULL/EXPORT/RC4 等弱套件；客户端默认 **校验服务端证书**。
- 自签证书仅用于**首次开箱**；文档 + 启动日志醒目提示生产换正式证书。

#### 身份、会话、准入
- 密码 **bcrypt**（cost ≥ 12）存储；**明文、MD5、SHA1 一律禁止**。
- WebUI Session：**HttpOnly + Secure（HTTPS 时）+ SameSite=Lax** Cookie；登出即销毁 session。
- 表单 **CSRF Token**；登录接口 **同 IP 5 次失败锁 15 分钟**（可配置）。
- **首次登录强制修改**默认 admin 密码；密码最小 8 位，建议大小写+数字（校验器提示）。
- 隧道握手：**peer 公钥必须在白名单**；对应 user/peer `enabled=false` → **拒绝握手**。
- 可选 **`tunnel.allowed_ips`**：仅允许指定来源 IP 发起 TLS（现场固定出口场景）。

#### 最小授权（工控场景尤为重要）
- 客户端默认 **分流**（AllowedIPs 仅工控网段 + VPN 网段），**禁止默认 0.0.0.0/0 全隧道**。
- 服务端 NAT **仅**对 `nat.allowed_lan_cidrs` 配置网段做 SNAT，不泛化到全网。
- 每个 peer 独立 AllowedIPs，**不能横向访问**其他 peer 的虚拟 IP（会话隔离）。

#### 敏感数据与日志
- 密码、私钥、session token、导出 zip 密码：**禁止出现在日志和 API 响应**（`redact.go` 统一脱敏）。
- SQLite 存私钥时 **AES 加密**（密钥来自机器本地或配置，文档说明备份注意）。
- 配置文件、数据库文件：启动时检查权限，过宽则 **WARN** 并文档说明 chmod 600。

#### 审计与追责
- 登录/登出、用户 CRUD、改密、peer 操作、踢下线、配置导出、DB 备份 → **全部写 audit_logs**。
- 审计日志 v1.0 **只追加不删除**（无 UI 删除按钮）；查询分页。
- 连接事件（上下线）与审计分离，便于运维排障。

#### HTTP 安全头（管理 WebUI）
- `X-Content-Type-Options: nosniff`
- `X-Frame-Options: DENY`
- `Content-Security-Policy: default-src 'self'`（极简，不引外站 CDN）
- 管理页 **禁止** embed 第三方脚本/统计。

#### 安全测试（step11 必测）
1. `listen_hosts: ["0.0.0.0"]` 且 **`allow_public_bind: false`** → 进程 **拒绝启动**。
2. 同上但 **`allow_public_bind: true`** → **正常启动**，日志含 WARN 横幅，audit 有记录。
3. 默认配置（仅 127.0.0.1 + TUN）下，非本机非 VPN 网段 `curl` 管理端口 → **连接失败**。
4. 错误密码连续 5 次 → **锁定**；审计有记录。
5. 未登录 `curl` 导出配置 API → **401**。
6. 禁用用户 → **秒级踢线** + 新连接 **握手失败**。
7. 日志文件中 **搜不到** 明文密码和 private_key。
8. 默认 AllowedIPs **不含** `0.0.0.0/0`。
9. CSRF：无 token 的 POST → **403**。

## 开发执行顺序（严格按这个流程，不要跳，底座优先）
> 原则：底层先跑通单元测试，再向上组装上层业务；不要先写web再写底层。

1. **step1：底座工具包**
    - logger日志模块完整实现。
    - safeutil：GoSafe安全goroutine、信号捕获、优雅关闭工具。

2. **step2：transport传输层**
    - frame帧编解码、分片重组；写单元测试模拟粘包。
    - pool buffer池。
    - TLS‑TCP服务端、客户端；心跳、状态机、断线回调、客户端自动重连。
    - MTU探测、发送队列限流、写超时逻辑。

3. **step3：tun虚拟网卡抽象层**
    - Linux TUN、Windows Wintun、**macOS utun** 实现；验证读写原始IP报文。

4. **step4：crypto密码封装层**
    - 封装wireguard‑go；只调用密码学能力；不启用它的UDP。
    - 单元测试：密钥生成、加密解密正确性。

5. **step5：网络业务层**
    - ippool IP地址池。
    - persist SQLite持久存储（schema、migrate、CRUD）。
    - auth 用户账号模块（密码哈希、默认 admin）。
    - sessionmgr多会话管理器，实现IP报文转发、AllowedIPs路由逻辑、连接统计上报。
    - netstack路由NAT模块，跨平台路由/NAT操作。

6. **step6：config配置模块**
    - YAML 结构体定义（server/client）。
    - 首次启动检测配置文件不存在 → 生成带详细中文注释的默认 `server.yaml` / `client.yaml`。
    - 配置校验与友好错误提示。

7. **step7：HTTP API + WebUI**
    - api handler、router；middleware（鉴权、CSRF、登录限流）；Dashboard、审计、备份、踢下线接口。
    - web embed 单二进制；`bindcheck` 校验管理口绑定策略。
    - html template：Dashboard、登录（含首次改密）、用户/peer、连接详情、审计日志。

8. **step8：security + health + audit + TLS 自签 + version**
    - `bindcheck` 管理口校验（未勾选 `allow_public_bind` 拒绝 0.0.0.0）；TLS 策略；日志脱敏；安全响应头中间件。
    - 启动自检；健康探针；审计写入；证书自动生成；`-version` 与 system info API。

9. **step9：组装 cmd/server、cmd/client 完整主程序**
    - 解析命令行（`-c`、`-version`、`--service`）；加载/生成 YAML；打开 SQLite；启动各模块；优雅关闭串联。

10. **step10：文档与打包**
    - 维护 docs/ 与 记忆.md；`scripts/build-release` 交叉编译 release 包。

11. **step11：全场景测试（必须覆盖）**
    1. 正常多客户端接入，ping现场工控设备。
    2. 网络断开，客户端自动重连，会话恢复。
    3. goroutine模拟panic，验证进程不会崩溃，打印堆栈，清理会话。
    4. 断开客户端，IP正确回收，SQLite 记录更新。
    5. 默认配置下公网无法访问管理接口；本机、VPN内可以访问；**勾选 allow_public_bind 后**按 listen_hosts 可达（用于开发验证）。
    6. Ctrl+C优雅退出，TUN设备、NAT路由正确清理。
    7. 首次启动服务端/客户端，自动生成默认 YAML 且注释完整可读。
    8. WebUI 创建用户、分配 peer，该用户连接后可查看连接详情与流量统计。
    9. 禁用用户后，对应 peer **立即踢下线**且无法新建 TLS 连接。
    10. 首次启动自动生成 YAML + 自签证书；删证书后重启可再生成。
    11. Dashboard 数据与在线 peer 一致；审计日志记录踢人与导出操作。
    12. Windows 客户端 `--service` 安装后重启机器可自动连上；退出后路由无残留。
    13. `make release` 产出 win/linux 四件套（server/client × amd64）可直接发给现场。
    14. **安全测试 9 项**（见「安全测试」清单）全部通过。

### 管理口绑定策略（`api.listen_hosts` + `api.allow_public_bind`）

| 场景 | listen_hosts | allow_public_bind | 行为 |
|------|--------------|-------------------|------|
| **生产默认（推荐）** | `["127.0.0.1"]` + TUN IP 自动追加 | `false` | 仅本机 / VPN 内可访问管理页 |
| **手滑误配** | 含 `0.0.0.0` 或 `::` | `false` | **拒绝启动**，提示勾选 |
| **开发联调 / 内网测试** | `["0.0.0.0"]` 或具体内网 IP | `true` | 允许启动；WARN 横幅 + audit + 免责声明 |
| **仅绑内网 IP** | `["192.168.1.100"]` | `false` | 允许；仅该 IP 可访问，无需勾选 |

> **原则**：软件**不替用户决定**是否暴露公网，但**默认最安全**；要暴露必须**两步显式操作**（改 listen + 勾选 true），且启动时**大声提醒**。配置者已知悉风险则与软件无关。

## v1.0 验收演示脚本（对外 Demo / 交付验收用）
> 按此流程跑通即视为 v1.0 可交付。演示顺序：**先讲安全，再讲简单，最后讲快**。

### A. 安全演示（必做，放在最前面讲）
1. 展示默认 `server.yaml`：`listen_hosts` 仅 `127.0.0.1`，**`allow_public_bind: false`**。
2. 演示误配：只改 `listen_hosts: ["0.0.0.0"]` 不改勾选 → 服务 **拒绝启动**（防手滑）。
3. 说明开发场景：同时设 `allow_public_bind: true` → 可启动，日志 **WARN + 免责声明**，audit 有记录；**强调生产必须 false**。
4. 默认配置下，外网 `curl` 管理端口 → **失败**；本机 / VPN 内 → **成功**。
5. 审计页：创建用户、导出配置、踢下线 → **每条都有记录**。
6. 禁用账户 → 客户端 **立刻断开**；启用后恢复。
7. 日志全文搜索 `password` / `private_key` → **无明文**。

### B. 简单演示
1. **5 分钟部署**：只拷贝一个 server 二进制 → 双击/运行 → 自动生成配置 + 证书 + 数据库 → 浏览器改密码 → 完成。
2. **1 步开户**：新建账号（自动密钥+IP）→ 下载 zip → 发给工程师。
3. 工程师侧：解压 → 运行 client → **策略由握手下发，无需手改 allowed_ips**。

### C. 快速演示
1. 计时：client 启动到 Dashboard 显示 **在线 ≤10s**。
2. 拔网线 / 断 WiFi → 恢复后 **≤8s** 自动重连，IP 不变。
3. 客户端 ping 通 PLC（如 192.168.1.10），Connection Detail **流量实时涨**。

### D. 灾备（可选）
1. 下载 SQLite 备份 → 模拟误删 peer → 按文档恢复 → 数据回来。

## v1.0允许存在的短板（第一版不做，后续小迭代）
> 全部功能模块都具备，下面这些属于优化增强，不影响完整流程跑通
1. 完整TCP‑over‑TCP高级拥塞滑动窗口（v1.0只做队列限流、MTU规避分片，不做复杂拥塞算法）
2. RBAC 多角色细粒度权限（v1.0 仅区分：未登录 / 已登录管理员；不做只读运维等角色）
3. ACME/Let's Encrypt 自动续期（v1.0 仅自签 + 手工替换正式证书）
4. Prometheus 指标埋点（v1.0 用 Dashboard + SQLite 统计代替）
5. 配置热重载（改 YAML 需重启，文档写清楚）
6. 连接监控历史自动归档/TTL 清理（v1.0 全量写 SQLite）
7. ~~客户端系统托盘 GUI、移动端（v1.0 仅 CLI + Windows 服务）~~ → **已交付** Fyne 桌面 GUI（`cmd/client-gui` / `internal/clientgui`）；移动端仍不做
8. 多节点集群 / 高可用（v1.0 单节点现场部署）
9. IPv6 双栈（v1.0 仅 IPv4）
10. LDAP/AD 对接（v1.0 仅内置账号体系）

## SQLite 数据模型（权威：schema.sql）

> **以 [`internal/persist/schema.sql`](../internal/persist/schema.sql) 为准**；下表为历史草稿，已过时处勿再照抄。
>
> 现状要点：`users` 合一（无独立 `peers` 表）；另有 `peer_routes` / `peer_route_members` / `peer_access` / `client_lan_registry`、`security_events` / `ip_blocks`、日志历史库等。详见 [architecture.md](architecture.md) 与 `persist` 包。

| 表名（现行） | 用途 |
|------|------|
| `users` | Web+隧道合一账号（含密钥、vpn_ip、allowed_ips、ip_mode…） |
| `peer_routes` / `peer_route_members` | 托管路由 dest via + 访问方 |
| `peer_access` | 账号互访白名单 |
| `client_lan_registry` | 客户端上报的 local_lans 临时广告 |
| `ip_allocations` / `connection_events` / `session_stats` / `audit_logs` | IP 池、连接、会话、管理审计 |
| `security_events` / `ip_blocks` | 探针事件与封禁 |

### 历史草稿（归档，勿用于实现）

| 表名 | 用途 | 主要字段 |
|------|------|----------|
| `users` | 管理面登录账号 | id, username, password_hash, enabled, created_at, updated_at |
| `peers` | （已合并进 users） | — |
| `ip_allocations` | IP 池占用 | ip, peer_id, allocated_at, released_at |
| `connection_events` | 上下线/断线事件 | id, peer_id, event_type, remote_addr, created_at, detail_json |
| `session_stats` | 当前会话与累计统计 | peer_id, connected_at, last_heartbeat, rx_bytes, tx_bytes, reconnect_count |
| `audit_logs` | 管理操作审计 | id, actor_user_id, action, target_type, target_id, client_ip, detail_json, created_at |

- 外键与索引以 `schema.sql` 为准。
- 依赖：`modernc.org/sqlite`（纯 Go）。

## YAML 配置与首次启动生成

### 行为约定
1. 启动时检查 `-c` 指定路径（或默认 `./server.yaml` / `./client.yaml`）。
2. **文件不存在** → 创建父目录 → 写入**带注释的默认配置** → 打印提示「已生成默认配置，请检查后重启或继续启动」。
3. **文件已存在** → 正常加载；校验失败则 FATAL 并指出字段。
4. 注释规范：每个顶层块与关键字段上方用 `#` 写清：含义、默认值、单位、示例、安全注意（如证书、密码、**allow_public_bind 免责声明**）。
5. `allow_public_bind` 在默认模板中必须为 `false`，且附有**加粗式多行警告注释**（`defaults.go` 内嵌模板）。

### 服务端 `server.yaml` 结构（示意）
```yaml
# HaoVPN 服务端配置
# 首次启动自动生成，修改后重启生效

server:
  # TLS 监听地址，仅项目现场内网/frp 回连使用，不要暴露到公网管理口
  listen: "0.0.0.0:8443"
  tls:
    cert_file: "./certs/server.crt"
    key_file: "./certs/server.key"
    # 证书不存在时自动生成自签（生产建议关 false 并换正式证书）
    auto_generate: true

vpn:
  # VPN 虚拟网段，客户端从此池分配 IP
  subnet: "10.88.0.0/24"
  mtu: 1420
  heartbeat_timeout_sec: 30

nat:
  # 允许 VPN 客户端访问的现场工控网段
  allowed_lan_cidrs:
    - "192.168.1.0/24"

database:
  # SQLite 数据库文件路径
  path: "./data/haovpn.db"

api:
  # 管理 API/WebUI 监听地址。默认仅本机；TUN 启动后会自动追加 VPN 网卡 IP
  # 可填 127.0.0.1、具体内网 IP、或 0.0.0.0/::（见下方 allow_public_bind）
  listen_hosts: ["127.0.0.1"]
  port: 8080
  # ── 危险选项 · 默认 false ─────────────────────────────────────
  # 设为 true 表示：您已知悉将管理口绑定到所有网卡（含公网）的风险，并自行承担后果，与软件无关。
  # 仅建议在开发联调、内网测试时使用；生产现场务必保持 false。
  # 当 listen_hosts 含 0.0.0.0 或 :: 时，本项必须为 true，否则拒绝启动。
  allow_public_bind: false
  # ─────────────────────────────────────────────────────────────
  # 登录防暴力：同 IP 连续失败次数 / 锁定时长（秒）
  login_max_attempts: 5
  login_lockout_sec: 900
  # Session 有效期（秒），默认 8 小时
  session_ttl_sec: 28800

security:
  # 隧道准入：可选，限制哪些来源 IP 能发起 TLS 连接（空=不限制）
  tunnel_allowed_source_ips: []
  # 客户端导出配置是否强制分流（true=禁止下发 0.0.0.0/0）
  enforce_split_tunnel: true

admin:
  # 首次初始化默认管理员（仅 users 表为空时生效）
  username: "admin"
  password: "changeme"

log:
  level: "info"
  file: "./logs/server.log"
  max_size_mb: 100
  max_backups: 7
```

### 客户端 `client.yaml` 结构（示意）
```yaml
# HaoVPN 客户端配置
# vpn_ip / allowed_ips / gateway / 私钥均由握手下发，不含 peer 段

server:
  address: "vpn.example.com:8443"
  tls:
    ca_file: "./certs/ca.crt"
    insecure_skip_verify: false

tun:
  name: "haovpn0"
  mtu: 1420

auth:
  username: "engineer1"
  remember_password: false
  # password:  # GUI 记住密码或 HAOVPN_PASSWORD

security:
  kill_switch: false

reconnect:
  initial_sec: 1
  max_sec: 8

log:
  level: "info"
  file: "./logs/client.log"
```

> 实现说明：标准 `yaml.v3` 不保留注释写入能力，**首次生成**使用 `defaults.go` 内嵌模板字符串（保证注释完整）；**加载**用结构体反序列化即可。

## 数据流全景（v1.0完整链路）
```
【工程师客户端】
操作系统协议栈 → TUN网卡 → crypto加密 → transport(TLS‑TCP) → frp反向隧道 → VPS中转

【项目现场服务端】
transport(TLS‑TCP) → crypto解密 → sessionmgr匹配会话&AllowedIPs → TUN网卡 → netstack(NAT) → 现场工控物理局域网

管理访问（默认）：
现场本机 / VPN接入工程师 → http://tun‑ip:port → 登录 → api + webui

管理访问（仅开发/自测，须显式勾选 allow_public_bind: true）：
listen_hosts 含 0.0.0.0 → 按配置监听；风险由配置者承担，启动日志已免责声明

对外演示主线（一句话）：
**安全** — 默认非公网管理，误配 0.0.0.0 须显式勾选并自担风险 → **简单** — 一个文件拷过去就能跑 → **快** — 5 分钟部署，10 秒连上，8 秒重连
```

---

## 目录变更（2026-08 架构重构）

> **存档说明**：v1.0 规划正文中的目录树与「peer」术语为历史快照（账号已物理合并进 `users`；client.yaml 无 peer 段）。**当前结构以 [architecture.md](architecture.md) 为准。**

重构后新增/调整的模块主要包括：

| 新增/调整 | 说明 |
|-----------|------|
| `cmd/client-gui` | Fyne 桌面入口（薄）；UI 在 `internal/clientgui` |
| `internal/clientgui` | 桌面 GUI（登录/托盘/主窗） |
| `internal/clientapp` | CLI/GUI 共用拨号引擎 |
| `internal/serverapp` | 服务端启动编排 |
| `internal/netutil` | CIDR/监听/MTU/IP 匹配等纯函数 |
| `internal/winnet` | Windows 网卡 LUID/netsh 公共层 |
| `internal/vpnaccount` | Web 开户、IP 模式、PATCH/启禁/删号 |
| `internal/singleinstance` | 客户端单实例（**127.0.0.1 TCP 协调**，非文件锁） |
| `internal/credentials` | Windows DPAPI 凭据 |
| `internal/logstore` | WebUI 结构化历史日志库 |
| `internal/brand` | 产品名与路径常量 |
| `internal/paginate` | 分页 / bool / ParseLimitOffset 纯函数 |
| `internal/maintenance` | 数据保留后台 |
| `internal/fileutil` | 原子写 / exe 目录 / EnsureDir / AbsPair / RestrictToAdminsOnly |
| `internal/timeutil` | SQLite UTC + RFC3339 |
| `internal/readmodel` | Web/API 读模型 DTO（含 `peers.go`） |
| `internal/platform` | UAC（EscapeArg）、无窗口子进程 |
| `internal/autostart` | 登录自启 + 开机服务（Win/Linux/macOS）；`gen.go` / `logon_*.go` / `service_*.go` |
| `internal/serverapp` | 启动分阶段 `boot_persist.go` … `boot_api.go` |
| `persist/peer_*.go` | 托管路由 / 互访 / 规范化（自胖 handler 拆出） |
| `vpnaccount/peer_apply.go` | `PeerPolicyApplier`（peer dirty/apply 出 api） |
| `web/static/login.js` 及各页 `*.js` | 管理页脚本外置；CSP `script-src 'self'` |

各 `internal/*` 包均有中文 `doc.go`；默认值与 transport 映射已收敛到 `config.ApplyDefaults` 与 `transport.FromClientConfig`。
