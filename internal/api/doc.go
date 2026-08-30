// Package api 提供 HTTP 管理 API 与 WebUI 路由（默认仅本机 + TUN IP）。
//
// 关键文件：
//   handler.go / handler_routes.go / handler_listen.go — Server、路由注册、多地址监听
//   httputil.go — writeJSON/writeOK/writeOKWith/writePendingApply/writeItems/requireMethod/parseFormInt64…
//   auth_handlers.go — 登录/登出/改密/CSRF、requireAuth
//   users_crud.go / users_vpn.go / users_export.go / export_zip.go — 账号 CRUD、策略、导出
//   handler_peer_routes.go / handler_peer_access.go / handler_lan_registry.go —
//     托管路由、互访、LAN 注册（只写库；须「应用生效」）
//   handler_peers_apply.go / handler_peers_dirty.go — pending_apply 脏集与踢线刷新
//   handler_vpn_peers_policy.go — 全局互访开关
//   handler_ops.go / monitor_handler.go / handler_security.go / logs.go — 运维与探针
//
// 业务边界（避免后人误「一切经 vpnaccount」）：
//   - 账号 IP 模式 / AllowedIPs / 启禁 / 删号 → vpnaccount（ApplyVPNPatch、SetAccountEnabled、DeleteAccount）
//   - 托管路由 / 互访白名单 / LAN 注册 / 应用生效 → persist 写库 + sessionmgr.KickUser（编排在本包）
//   - HTTP 读模型 DTO → readmodel（PeerRouteView / PeerAccessView / LANRegistryView 等）
//
// 上游：serverapp 启动 Listen；WebUI / 脚本调用 REST。
// 下游：auth、vpnaccount、sessionmgr、persist、readmodel、paginate、timeutil、netutil、config、probedefense。
// 并发：HTTP 多 goroutine；依赖 store/auth 各自线程安全。
// 不变量：写操作须 CSRF；公开 health 仅 ok+uptime；改密/重置/禁用吊销 Web 会话。
package api
