// Package persist 提供 SQLite 持久化：用户/VPN 账号、IP 占用、连接事件、审计与 schema。
//
// 关键文件（第九轮同包拆分）：
//   store.go — Store、Open/Close/DB、migrate、User/AuditEntry 等类型
//   constants.go — DefaultIPLeaseSec（与 schema 默认租约同源）
//   users.go — 账号 CRUD（Create/Get/List/Delete/密码/VPN 字段）
//   audit_store.go — 审计日志写入与简单分页
//   session_store.go — connection_events、session_stats、scanUser 辅助
//   query_page.go — queryPageTotal 分页 COUNT+SELECT 辅助
//   query_users.go — ListUsersPage、UsernameByID
//   query_audit.go — ListAuditLogsFiltered、PruneAuditLogs
//   query_events.go — ListConnectionEventsFiltered、PruneConnectionEvents
//   query_monitor.go — ListMonitorAccountRows（JOIN 无 N+1）
//   scan.go / jsoncol.go / audit_view.go — 行扫描、JSON 列、审计视图；时间统一 timeutil
// 查询层 DTO 边界：ListUsersPage / ListMonitorAccountRows 等直接返回 readmodel 类型，
// 避免 api 层再做 persist→JSON 字段映射；审计视图转换在 audit_view.go。
//
// 上游：serverapp、api、auth、vpnaccount、sessionmgr、tunnel。
// 下游：modernc.org/sqlite、paginate、readmodel、timeutil。
// 并发：max_open_conns=1；事务经 DB() 自行管理。
// 不变量：schema.sql 为唯一表结构；DeleteUser 须级联清理子表。
package persist
