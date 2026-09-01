// Package sessionmgr 管理 VPN 在线会话：注册、踢线、出站路由与入站隔离。
//
// 关键文件（第十六轮同包拆分）：
//   manager.go — Manager、AccountSession、IP 索引、踢线/断线回调
//   register.go — RegisterVPN、RemoveIfConn
//   register_grace.go — 注册宽限期与 replay 协调
//   register_lan.go — LAN 注册表上报与合并
//   kick.go — KickUser
//   route.go — RouteOutbound、sendToAccount
//   route_inbound.go — HandleInbound
//   route_lookup.go — 按 VPN IP / via 查找
//   route_policy.go — 源 IP / ExitLAN / 横向隔离策略
//   stats.go — 在线统计、入站字节累加
//
// Conn 接口（PeerActivityConn / DrainableConn / DataCallbackConn）由 transport.Conn 方法集满足，
// register 内 type assert 使用，勿在 sessionmgr 外 import transport 仅为挂名。
//
// 上游：serverapp、tunnel ServerHandler、api（踢线/监控）。
// 下游：persist、crypto、netutil、logger、auth。
// 并发：Manager 持 RWMutex；RegisterVPN 异步 Close 旧连接避免死锁。
// 不变量：每 userID 至多一条活跃 Conn；入站 src 须为本账号 VPN IP（或合法 ExitLAN+via）。
package sessionmgr
