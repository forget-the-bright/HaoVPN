# HaoVPN WebUI

极简管理控制台前端：HTML 模板 + 静态 CSS/JS，由服务端 `go:embed` 打入二进制。

## 目录结构

| 路径 | 说明 |
|------|------|
| `embed.go` | `//go:embed templates/*.html static/*` → `web.FS`，供 `internal/api` 挂载 |
| `templates/*.html` | 登录、用户列表、**托管路由**、连接详情、审计、探针、工具 |
| `static/style.css` | 共用样式（无外部 CDN） |
| `static/app.js` | 共用脚本：CSRF、fetch 封装、Toast、分页、格式化 |

## 与后端的对应关系

- **路由注册**：`internal/api/handler_routes.go` 将 `/`、`/login`、`/users`、`/peers`、`/security` 等映射到 `templates` 渲染。
- **API 调用**：页面内联脚本或 `app.js` 的 `HaoVPN.api()` 访问 `/api/v1/*`（须带 Session Cookie + CSRF）。
- **CSRF**：`HaoVPN.refreshCSRF()` 从 `GET /api/v1/csrf` 取 token，写请求头 `X-CSRF-Token`。
- **登录改密**：须改密时填当前密码+新密码；成功后跳转 `/login`（服务端已吊销全部 Web Session）。
- **账号页**：`/users` — 新建、策略编辑、管理员改密、踢线、导出 ZIP/YAML（**POST+CSRF**，`downloadPost`；不含私钥）。
- **审计页**：`/audit` — 动作 `码（中文）`、用户目标 `用户名 (#id)`；字典 `internal/audit/labels.go`。
- **工具页**：备份数据库为 **POST** `/api/v1/backup`（须 CSRF）。
- **托管路由页**：`/peers` — 全局互访开关、Managed Routes（`dest via vpn_ip`）、互访白名单；API `/api/v1/peer-routes`、`/peer-access`、`/security/vpn-peers`。
- **探针页**：`/security` — `security_events` / `ip_blocks`。
- **静态资源**：`/static/style.css`、`/static/app.js` 由 embed FS 直接 Serve。

## 开发注意

- 修改模板或静态文件后须 **重新编译服务端**（embed 在构建时固化）。
- 无 npm/webpack；保持零外部依赖，便于离线/内网部署。
- 新页面：在 `templates/` 增加 HTML，必要时在 `handler_routes.go` 注册路由；共用逻辑放入 `app.js` 并挂到 `window.HaoVPN`。
- **注释**：HTML/JS/CSS 须中文注释说明用途；`app.js` 主要函数遵循 [comment-style.md](../docs/comment-style.md)。

## 相关文档

- [architecture.md](../docs/architecture.md) — `api` 包与 WebUI 边界
- [comment-style.md](../docs/comment-style.md) — `app.js` 主要函数须有中文文件头/注释
