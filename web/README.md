# HaoVPN WebUI

极简管理控制台前端：HTML 模板 + 静态 CSS/JS，由服务端 `go:embed` 打入二进制。

## 目录结构

| 路径 | 说明 |
|------|------|
| `embed.go` | `//go:embed templates/*.html static/*` → `web.FS`，供 `internal/api` 挂载 |
| `templates/*.html` | 各页面骨架：登录、用户列表、连接详情、审计、工具 |
| `static/style.css` | 共用样式（无外部 CDN） |
| `static/app.js` | 共用脚本：CSRF、fetch 封装、Toast、分页、格式化 |

## 与后端的对应关系

- **路由注册**：`internal/api/handler.go` 将 `/`、`/login`、`/users` 等映射到 `templates` 渲染。
- **API 调用**：页面内联脚本或 `app.js` 的 `HaoVPN.api()` 访问 `/api/v1/*`（须带 Session Cookie + CSRF）。
- **CSRF**：`HaoVPN.refreshCSRF()` 从 `GET /api/v1/csrf` 取 token，写请求头 `X-CSRF-Token`。
- **账号页**：`/users` — 新建账号、列表筛选、策略编辑、**改密**（`POST /api/v1/users/{id}/password`）、踢线、导出 ZIP/YAML。
- **静态资源**：`/static/style.css`、`/static/app.js` 由 embed FS 直接 Serve。

## 开发注意

- 修改模板或静态文件后须 **重新编译服务端**（embed 在构建时固化）。
- 无 npm/webpack；保持零外部依赖，便于离线/内网部署。
- 新页面：在 `templates/` 增加 HTML，必要时在 `handler.go` 注册路由；共用逻辑放入 `app.js` 并挂到 `window.HaoVPN`。
- **注释**：HTML/JS/CSS 须中文注释说明用途；`app.js` 主要函数遵循 [comment-style.md](../docs/comment-style.md)（文件头 + 非显而易见逻辑）。

## 相关文档

- [architecture.md](../docs/architecture.md) — `api` 包与 WebUI 边界
- [comment-style.md](../docs/comment-style.md) — `app.js` 主要函数须有中文文件头/注释
