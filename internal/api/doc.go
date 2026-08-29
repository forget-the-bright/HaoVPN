// Package api 提供 HTTP 管理 API 与 WebUI 路由（默认仅本机 + TUN IP）。
//
// 关键文件（第九轮同包拆分）：
//   handler.go — Server、NewServer、Listen/Close
//   handler_routes.go — routes 注册
//   handler_ops.go — health/audit/dashboard/logs/backup、LogPublicBindAudit
//   handler_listen.go — StartAllListeners、FormatBoundAddrs、listenAPI
//   auth_handlers.go — 登录/登出/改密（须 old_password）/CSRF、requireAuth（失败关闭）
//   handler_security.go — 探针事件/封禁 API
//   users_crud.go / users_vpn.go / users_export.go — 账号 CRUD、策略 PATCH、导出（不解密私钥）
//   monitor_handler.go — 监控 API（JOIN 无 N+1）
//   httputil.go — writeJSON/writeOK/writePage/writeAttachment、parseSinceQuery、clientIP
//   export_zip.go — ZIP 导出（仅 yaml+证书）
//
// 上游：serverapp 启动 Listen；WebUI 浏览器 / 脚本调用 REST。
// 下游：auth、vpnaccount、sessionmgr、persist、readmodel、paginate、timeutil、netutil、config、probedefense。
// 并发：HTTP 多 goroutine；依赖 store/auth 各自线程安全。
// 不变量：写操作须 CSRF；VPN 写经 vpnaccount.ApplyVPNPatch/SetAccountEnabled；
// 改密/重置/禁用吊销 Web 会话；must_change/用户失效时 requireAuth 不得跳过。
package api
