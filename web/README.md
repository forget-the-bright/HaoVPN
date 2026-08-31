# HaoVPN WebUI

极简管理控制台前端：HTML 模板 + 静态 CSS/JS，由服务端 `go:embed` 打入二进制。

## 目录结构

| 路径 | 说明 |
|------|------|
| `embed.go` | `//go:embed templates/*.html static/*` → `web.FS`，供 `internal/api` 挂载 |
| `templates/*.html` | 登录、首页、用户列表、**托管路由**、连接详情、审计、探针、工具（**无内联业务 script**） |
| `static/style.css` | 共用样式（无外部 CDN） |
| `static/app.js` | 共用脚本：CSRF、fetch 封装、Toast、分页、`formatTime`（按 `api.display_timezone`） |
| `static/login.js` | 登录页 |
| `static/index.js` | 控制台首页 |
| `static/user_list.js` | 账号列表 |
| `static/peer_routes.js` | 托管路由 / 互访 |
| `static/connection_detail.js` | 连接详情 |
| `static/audit_log.js` | 审计日志 |
| `static/security_probe.js` | 探针 / 封禁 |
| `static/tools.js` | 工具（备份等） |
| `static/logo.png` | 品牌图（登录页 / 侧栏）；与 `docs/assets/haovpn-logo.png` 同源 |
| `static/favicon.ico` / `favicon-32.png` / `favicon-16.png` | 浏览器页签图标；由 `scripts/gen-icons.go` 从 `assets/haovpn-logo-source.png` 生成 |

## 页面 → 模板 → JS 对照

| 路由 | 模板 | 页脚本 | 主要功能 |
|------|------|--------|----------|
| `/login` | `login.html` | `login.js` | 登录、须改密 |
| `/` | `index.html` | `index.js` | 总览 / Dashboard |
| `/users` | `user_list.html` | `user_list.js` | 账号 CRUD、策略、导出 |
| `/peers` | `peer_routes.html` | `peer_routes.js` | 托管路由、互访、应用生效 |
| `/connections` | `connection_detail.html` | `connection_detail.js` | 在线连接详情 |
| `/audit` | `audit_log.html` | `audit_log.js` | 管理审计日志 |
| `/security` | `security_probe.html` | `security_probe.js` | 探针事件、封禁（时长预设/自定义）、**封禁豁免**、解封 |
| `/tools` | `tools.html` | `tools.js` | 备份、日志等维护 |

共用逻辑（退出、`HaoVPN.api`、Toast、分页）在 `app.js`。

## CSP

- `script-src 'self'`：页面逻辑必须在 `static/*.js`，禁止模板内联 `<script>` **与** `onclick=` 等内联事件（后者同样被浏览器 CSP 拦截）。
- 动态表格按钮用 `data-action` + `addEventListener` 委托（如探针页 `unban-ip`、`ban-event-ip`、`remove-exempt`）。
- 退出登录：`data-action="logout"`，由 `app.js` 绑定。
- `style-src` 仍允许 `'unsafe-inline'`（见 `internal/security/tls_policy.go`）。
- 各页 `<head>` 须含 `rel="icon"` 指向 `/static/favicon.ico`（回归见 `internal/api/webui_csp_test.go`）。

## 与后端的对应关系

- **路由注册**：`internal/api/handler_routes.go` 将 `/`、`/login`、`/users`、`/peers`、`/security` 等映射到 `templates` 渲染。
- **API 调用**：页面脚本经 `app.js` 的 `HaoVPN.api()` 访问 `/api/v1/*`（须带 Session Cookie + CSRF）。
- **CSRF**：`HaoVPN.refreshCSRF()` 从 `GET /api/v1/csrf` 取 token，写请求头 `X-CSRF-Token`（须改密时亦可取 CSRF）。
- **登录改密**：须改密时填当前密码+新密码；成功后跳转 `/login`（服务端已吊销全部 Web Session）。
- **账号页**：`/users` — 新建、策略编辑、管理员改密、踢线、导出 ZIP/YAML（**POST+CSRF**，`downloadPost`；不含私钥）。
- **审计页**：`/audit` — 动作 `码（中文）`、用户目标 `用户名 (#id)`；字典 `internal/audit/labels.go`；时间经 `HaoVPN.formatTime`。
- **工具页**：备份数据库为 **POST** `/api/v1/backup`（须 CSRF）。
- **托管路由页**：`/peers` — 全局互访开关、Managed Routes、互访白名单；改完后点「应用生效」（领域层 `vpnaccount.PeerPolicyApplier`，HTTP 仅薄封装）。
- **探针页**：`/security` — `security_events` / `ip_blocks` / `exempts`；封禁 POST 含 `duration_sec`；豁免 CRUD；隧道口拒绝码 `HAOVPN:IP_BANNED` / `SOURCE_DENIED`（见 `transport/probe_banner.go`）。
- **连接详情**：`/connections/...` — 事件时间经 `formatTime`。
- **展示时区**：`GET /api/v1/system/info` 的 `display_timezone`；存库与 API JSON 仍为 UTC。配置项 `api.display_timezone`。
- **静态资源**：`/static/*` 由 embed FS 直接 Serve。

## 开发注意

- 修改模板或静态文件后须 **重新编译服务端**（embed 在构建时固化）。
- 更新 favicon：改源图后 `go run scripts/gen-icons.go`（在 `scripts/` 目录或项目根执行均可）。
- 无 npm/webpack；保持零外部依赖，便于离线/内网部署。
- 新页面：在 `templates/` 增加 HTML + `static/<page>.js`，在 `handler_routes.go` 注册路由；共用逻辑放入 `app.js` 并挂到 `window.HaoVPN`。
- **注释**：HTML/JS/CSS 须中文注释说明用途；遵循 [comment-style.md](../docs/comment-style.md)。

## 相关文档

- [architecture.md](../docs/architecture.md) — `api` 包与 WebUI 边界
- [security-hardening.md](../docs/security-hardening.md) — CSP、探针封禁 API
- [comment-style.md](../docs/comment-style.md) — `app.js` 主要函数须有中文文件头/注释
