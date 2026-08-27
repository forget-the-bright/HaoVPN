// Package persist 提供 SQLite 持久化：用户/VPN 账号、IP 占用、连接事件、审计与 schema 应用。
//
// 上游：serverapp 启动时 Open；api 读写用户/审计；auth 校验密码；vpnaccount 分配 IP；
// sessionmgr 写入会话统计与连接事件；tunnel.ServerHandler 按公钥/用户名查账号。
// 下游：modernc.org/sqlite；paginate 分页裁剪；readmodel 列表 DTO；security.KeyEnc 私钥加密封存。
// 并发：Store 内部 max_open_conns=1，多 goroutine 调用方法由 SQLite 串行化；
// 事务须通过 DB() 自行 Begin/Commit；Close 后所有方法失败。
// 不变量：schema.sql 为唯一表结构来源；启动 migrate 仅 CREATE IF NOT EXISTS（无 v1→v3 运行时迁移）；
// DeleteUser 须级联清理子表否则 FK 失败；私钥字段须经 KeyEnc 密封后入库；IP 占用与 users.vpn_ip 由 vpnaccount 协调一致。
package persist
