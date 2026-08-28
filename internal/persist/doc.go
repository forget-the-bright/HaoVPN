// Package persist 提供 SQLite 持久化：用户/VPN 账号、IP 占用、连接事件、审计与 schema。
//
// 关键文件（同包拆分）：
//   store.go — Store、Open/Close/DB、migrate、User/AuditEntry 等类型
//   users.go — 账号 CRUD（Create/Get/List/Delete/密码/VPN 字段）
//   audit_store.go — 审计日志写入与简单分页
//   session_store.go — connection_events、session_stats、scanUser 辅助
//   query_ext.go — 带筛选的分页列表（ListUsersPage 等）
//   scan.go / jsoncol.go / timefmt.go — 行扫描与时间（委托 timeutil）
//
// 上游：serverapp、api、auth、vpnaccount、sessionmgr、tunnel。
// 下游：modernc.org/sqlite、paginate、readmodel、timeutil。
// 并发：max_open_conns=1；事务经 DB() 自行管理。
// 不变量：schema.sql 为唯一表结构；DeleteUser 须级联清理子表。
package persist
