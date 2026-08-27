// Package sessionmgr VPN 在线会话注册、踢线、TUN 出站路由与入站隔离校验。
//
// 关键文件：
//   manager.go — RegisterVPN、KickUser、RouteOutbound、HandleInbound
//   conn.go — PacketConn 窄接口（解耦 transport.Conn）
//
// 上游：serverapp、tunnel ServerHandler、api（踢线/监控）。
// 下游：persist（connection_events、session_stats）、crypto、netutil。
// 并发：Manager 持 RWMutex；RegisterVPN 异步 Close 旧连接避免死锁。
// 不变量：每 userID 最多一条 Conn；入站 src 须等于 VPN IP。
package sessionmgr
