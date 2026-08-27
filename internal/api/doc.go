// Package api 提供 HTTP 管理 API 与 WebUI 路由（默认仅本机 + TUN IP）。
//
// 关键文件：
//   handler.go — Server 类型、路由注册、健康/备份/仪表盘
//   auth_handlers.go — 登录/登出/改密、requireAuth 中间件
//   users.go — VPN 账号 CRUD、策略 PATCH、管理员改密、导出
//   httputil.go — writeJSON、clientIP、分页 clamp
//   export.go / export_zip.go — 客户端配置导出
//   monitor_handler.go — 在线监控 API
//   pages.go — WebUI 页面渲染
//
// 上游：serverapp 启动 Listen；WebUI 浏览器 / 脚本调用 REST。
// 下游：auth、persist、vpnaccount、sessionmgr、readmodel。
// 并发：HTTP 多 goroutine；依赖 store/auth 各自线程安全。
// 不变量：写操作须 CSRF；账号/IP 细节经 vpnaccount.Service，不直接操作 IP 池 SQL。
package api
